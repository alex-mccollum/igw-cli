package cli

import (
	"bytes"
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
		switch r.URL.Path {
		case "/data/api/v1/gateway-info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"gateway"}`))
		case "/data/api/v1/scan/projects":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	c := newDoctorTestCLI(srv.Client(), &out)

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
	if !strings.Contains(got, "ok\tscan_projects\tskipped (use --check-write)") {
		t.Fatalf("missing default skipped write check: %q", got)
	}
}

func TestDoctorCheckWriteEnabled(t *testing.T) {
	t.Parallel()

	var sawWriteCheck bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/api/v1/gateway-info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"gateway"}`))
		case "/data/api/v1/scan/projects":
			sawWriteCheck = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newDoctorTestCLI(srv.Client(), nil)

	if err := c.Execute([]string{
		"doctor",
		"--gateway-url", srv.URL,
		"--api-key", "secret",
		"--check-write",
	}); err != nil {
		t.Fatalf("doctor with write check failed: %v", err)
	}

	if !sawWriteCheck {
		t.Fatalf("expected write check call to scan/projects")
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
	c := newDoctorTestCLI(srv.Client(), &out)

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

func TestDoctorHintForTimeoutTransportError(t *testing.T) {
	t.Parallel()

	hint := doctorHintForError(&igwerr.TransportError{Timeout: true})
	if !strings.Contains(hint, "WSL2 -> Windows") {
		t.Fatalf("unexpected timeout hint: %q", hint)
	}
}

func newDoctorTestCLI(httpClient *http.Client, out *bytes.Buffer) *CLI {
	if out == nil {
		out = new(bytes.Buffer)
	}

	return &CLI{
		In:         strings.NewReader(""),
		Out:        out,
		Err:        new(bytes.Buffer),
		Getenv:     func(string) string { return "" },
		ReadConfig: func() (config.File, error) { return config.File{}, nil },
		HTTPClient: httpClient,
	}
}

func requireDoctorUsageError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := igwerr.ExitCode(err); code != 2 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
