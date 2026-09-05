package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestJoinURLRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ base, path string }{
		{"https://gateway.test", "https://foreign.test/data"},
		{"https://gateway.test", "//foreign.test/data"},
		{"https://gateway.test", "http://gateway.test/data"},
		{"https://gateway.test", "https://gateway.test:8443/data"},
		{"https://gateway.test", "https://user:secret@gateway.test/data"},
		{"https://gateway.test", "https:opaque"},
		{"https://gateway.test", "/data#fragment"},
		{"https://user:secret@gateway.test", "/data"},
		{"https://gateway.test?token=secret", "/data"},
		{"file:///tmp/gateway", "/data"},
		{"gateway.test", "/data"},
	} {
		t.Run(tc.base+"/"+tc.path, func(t *testing.T) {
			_, err := JoinURL(tc.base, tc.path)
			if err == nil {
				t.Fatal("expected unsafe URL to be rejected")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("URL validation exposed credentials")
			}
		})
	}
}

func TestCallRejectsForeignURLBeforeTransport(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := &Client{BaseURL: "https://gateway.test", Token: "secret", HTTP: srv.Client()}
	_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: srv.URL, Timeout: time.Second})
	if igwerr.ExitCode(err) != 2 || calls.Load() != 0 {
		t.Fatalf("expected usage error without sending token, err=%v calls=%d", err, calls.Load())
	}
}

func TestCallBlocksForeignRedirect(t *testing.T) {
	t.Parallel()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var foreignCalls atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				foreignCalls.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer foreign.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, foreign.URL, status)
			}))
			defer source.Close()
			client := &Client{BaseURL: source.URL, Token: "secret", HTTP: source.Client()}
			_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/start", Timeout: time.Second})
			if igwerr.ExitCode(err) != 7 || foreignCalls.Load() != 0 {
				t.Fatalf("expected blocked redirect, err=%v foreign calls=%d", err, foreignCalls.Load())
			}
		})
	}
}

func TestCallPreservesSameOriginRedirectAndCallerPolicy(t *testing.T) {
	t.Parallel()
	var hookCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/done", http.StatusFound)
			return
		}
		if r.Header.Get(tokenHeader) != "secret" {
			t.Error("same-origin request lost token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	original := srv.Client()
	original.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		hookCalls.Add(1)
		return nil
	}
	client := &Client{BaseURL: srv.URL, Token: "secret", HTTP: original}
	if _, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/start", Timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	// Calling the original hook with a foreign URL still succeeds: we did not
	// replace its policy when constructing a Gateway-scoped client.
	foreign, _ := url.Parse("https://foreign.test")
	if err := original.CheckRedirect(&http.Request{URL: foreign}, nil); err != nil || hookCalls.Load() != 2 {
		t.Fatalf("caller redirect policy changed: %v", err)
	}
}

func TestRedirectHookCannotMoveRequestToForeignOrigin(t *testing.T) {
	t.Parallel()
	origin, _ := url.Parse("https://gateway.test")
	foreign, _ := url.Parse("https://foreign.test")
	client := clientForOrigin(&http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		req.URL = foreign
		return nil
	}}, origin)
	if err := client.CheckRedirect(&http.Request{URL: origin}, nil); err != http.ErrUseLastResponse {
		t.Fatalf("rewritten origin was accepted: %v", err)
	}
}

func TestOriginNormalizesDefaultPorts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ base, target string }{
		{"https://gateway.test", "https://GATEWAY.test:443/data"},
		{"http://[::1]", "http://[::1]:80/data"},
	} {
		if _, err := JoinURL(tc.base, tc.target); err != nil {
			t.Fatalf("equivalent origin rejected: %v", err)
		}
	}
}
