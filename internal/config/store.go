package config

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
)

const (
	V1Filename     = "config.v1.json"
	LegacyFilename = "config.json"
	configVersion  = "igw-config/1"
	maxConfigBytes = 1 << 20
)

// Store leaves the legacy file untouched. Dir is a private, user-owned local
// directory; writers cooperate through a persistent kernel-locked lock file.
type Store struct{ Dir string }

type document struct {
	Version      string `json:"schemaVersion"`
	Revision     string `json:"revision"`
	LegacySHA256 string `json:"legacySHA256,omitempty"`
	Config       File   `json:"configuration"`
}

type State struct {
	Config   File   `json:"-"`
	Source   string `json:"source"`
	Revision string `json:"revision,omitempty"`
	doc      document
	raw      []byte
}

type ProfileSummary struct {
	Name            string `json:"name,omitempty"`
	GatewayURL      string `json:"gatewayURL"`
	TokenConfigured bool   `json:"tokenConfigured"`
}

type Summary struct {
	Source        string           `json:"source"`
	Revision      string           `json:"revision,omitempty"`
	ActiveProfile string           `json:"activeProfile"`
	Defaults      ProfileSummary   `json:"defaults"`
	Profiles      []ProfileSummary `json:"profiles"`
}

func (s State) Summary() Summary {
	summary := Summary{Source: s.Source, Revision: s.Revision, ActiveProfile: s.Config.ActiveProfile,
		Defaults: ProfileSummary{GatewayURL: s.Config.GatewayURL, TokenConfigured: strings.TrimSpace(s.Config.Token) != ""}, Profiles: []ProfileSummary{}}
	for name, p := range s.Config.Profiles {
		summary.Profiles = append(summary.Profiles, ProfileSummary{Name: name, GatewayURL: p.GatewayURL, TokenConfigured: strings.TrimSpace(p.Token) != ""})
	}
	sort.Slice(summary.Profiles, func(i, j int) bool { return summary.Profiles[i].Name < summary.Profiles[j].Name })
	return summary
}

func (s Store) directory() (string, error) {
	if s.Dir != "" {
		return s.Dir, nil
	}
	return Dir()
}

// Read prefers v1. An invalid v1 file never silently selects legacy credentials.
// Reading, including a missing directory, has no filesystem side effects.
func (s Store) Read() (State, error) {
	dir, err := s.directory()
	if err != nil {
		return State{}, errors.New("could not resolve configuration directory")
	}
	if err = checkDirectory(dir, false); err != nil {
		return State{}, err
	}
	raw, exists, err := readPrivateFile(filepath.Join(dir, V1Filename))
	if err != nil {
		return State{}, err
	}
	if exists {
		doc, err := decodeDocument(raw)
		if err != nil {
			return State{}, err
		}
		return State{Config: doc.Config, Source: "v1", Revision: doc.Revision, doc: doc, raw: raw}, nil
	}
	raw, exists, err = readPrivateFile(filepath.Join(dir, LegacyFilename))
	if err != nil {
		return State{}, err
	}
	if !exists {
		return State{Source: "none"}, nil
	}
	cfg, err := decodeConfig(raw)
	if err != nil {
		return State{}, err
	}
	return State{Config: cfg, Source: "legacy", raw: raw}, nil
}

func checkDirectory(dir string, write bool) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) && !write {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configuration directory must be a real directory")
	}
	if write && runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("configuration writes require a private directory (mode 0700)")
	}
	return nil
}

func readPrivateFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, errors.New("configuration must be a readable regular file, not a link")
	}
	if info.Size() > maxConfigBytes {
		return nil, false, errors.New("configuration exceeds 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, errors.New("could not open configuration")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, false, errors.New("configuration changed while opening it")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil || len(raw) > maxConfigBytes {
		return nil, false, errors.New("could not read configuration within 1 MiB")
	}
	return raw, true, nil
}

func object(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	bad := errors.New("configuration contains invalid, duplicate, unknown, or incorrectly cased fields")
	var fields map[string]json.RawMessage
	if jsonvalue.Validate(raw) != nil || !jsonvalue.ValidUnicode(raw) || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, bad
	}
	for key, value := range fields {
		found := false
		for _, a := range allowed {
			if key == a {
				found = true
				break
			}
		}
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, bad
		}
	}
	return fields, nil
}

func decodeConfig(raw []byte) (File, error) {
	fields, err := object(raw, "gatewayURL", "token", "activeProfile", "profiles")
	if err != nil {
		return File{}, err
	}
	if profiles, ok := fields["profiles"]; ok {
		var entries map[string]json.RawMessage
		if json.Unmarshal(profiles, &entries) != nil || entries == nil {
			return File{}, errors.New("profiles must be an object")
		}
		for name, entry := range entries {
			if !validName(name) {
				return File{}, errors.New("profile names must be nonempty with no surrounding whitespace or control characters")
			}
			if _, err := object(entry, "gatewayURL", "token"); err != nil {
				return File{}, err
			}
		}
	}
	var cfg File
	if json.Unmarshal(raw, &cfg) != nil {
		return File{}, errors.New("configuration fields have invalid types")
	}
	if cfg.ActiveProfile != "" {
		if _, ok := cfg.Profiles[strings.TrimSpace(cfg.ActiveProfile)]; !ok {
			return File{}, errors.New("active profile does not exist")
		}
	}
	if !validURL(cfg.GatewayURL) {
		return File{}, errors.New("configured Gateway URL must be HTTP(S) without credentials, query, or fragment")
	}
	for _, p := range cfg.Profiles {
		if !validURL(p.GatewayURL) {
			return File{}, errors.New("profile Gateway URL must be HTTP(S) without credentials, query, or fragment")
		}
	}
	return cfg, nil
}

func validName(name string) bool {
	if name == "" || !utf8.ValidString(name) || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func validURL(raw string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	if strings.TrimSpace(raw) == "" {
		return true
	}
	_, err := gateway.JoinURL(strings.TrimSpace(raw), "")
	if err != nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." || strings.Contains(part, "\\") {
			return false
		}
	}
	return true
}

func decodeDocument(raw []byte) (document, error) {
	fields, err := object(raw, "schemaVersion", "revision", "legacySHA256", "configuration")
	if err != nil {
		return document{}, err
	}
	var doc document
	if json.Unmarshal(raw, &doc) != nil || doc.Version != configVersion || !hexLength(doc.Revision, 16) || doc.LegacySHA256 != "" && !hexLength(doc.LegacySHA256, 32) {
		return document{}, errors.New("unsupported or invalid v1 configuration metadata")
	}
	doc.Config, err = decodeConfig(fields["configuration"])
	return doc, err
}

func hexLength(value string, n int) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == n && strings.ToLower(value) == value
}
func fingerprint(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func revision() string              { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }

func publish(path string, raw []byte, replace bool) error {
	w, err := artifact.New(path, replace)
	if err != nil {
		return errors.New("could not prepare private configuration file")
	}
	defer w.Abort()
	if _, err = w.Write(raw); err != nil {
		return errors.New("could not write private configuration file")
	}
	if _, err = w.Commit(); err != nil {
		return errors.New("could not publish configuration file")
	}
	return nil
}
