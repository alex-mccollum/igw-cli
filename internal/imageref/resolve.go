// Package imageref resolves official Ignition releases without invoking Docker.
// Resolution identifies an update candidate; it does not qualify that image.
package imageref

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
)

const (
	Repository       = "inductiveautomation/ignition"
	Platform         = "linux/amd64"
	authURL          = "https://auth.docker.io/token?service=registry.docker.io&scope=repository:inductiveautomation/ignition:pull"
	manifestURL      = "https://registry-1.docker.io/v2/" + Repository + "/manifests/"
	ociIndex         = "application/vnd.oci.image.index.v1+json"
	dockerIndex      = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociManifest      = "application/vnd.oci.image.manifest.v1+json"
	dockerManifest   = "application/vnd.docker.distribution.manifest.v2+json"
	maxManifestBytes = 4 << 20
)

var releaseTag = regexp.MustCompile(`^8\.3(?:\.(?:0|[1-9][0-9]{0,4}))?$`)
var sha256Digest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type Resolution struct {
	Version        int       `json:"version"`
	Repository     string    `json:"repository"`
	Tag            string    `json:"tag"`
	ResolvedAt     time.Time `json:"resolvedAt"`
	Image          string    `json:"image"`
	IndexDigest    string    `json:"indexDigest"`
	ManifestDigest string    `json:"manifestDigest"`
	Platform       string    `json:"platform"`
}

// Candidate retains exact registry bytes separately from the receipt. In
// particular, JSON formatting must not change before their digests are checked.
type Candidate struct {
	Resolution Resolution
	Index      []byte
	Manifest   []byte
}

type descriptor struct {
	MediaType string   `json:"mediaType"`
	Digest    string   `json:"digest"`
	Size      int64    `json:"size"`
	URLs      []string `json:"urls"`
	Platform  struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"platform"`
}

type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []descriptor `json:"manifests"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

// Resolve accepts the 8.3 channel or an explicit 8.3 patch tag. All network
// endpoints are fixed, HTTPS-only, and redirect-free. Registry credentials are
// anonymous, pull-scoped, held only in memory, and never included in errors.
func Resolve(ctx context.Context, tag string) (Candidate, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return resolve(ctx, tag, client, time.Now)
}

func resolve(ctx context.Context, tag string, client *http.Client, now func() time.Time) (Candidate, error) {
	if !releaseTag.MatchString(tag) {
		return Candidate{}, errors.New("release tag must be 8.3 or 8.3.<patch>")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	// Copy the caller's client so tests can replace its transport but cannot
	// accidentally allow redirects or persist a registry token in a cookie jar.
	copyClient := *client
	copyClient.Jar = nil
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	tokenBody, _, _, err := get(ctx, &copyClient, authURL, "", 512<<10)
	if err != nil {
		return Candidate{}, fmt.Errorf("registry authentication: %w", err)
	}
	var auth struct {
		Token string `json:"token"`
	}
	if decode(tokenBody, &auth) != nil || auth.Token == "" || len(auth.Token) > 256<<10 || strings.ContainsAny(auth.Token, "\r\n") {
		return Candidate{}, errors.New("registry authentication returned an invalid token")
	}
	index, indexDigest, indexType, err := get(ctx, &copyClient, manifestURL+tag, auth.Token, maxManifestBytes)
	if err != nil {
		return Candidate{}, fmt.Errorf("release index: %w", err)
	}
	if err := verifyDigest(index, indexDigest); err != nil {
		return Candidate{}, err
	}
	var root manifest
	if decode(index, &root) != nil || root.SchemaVersion != 2 || (root.MediaType != ociIndex && root.MediaType != dockerIndex) || root.MediaType != indexType {
		return Candidate{}, errors.New("release must provide a supported image index with platform descriptors")
	}
	var selected descriptor
	matches := 0
	for _, entry := range root.Manifests {
		if entry.Platform.OS == "linux" && entry.Platform.Architecture == "amd64" && entry.Platform.Variant == "" {
			selected = entry
			matches++
		}
	}
	if matches != 1 {
		return Candidate{}, errors.New("release index must identify exactly one linux/amd64 manifest")
	}
	if !sha256Digest.MatchString(selected.Digest) || selected.Size <= 0 || selected.Size > maxManifestBytes || len(selected.URLs) > 0 || (selected.MediaType != ociManifest && selected.MediaType != dockerManifest) {
		return Candidate{}, errors.New("release index has an unsupported platform manifest descriptor")
	}
	body, digest, mediaType, err := get(ctx, &copyClient, manifestURL+selected.Digest, auth.Token, maxManifestBytes)
	if err != nil {
		return Candidate{}, fmt.Errorf("platform manifest: %w", err)
	}
	if digest != selected.Digest || int64(len(body)) != selected.Size || mediaType != selected.MediaType {
		return Candidate{}, errors.New("platform manifest does not match its index descriptor")
	}
	if err := verifyDigest(body, digest); err != nil {
		return Candidate{}, err
	}
	var child manifest
	if decode(body, &child) != nil || child.SchemaVersion != 2 || child.MediaType != mediaType || len(child.Manifests) != 0 || !sha256Digest.MatchString(child.Config.Digest) || child.Config.Size <= 0 || len(child.Layers) == 0 {
		return Candidate{}, errors.New("registry returned an invalid platform image manifest")
	}
	for _, layer := range child.Layers {
		if !sha256Digest.MatchString(layer.Digest) || layer.Size <= 0 {
			return Candidate{}, errors.New("registry returned an invalid image layer descriptor")
		}
	}
	return Candidate{Resolution: Resolution{Version: 1, Repository: Repository, Tag: tag, ResolvedAt: now().UTC(), Image: Repository + "@" + indexDigest, IndexDigest: indexDigest, ManifestDigest: digest, Platform: Platform}, Index: index, Manifest: body}, nil
}

func get(ctx context.Context, client *http.Client, address, token string, limit int64) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "", "", errors.New("invalid registry request")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", strings.Join([]string{ociIndex, dockerIndex, ociManifest, dockerManifest}, ", "))
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", "", ctx.Err()
		}
		return nil, "", "", errors.New("registry request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("registry returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", "", ctx.Err()
		}
		return nil, "", "", errors.New("registry response could not be read")
	}
	if int64(len(body)) > limit {
		return nil, "", "", errors.New("registry response exceeds size limit")
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return body, resp.Header.Get("Docker-Content-Digest"), mediaType, nil
}

func verifyDigest(body []byte, digest string) error {
	sum := sha256.Sum256(body)
	if !sha256Digest.MatchString(digest) || digest != "sha256:"+hex.EncodeToString(sum[:]) {
		return errors.New("registry digest does not match the exact response bytes")
	}
	return nil
}

func decode(body []byte, into any) error {
	if err := jsonvalue.Validate(body); err != nil {
		return errors.New("invalid registry JSON")
	}
	return json.Unmarshal(body, into)
}
