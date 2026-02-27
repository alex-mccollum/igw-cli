package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestJoinURL(t *testing.T) {
	t.Parallel()

	got, err := JoinURL("http://127.0.0.1:8088/", "/data/api/v1/gateway-info")
	if err != nil {
		t.Fatalf("join url: %v", err)
	}

	want := "http://127.0.0.1:8088/data/api/v1/gateway-info"
	if got != want {
		t.Fatalf("unexpected url: got %q want %q", got, want)
	}
}

func TestCallBuildsRequest(t *testing.T) {
	t.Parallel()

	var (
		gotMethod      string
		gotPath        string
		gotQuery       string
		gotTokenHeader string
		gotCustom      string
		gotBody        string
		gotContentType string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotTokenHeader = r.Header.Get(tokenHeader)
		gotCustom = r.Header.Get("X-Test")
		gotContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &Client{
		BaseURL: srv.URL,
		Token:   "secret-token",
		HTTP:    srv.Client(),
	}

	resp, err := client.Call(context.Background(), CallRequest{
		Method:      http.MethodPost,
		Path:        "/data/api/v1/scan/projects",
		Query:       []string{"dryRun=true"},
		Headers:     []string{"X-Test: abc"},
		Body:        []byte(`{"scan":true}`),
		ContentType: "application/json",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method: got %q", gotMethod)
	}
	if gotPath != "/data/api/v1/scan/projects" {
		t.Fatalf("path: got %q", gotPath)
	}
	if gotQuery != "dryRun=true" {
		t.Fatalf("query: got %q", gotQuery)
	}
	if gotTokenHeader != "secret-token" {
		t.Fatalf("token header missing")
	}
	if gotCustom != "abc" {
		t.Fatalf("custom header: got %q", gotCustom)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type: got %q", gotContentType)
	}
	if gotBody != `{"scan":true}` {
		t.Fatalf("body: got %q", gotBody)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response status: got %d", resp.StatusCode)
	}
	if strings.TrimSpace(string(resp.Body)) != `{"ok":true}` {
		t.Fatalf("response body: got %q", string(resp.Body))
	}
}

func TestCallAuthStatusMapsToAuthExitCode(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "nope", status)
			}))
			defer srv.Close()

			client := &Client{
				BaseURL: srv.URL,
				Token:   "secret-token",
				HTTP:    srv.Client(),
			}

			_, err := client.Call(context.Background(), CallRequest{
				Method:  http.MethodGet,
				Path:    "/data/api/v1/gateway-info",
				Timeout: time.Second,
			})
			if err == nil {
				t.Fatalf("expected error")
			}

			if code := igwerr.ExitCode(err); code != 6 {
				t.Fatalf("exit code: got %d want 6", code)
			}
		})
	}
}

func TestCallRetriesOnServerError(t *testing.T) {
	t.Parallel()

	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 2 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &Client{
		BaseURL: srv.URL,
		Token:   "secret-token",
		HTTP:    srv.Client(),
	}

	resp, err := client.Call(context.Background(), CallRequest{
		Method:       http.MethodGet,
		Path:         "/data/api/v1/gateway-info",
		Timeout:      time.Second,
		Retry:        1,
		RetryBackoff: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("call with retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

func TestRetryDelayForResponseUsesFallbackWhenUnavailable(t *testing.T) {
	t.Parallel()

	fallback := 250 * time.Millisecond
	now := time.Unix(1700000000, 0).UTC()

	if got := retryDelayForResponse(http.StatusInternalServerError, http.Header{}, fallback, now); got != fallback {
		t.Fatalf("expected fallback for non-429 status, got %s", got)
	}
	if got := retryDelayForResponse(http.StatusTooManyRequests, http.Header{}, fallback, now); got != fallback {
		t.Fatalf("expected fallback when Retry-After header missing, got %s", got)
	}
	if got := retryDelayForResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"invalid"}}, fallback, now); got != fallback {
		t.Fatalf("expected fallback for invalid Retry-After header, got %s", got)
	}
}

func TestRetryDelayForResponseParsesRetryAfterSeconds(t *testing.T) {
	t.Parallel()

	fallback := 250 * time.Millisecond
	now := time.Unix(1700000000, 0).UTC()

	got := retryDelayForResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{"2"}}, fallback, now)
	if got != 2*time.Second {
		t.Fatalf("expected 2s retry delay, got %s", got)
	}
}

func TestRetryDelayForResponseParsesRetryAfterDate(t *testing.T) {
	t.Parallel()

	fallback := 250 * time.Millisecond
	now := time.Unix(1700000000, 0).UTC()
	target := now.Add(3 * time.Second)
	header := target.Format(http.TimeFormat)

	got := retryDelayForResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{header}}, fallback, now)
	if got != 3*time.Second {
		t.Fatalf("expected 3s retry delay from date header, got %s", got)
	}

	pastHeader := now.Add(-time.Second).Format(http.TimeFormat)
	got = retryDelayForResponse(http.StatusTooManyRequests, http.Header{"Retry-After": []string{pastHeader}}, fallback, now)
	if got != 0 {
		t.Fatalf("expected zero retry delay when Retry-After date is in the past, got %s", got)
	}
}
