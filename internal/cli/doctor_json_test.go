package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

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
	c := newDoctorTestCLI(srv.Client(), &out)

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

func TestDoctorJSONSelectRawExtractionSuccess(t *testing.T) {
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
	c := newDoctorTestCLI(srv.Client(), &out)

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "checks.0.name",
		"--raw",
	})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if out.String() != "gateway_url\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
}

func TestDoctorJSONSelectRawExtractionFromErrorEnvelope(t *testing.T) {
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
	c := newDoctorTestCLI(srv.Client(), &out)

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "code",
		"--raw",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}
	if out.String() != "6\n" {
		t.Fatalf("unexpected field output %q", out.String())
	}
}

func TestDoctorJSONSelectSubsetFromErrorEnvelope(t *testing.T) {
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
	c := newDoctorTestCLI(srv.Client(), &out)

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "code",
		"--select", "error",
	})
	if err == nil {
		t.Fatalf("expected auth failure")
	}
	if code := igwerr.ExitCode(err); code != 6 {
		t.Fatalf("unexpected exit code %d", code)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if int(payload["code"].(float64)) != 6 {
		t.Fatalf("unexpected subset code %#v", payload["code"])
	}
	if !strings.Contains(payload["error"].(string), "http 403") {
		t.Fatalf("unexpected subset error %#v", payload["error"])
	}
}

func TestDoctorJSONSelectSubsetExtractionSuccess(t *testing.T) {
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
	c := newDoctorTestCLI(srv.Client(), &out)

	err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--json",
		"--select", "ok",
		"--select", "checks.0.name",
	})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode fields json: %v", decodeErr)
	}
	if payload["ok"] != true {
		t.Fatalf("unexpected ok value %#v", payload["ok"])
	}
	if payload["checks.0.name"] != "gateway_url" {
		t.Fatalf("unexpected checks.0.name %#v", payload["checks.0.name"])
	}
}
