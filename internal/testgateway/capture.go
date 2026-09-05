package testgateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

type Evidence struct {
	Version         int                    `json:"version"`
	Image           string                 `json:"image"`
	GatewayVersion  string                 `json:"gatewayVersion,omitempty"`
	ModuleWhitelist []string               `json:"moduleWhitelist,omitempty"`
	Source          string                 `json:"source"`
	CapturedAt      time.Time              `json:"capturedAt"`
	RawSHA256       string                 `json:"rawSha256"`
	ContractSHA256  string                 `json:"contractSha256,omitempty"`
	Operations      int                    `json:"operations,omitempty"`
	ParserVersion   string                 `json:"parserVersion"`
	Validated       bool                   `json:"validated"`
	Compatibility   *catalog.Compatibility `json:"compatibility,omitempty"`
	ValidationError string                 `json:"validationError,omitempty"`
}

// WaitOpenAPI waits for RUNNING and three identical JSON contracts. This avoids
// capturing the first partial route set while modules are still starting.
func (s *Session) WaitOpenAPI(ctx context.Context) ([]byte, error) {
	client := s.HTTPClient()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous []byte
	stable := 0
	loggedIn := false
	last := "Gateway has not responded"
	for {
		status, err := readURL(ctx, client, s.URL+"/StatusPing", 4096)
		var health struct {
			State string `json:"state"`
		}
		if err == nil && json.Unmarshal(status, &health) == nil && health.State == "RUNNING" {
			if !loggedIn {
				if err := s.login(ctx, client); err != nil {
					return nil, err
				}
				info, _, err := sessionRequest(ctx, client, http.MethodGet, s.URL+"/data/api/v1/gateway-info", nil, nil)
				if err != nil {
					return nil, err
				}
				var gatewayInfo struct {
					IgnitionVersion string `json:"ignitionVersion"`
				}
				if json.Unmarshal(info, &gatewayInfo) != nil || gatewayInfo.IgnitionVersion == "" {
					return nil, errors.New("capture could not verify the running Gateway version")
				}
				s.GatewayVersion = gatewayInfo.IgnitionVersion
				loggedIn = true
			}
			raw, err := readURL(ctx, client, s.URL+"/openapi.json", catalog.MaxDocumentBytes)
			if err == nil {
				var root struct {
					OpenAPI string                     `json:"openapi"`
					Paths   map[string]json.RawMessage `json:"paths"`
				}
				if json.Unmarshal(raw, &root) == nil && root.OpenAPI != "" && len(root.Paths) > 0 {
					if bytes.Equal(raw, previous) {
						stable++
					} else {
						stable = 1
						previous = raw
					}
					if stable >= 3 {
						return raw, nil
					}
					last = "waiting for stable module routes"
				} else {
					last = "OpenAPI endpoint did not return a document"
					stable = 0
				}
			} else {
				last = err.Error()
				stable = 0
			}
		} else if err != nil {
			last = err.Error()
			stable = 0
		} else {
			last = "waiting for Gateway RUNNING state"
			stable = 0
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("Gateway capture timed out (%s): %w", last, ctx.Err())
		case <-ticker.C:
		}
	}
}

func readURL(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Gateway is not reachable yet")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Gateway returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("capture response exceeds size limit")
	}
	return b, nil
}

// Save preserves original bytes even if the CLI parser rejects a vendor
// document. Such evidence is explicitly unvalidated and must not be bundled as
// a qualified catalog until the incompatibility has been reviewed and repaired.
func (s *Session) Save(dir string, raw []byte) (Evidence, error) {
	sum := sha256.Sum256(raw)
	evidence := Evidence{Version: 1, Image: s.Image, GatewayVersion: s.GatewayVersion, ModuleWhitelist: s.Modules, Source: "/openapi.json", CapturedAt: time.Now().UTC(), RawSHA256: hex.EncodeToString(sum[:]), ParserVersion: catalog.ParserVersion}
	if err := saveFile(filepath.Join(dir, "openapi.json"), raw); err != nil {
		return evidence, err
	}
	c, parseErr := catalog.Parse(raw)
	if parseErr != nil {
		evidence.ValidationError = parseErr.Error()
	} else {
		defer c.Close()
		evidence.Validated = true
		evidence.Compatibility = c.Compatibility()
		evidence.ContractSHA256 = c.ContractHash()
		evidence.Operations = len(c.Operations())
	}
	manifest, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return evidence, err
	}
	if err := saveFile(filepath.Join(dir, "capture.json"), append(manifest, '\n')); err != nil {
		return evidence, err
	}
	return evidence, parseErr
}

func saveFile(path string, data []byte) error {
	w, err := artifact.New(path, false)
	if err != nil {
		return err
	}
	_, writeErr := w.Write(data)
	if writeErr == nil {
		_, writeErr = w.Commit()
	}
	return errors.Join(writeErr, w.Abort())
}
