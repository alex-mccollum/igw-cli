package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Change contains explicit local configuration edits, never environment-derived
// values. A nil pointer preserves a field; an empty token explicitly clears it.
type Change struct {
	Action                string
	Name                  string
	GatewayURL, Token     *string
	Activate, UseDefaults bool
	IfRevision            string
	Yes, DryRun           bool
}

type ChangeResult struct {
	Action      string  `json:"action"`
	Applied     bool    `json:"applied"`
	Before      Summary `json:"before"`
	After       Summary `json:"after"`
	Archive     string  `json:"archive,omitempty"`
	TokenChange string  `json:"tokenChange,omitempty"`
}

func (r Change) validate() error {
	if r.Yes == r.DryRun {
		return errors.New("choose --dry-run to preview or --yes to change local configuration")
	}
	if r.IfRevision != "" && !hexLength(r.IfRevision, 16) {
		return errors.New("configuration revision must be 32 lowercase hexadecimal characters")
	}
	switch r.Action {
	case "set":
		if !validName(r.Name) {
			return errors.New("a valid profile name is required")
		}
		if r.GatewayURL == nil && r.Token == nil && !r.Activate {
			return errors.New("supply a Gateway URL, token change, or --use")
		}
		if r.UseDefaults {
			return errors.New("set requires a named profile")
		}
	case "use":
		if r.UseDefaults == (r.Name != "") || r.Name != "" && !validName(r.Name) {
			return errors.New("choose a profile name or --default")
		}
	case "remove":
		if !validName(r.Name) {
			return errors.New("a valid profile name is required")
		}
	case "migrate":
		if r.IfRevision != "" {
			return errors.New("legacy configuration has no v1 revision; migration reads the current legacy file")
		}
	case "rollback":
		if r.IfRevision == "" {
			return errors.New("rollback requires --if-revision from profile list or its preview")
		}
	default:
		return errors.New("unknown profile change")
	}
	if r.Action != "set" && (r.GatewayURL != nil || r.Token != nil || r.Activate) || r.Action != "use" && r.UseDefaults {
		return errors.New("profile change contains inapplicable options")
	}
	if (r.Action == "migrate" || r.Action == "rollback") && r.Name != "" {
		return errors.New("migration does not accept a profile name")
	}
	if r.GatewayURL != nil && (!validURL(*r.GatewayURL) || strings.TrimSpace(*r.GatewayURL) == "") {
		return errors.New("profile Gateway URL must be a nonempty HTTP(S) URL without credentials, query, or fragment")
	}
	if r.Token != nil {
		if len(*r.Token) > 16<<10 || !utf8.ValidString(*r.Token) {
			return errors.New("token must be valid text within 16 KiB")
		}
		for _, r := range *r.Token {
			if r < 32 || r == 127 {
				return errors.New("token must not contain control characters")
			}
		}
	}
	return nil
}

func (s Store) plan(before State, r Change) (State, string, error) {
	if r.IfRevision != "" && r.IfRevision != before.Revision {
		return State{}, "", errors.New("configuration revision changed; inspect the current profiles before retrying")
	}
	after := before
	after.Config.Profiles = make(map[string]Profile, len(before.Config.Profiles))
	for k, v := range before.Config.Profiles {
		after.Config.Profiles[k] = v
	}
	if r.Action == "migrate" {
		if before.Source != "legacy" {
			return State{}, "", errors.New("migration requires a legacy configuration and no active v1 file")
		}
		after.doc.LegacySHA256 = fingerprint(before.raw)
	} else if r.Action == "rollback" {
		if before.Source != "v1" || before.doc.LegacySHA256 == "" {
			return State{}, "", errors.New("rollback requires a configuration created by migration")
		}
		dir, _ := s.directory()
		raw, exists, err := readPrivateFile(filepath.Join(dir, LegacyFilename))
		if err != nil || !exists || fingerprint(raw) != before.doc.LegacySHA256 {
			return State{}, "", errors.New("legacy configuration changed or is unavailable; rollback refused")
		}
		cfg, err := decodeConfig(raw)
		if err != nil {
			return State{}, "", err
		}
		return State{Source: "legacy", Config: cfg, raw: raw}, "config.v1.rollback-" + before.Revision + ".json", nil
	} else {
		if before.Source == "legacy" {
			return State{}, "", errors.New("run profile migrate --dry-run, then profile migrate --yes before changing legacy configuration")
		}
		switch r.Action {
		case "set":
			p := after.Config.Profiles[r.Name]
			if r.GatewayURL != nil {
				p.GatewayURL = strings.TrimSpace(*r.GatewayURL)
			}
			if r.Token != nil {
				p.Token = strings.TrimSpace(*r.Token)
			}
			after.Config.Profiles[r.Name] = p
			if r.Activate {
				after.Config.ActiveProfile = r.Name
			}
		case "use":
			if !r.UseDefaults {
				if _, ok := after.Config.Profiles[r.Name]; !ok {
					return State{}, "", errors.New("profile does not exist")
				}
			}
			after.Config.ActiveProfile = r.Name
		case "remove":
			if _, ok := after.Config.Profiles[r.Name]; !ok {
				return State{}, "", errors.New("profile does not exist")
			}
			if strings.TrimSpace(after.Config.ActiveProfile) == r.Name {
				return State{}, "", errors.New("select another profile or --default before removing the active profile")
			}
			delete(after.Config.Profiles, r.Name)
		}
	}
	after.Source = "v1"
	after.Revision = "" // A preview cannot assign the revision of a future write.
	after.doc.Version = configVersion
	after.doc.Config = after.Config
	return after, "", nil
}

// Change validates without writing, then locks and rechecks the exact source
// before publishing. The revision is an opaque random identifier, not a token
// or credential-derived hash. Rollback archives v1 bytes before removing v1.
func (s Store) Change(ctx context.Context, r Change) (ChangeResult, error) {
	if err := r.validate(); err != nil {
		return ChangeResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ChangeResult{}, err
	}
	before, err := s.Read()
	if err != nil {
		return ChangeResult{}, err
	}
	after, archive, err := s.plan(before, r)
	if err != nil {
		return ChangeResult{}, err
	}
	out := ChangeResult{Action: r.Action, Before: before.Summary(), After: after.Summary(), Archive: archive}
	if r.Action == "set" {
		out.TokenChange = "preserved"
		if r.Token != nil {
			out.TokenChange = "set"
			if strings.TrimSpace(*r.Token) == "" {
				out.TokenChange = "cleared"
			}
		}
	}
	if r.Action != "rollback" {
		planned := after.doc
		planned.Revision = strings.Repeat("0", 32)
		if _, err = encodeDocument(planned); err != nil {
			return ChangeResult{}, err
		}
	}
	if r.DryRun {
		return out, nil
	}
	dir, err := s.directory()
	if err != nil {
		return ChangeResult{}, errors.New("could not resolve configuration directory")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return ChangeResult{}, errors.New("could not create configuration directory")
	}
	if err = checkDirectory(dir, true); err != nil {
		return ChangeResult{}, err
	}
	lock, err := acquireLock(filepath.Join(dir, "config.v1.lock"))
	if err != nil {
		return ChangeResult{}, err
	}
	defer lock.Close()
	current, err := s.Read()
	if err != nil {
		return ChangeResult{}, err
	}
	if current.Source != before.Source || !bytes.Equal(current.raw, before.raw) {
		return ChangeResult{}, errors.New("configuration changed while preparing the edit; inspect profiles before retrying")
	}
	// Repeat migration/rollback validation under the writer lock, including the
	// preserved legacy hash. Editors and older executables do not use this lock.
	after, archive, err = s.plan(current, r)
	if err != nil {
		return ChangeResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return ChangeResult{}, err
	}
	if r.Action == "rollback" {
		archivePath := filepath.Join(dir, archive)
		prior, exists, err := readPrivateFile(archivePath)
		if err != nil {
			return ChangeResult{}, err
		}
		if exists && !bytes.Equal(prior, before.raw) {
			return ChangeResult{}, errors.New("rollback archive exists with different content")
		}
		if !exists {
			if err = publish(archivePath, before.raw, false); err != nil {
				return ChangeResult{}, err
			}
		}
		if err = os.Remove(filepath.Join(dir, V1Filename)); err != nil {
			return ChangeResult{}, errors.New("v1 was archived but could not be deactivated; inspect profiles before retrying")
		}
	} else {
		after.Revision = revision()
		after.doc.Revision = after.Revision
		raw, err := encodeDocument(after.doc)
		if err != nil {
			return ChangeResult{}, err
		}
		if err = publish(filepath.Join(dir, V1Filename), raw, before.Source == "v1"); err != nil {
			return ChangeResult{}, err
		}
	}
	out.Applied = true
	out.After = after.Summary()
	return out, nil
}

func encodeDocument(doc document) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, errors.New("could not encode configuration")
	}
	raw = append(raw, '\n')
	if len(raw) > maxConfigBytes {
		return nil, errors.New("updated configuration exceeds 1 MiB")
	}
	if _, err = decodeDocument(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
