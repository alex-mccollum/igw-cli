package testgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

// login uses the fresh Gateway's built-in IdP to provision the test session.
// These private browser routes are intentionally confined to disposable test
// Gateways; the released CLI authenticates with API tokens, never this flow.
func (s *Session) login(ctx context.Context, client *http.Client) error {
	base, err := url.Parse(s.URL)
	if err != nil || s.password == "" {
		return errors.New("test Gateway credentials are unavailable")
	}
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host {
			return errors.New("test Gateway login redirected outside its origin or exceeded limit")
		}
		return nil
	}
	_, location, err := sessionRequest(ctx, client, http.MethodGet, s.URL+"/data/app/login", nil, nil)
	if err != nil {
		return err
	}
	if location.Path != "/idp/default/authn/login" {
		return errors.New("test Gateway did not offer its default IdP login")
	}
	query := location.Query()
	if query.Get("token") == "" {
		return errors.New("test Gateway did not provide a login challenge")
	}
	type challenge struct {
		Token    string `json:"token"`
		Success  bool   `json:"success"`
		Complete bool   `json:"complete"`
	}
	var next challenge
	post := func(path string, body any) error {
		raw, _, err := sessionRequest(ctx, client, http.MethodPost, s.URL+path, body, nil)
		if err != nil {
			return err
		}
		next = challenge{}
		if json.Unmarshal(raw, &next) != nil || next.Token == "" {
			return errors.New("test Gateway returned an invalid authentication challenge")
		}
		return nil
	}
	if err := post("/idp/default/authn/next-challenge", map[string]any{"token": query.Get("token")}); err != nil {
		return err
	}
	if err := post("/idp/default/authn/submit-challenge/basic", map[string]any{"token": next.Token, "rememberMe": false, "challenge": map[string]string{"username": "admin", "password": s.password}}); err != nil {
		return err
	}
	if !next.Success {
		return errors.New("test Gateway rejected temporary admin credentials")
	}
	if err := post("/idp/default/authn/next-challenge", map[string]any{"token": next.Token}); err != nil {
		return err
	}
	if !next.Complete {
		return errors.New("test Gateway requires an unsupported additional authentication challenge")
	}
	query.Set("token", next.Token)
	if _, _, err := sessionRequest(ctx, client, http.MethodGet, s.URL+"/idp/default/oidc/auth?"+query.Encode(), nil, nil); err != nil {
		return err
	}
	return nil
}

// sessionRequest never exposes URL query tokens, response bodies, or transport
// error strings. It bounds authentication responses and follows only the login
// client's explicit same-origin redirect policy.
func sessionRequest(ctx context.Context, client *http.Client, method, address string, body any, headers http.Header) ([]byte, *url.URL, error) {
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, nil, errors.New("cannot encode test Gateway request")
		}
		input = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, address, input)
	if err != nil {
		return nil, nil, errors.New("invalid test Gateway request")
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, errors.New("test Gateway request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("test Gateway request returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, nil, errors.New("invalid or oversized test Gateway response")
	}
	return raw, resp.Request.URL, nil
}
