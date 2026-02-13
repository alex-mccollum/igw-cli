package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestDoctorSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"gateway"}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--timeout", "1s",
	})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "ok\tgateway_info\tstatus 200") {
		t.Fatalf("missing gateway_info success check: %q", got)
	}
}

func TestDoctorAuthFailureMapsToAuthExitCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--timeout", "1s",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}

	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("exit code: got %d want 6", code)
	}

	if !strings.Contains(out.String(), "hint: 403 indicates permission mapping or secure-connection restrictions") {
		t.Fatalf("expected 403 hint in output, got %q", out.String())
	}
}

func TestDoctorJSONIncludesHint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/data/api/v1/gateway-info" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	var out bytes.Buffer

	c := &CLI{
		In:     strings.NewReader(""),
		Out:    &out,
		Err:    new(bytes.Buffer),
		Getenv: func(string) string { return "" },
		ReadConfig: func() (config.File, error) {
			return config.File{}, nil
		},
		HTTPClient: srv.Client(),
	}

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--timeout", "1s",
		"--json",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}

	var payload struct {
		Checks []struct {
			Name string `json:"name"`
			Hint string `json:"hint"`
		} `json:"checks"`
	}
	if unmarshalErr := json.Unmarshal(out.Bytes(), &payload); unmarshalErr != nil {
		t.Fatalf("parse doctor json: %v", unmarshalErr)
	}

	var gatewayHint string
	for _, check := range payload.Checks {
		if check.Name == "gateway_info" {
			gatewayHint = check.Hint
			break
		}
	}
	if gatewayHint == "" {
		t.Fatalf("expected gateway_info hint in json output: %s", out.String())
	}
}

func TestDoctorHintForTimeoutTransportError(t *testing.T) {
	t.Parallel()

	hint := doctorHintForError(&igwerr.TransportError{Timeout: true})
	if !strings.Contains(hint, "WSL2 -> Windows") {
		t.Fatalf("unexpected timeout hint: %q", hint)
	}
}
