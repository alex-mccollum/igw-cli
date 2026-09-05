package testgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const qualificationTokenName = "igw-qualification"
const qualificationSecurityLevel = "igwQualification"

// ProvisionAPIToken bootstraps a token only on this fresh disposable Gateway.
// The returned credential stays in the test process; never print or persist it.
// This private browser/session adapter is not part of the released CLI.
func (s *Session) ProvisionAPIToken(ctx context.Context) (string, error) {
	s.closeMu.Lock()
	active := s.created && !s.closed && s.owner != "" && containerID.MatchString(s.ID)
	s.closeMu.Unlock()
	u, err := url.Parse(s.URL)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Scheme != "http" || !net.ParseIP(u.Hostname()).IsLoopback() || !active {
		return "", errors.New("API token bootstrap requires this session's fresh loopback Gateway")
	}
	client := s.HTTPClient()
	if err := s.login(ctx, client); err != nil {
		return "", err
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return s.provisionAPIToken(ctx, client)
}

func (s *Session) provisionAPIToken(ctx context.Context, client *http.Client) (string, error) {
	raw, _, err := sessionRequest(ctx, client, http.MethodGet, s.URL+"/data/app/session", nil, nil)
	if err != nil {
		return "", err
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if json.Unmarshal(raw, &session) != nil || session.CSRFToken == "" {
		return "", errors.New("test Gateway did not provide a CSRF token")
	}
	headers := http.Header{"X-Csrf-Token": []string{session.CSRFToken}}
	if err := s.configureTokenPermissions(ctx, client, headers); err != nil {
		return "", err
	}
	raw, _, err = sessionRequest(ctx, client, http.MethodPost, s.URL+"/data/api/v1/api-token/generate", nil, headers)
	if err != nil {
		return "", err
	}
	var generated struct {
		Key  string `json:"key"`
		Hash string `json:"hash"`
	}
	if json.Unmarshal(raw, &generated) != nil || generated.Key == "" || generated.Hash == "" || strings.ContainsAny(generated.Key, "\r\n\t ") {
		return "", errors.New("test Gateway returned an invalid API token pair")
	}
	input := []any{map[string]any{
		"name": qualificationTokenName, "collection": "core", "enabled": true,
		"config": map[string]any{
			"profile": map[string]any{
				"type": "basic-token", "secureChannelRequired": false, "timestamp": time.Now().UnixMilli(),
				"securityLevels": []any{map[string]any{"name": qualificationSecurityLevel, "children": []any{}}},
			},
			"settings": map[string]string{"tokenHash": generated.Hash},
		},
	}}
	raw, _, err = sessionRequest(ctx, client, http.MethodPost, s.URL+"/data/api/v1/resources/ignition/api-token", input, headers)
	if err != nil {
		return "", err
	}
	var response struct {
		Success bool `json:"success"`
	}
	if json.Unmarshal(raw, &response) != nil || !response.Success {
		return "", errors.New("test Gateway did not confirm API token creation")
	}
	// The generator returns only the secret component. Ignition's header
	// credential also identifies the API-token resource by name.
	return qualificationTokenName + ":" + generated.Key, nil
}

// API keys use a dedicated security level; they do not impersonate the built-in
// Administrator role. Extend only the fresh container's observed AnyOf policies
// and preserve the administrator branch and all unrelated settings. Never grant
// public read/write access. Every singleton update uses its observed signature.
func (s *Session) configureTokenPermissions(ctx context.Context, client *http.Client, headers http.Header) error {
	for _, kind := range []string{"security-levels", "security-properties"} {
		raw, _, err := sessionRequest(ctx, client, http.MethodGet, s.URL+"/data/api/v1/resources/singleton/ignition/"+kind+"?collection=core", nil, nil)
		if err != nil {
			return err
		}
		var current struct {
			Signature  string                     `json:"signature"`
			Collection string                     `json:"collection"`
			Config     map[string]json.RawMessage `json:"config"`
		}
		if json.Unmarshal(raw, &current) != nil || current.Signature == "" || current.Collection != "core" || current.Config == nil {
			return errors.New("cannot verify fresh Gateway security configuration")
		}
		if kind == "security-levels" {
			var levels []json.RawMessage
			if json.Unmarshal(current.Config["securityLevels"], &levels) != nil {
				return errors.New("unexpected fresh Gateway security-level tree")
			}
			for _, raw := range levels {
				var level struct{ Name string }
				if json.Unmarshal(raw, &level) != nil || level.Name == qualificationSecurityLevel {
					return errors.New("test security level already exists or is invalid")
				}
			}
			level, _ := json.Marshal(map[string]any{"name": qualificationSecurityLevel, "children": []any{}})
			current.Config["securityLevels"], _ = json.Marshal(append(levels, level))
		} else {
			for _, field := range []string{"readPermissions", "writePermissions"} {
				current.Config[field], err = extendTestPermission(current.Config[field])
				if err != nil {
					return err
				}
			}
		}
		body := []any{map[string]any{"collection": "core", "signature": current.Signature, "config": current.Config}}
		raw, _, err = sessionRequest(ctx, client, http.MethodPut, s.URL+"/data/api/v1/resources/ignition/"+kind, body, headers)
		if err != nil {
			return err
		}
		var response struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(raw, &response) != nil || !response.Success {
			return errors.New("test Gateway did not confirm scoped token permissions")
		}
	}
	return nil
}

func extendTestPermission(raw json.RawMessage) (json.RawMessage, error) {
	var policy map[string]json.RawMessage
	var kind string
	var levels []json.RawMessage
	if json.Unmarshal(raw, &policy) != nil || json.Unmarshal(policy["type"], &kind) != nil || kind != "AnyOf" || json.Unmarshal(policy["securityLevels"], &levels) != nil || len(levels) == 0 {
		return nil, errors.New("test bootstrap requires the fresh Gateway's non-public AnyOf permission policy")
	}
	for _, raw := range levels {
		var level struct{ Name string }
		if json.Unmarshal(raw, &level) != nil || level.Name == "" || level.Name == "Public" {
			return nil, errors.New("test bootstrap refuses a public or invalid permission level")
		}
	}
	level, _ := json.Marshal(map[string]any{"name": qualificationSecurityLevel, "children": []any{}})
	policy["securityLevels"], _ = json.Marshal(append(levels, level))
	return json.Marshal(policy)
}
