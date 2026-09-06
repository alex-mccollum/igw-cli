package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func profileApp(t *testing.T) (App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	a, out, stderr := testApp(t, nil)
	a.ReadConfig = nil
	a.ConfigStore = &config.Store{Dir: filepath.Join(t.TempDir(), "profiles")}
	a.Getenv = func(string) string { t.Fatal("profile writes read environment"); return "" }
	a.HTTP = &http.Client{Transport: profileNoNetwork{t}}
	return a, out, stderr
}

type profileNoNetwork struct{ t *testing.T }

func (p profileNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	p.t.Fatal("profile command contacted a Gateway")
	return nil, fmt.Errorf("unexpected network")
}

func runProfile(t *testing.T, a App, out, stderr *bytes.Buffer, input string, args ...string) result.Result {
	t.Helper()
	out.Reset()
	stderr.Reset()
	a.In = strings.NewReader(input)
	err := a.Run(context.Background(), append(args, "--json"))
	r := decodeResult(t, out)
	if (err == nil) != r.OK {
		t.Fatal("exit and result disagree")
	}
	if stderr.Len() != 0 || strings.Contains(out.String(), "private-token") {
		t.Fatal("credential or unstructured output escaped")
	}
	return r
}

func TestProfileCLISetupAndExplicitEdits(t *testing.T) {
	a, out, stderr := profileApp(t)
	args := []string{"profile", "set", "dev", "--url", "https://gateway.invalid/proxy", "--use", "--token-stdin"}
	preview := runProfile(t, a, out, stderr, "private-token\n", append(args, "--dry-run")...)
	if !preview.OK || preview.Outcome != "preview" {
		t.Fatalf("preview: %+v", preview)
	}
	if preview.Data.(map[string]any)["tokenChange"] != "set" {
		t.Fatal("token action missing from preview")
	}
	if _, err := os.Stat(a.ConfigStore.Dir); !os.IsNotExist(err) {
		t.Fatal("preview created config directory")
	}
	applied := runProfile(t, a, out, stderr, "private-token\n", append(args, "--yes")...)
	if !applied.OK {
		t.Fatalf("setup: %+v", applied)
	}
	state, err := a.ConfigStore.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Config.ActiveProfile != "dev" || state.Config.Profiles["dev"].Token != "private-token" {
		t.Fatal("setup did not store explicit fields")
	}
	listed := runProfile(t, a, out, stderr, "", "profile", "list")
	data := listed.Data.(map[string]any)
	if data["source"] != "v1" || data["revision"] != state.Revision || data["active"] != "dev" || len(data["profiles"].([]any)) != 1 {
		t.Fatal("list does not expose reviewed revision")
	}
	a.Getenv = func(name string) string {
		if name == config.EnvGatewayURL {
			return "https://env.invalid/base"
		}
		return "private-env"
	}
	show := runProfile(t, a, out, stderr, "", "profile", "show", "--gateway-url", "https://flag.invalid")
	if !show.OK || show.Data.(map[string]any)["target"].(map[string]any)["url"] != "https://flag.invalid" {
		t.Fatal("runtime precedence changed")
	}
	a.Getenv = func(string) string { t.Fatal("edit imported environment"); return "" }
	cleared := runProfile(t, a, out, stderr, "", "profile", "set", "dev", "--clear-token", "--if-revision", state.Revision, "--yes")
	if !cleared.OK {
		t.Fatal("clear token failed")
	}
	current, _ := a.ConfigStore.Read()
	if current.Config.Profiles["dev"].Token != "" || current.Config.Profiles["dev"].GatewayURL != "https://gateway.invalid/proxy" {
		t.Fatal("edit persisted runtime overrides")
	}
	stale := runProfile(t, a, out, stderr, "", "profile", "use", "--default", "--if-revision", state.Revision, "--yes")
	if stale.OK || stale.Error.Code != 2 {
		t.Fatal("stale revision accepted")
	}
	for _, command := range [][]string{{"profile", "use", "--default", "--yes"}, {"profile", "remove", "dev", "--yes"}} {
		if r := runProfile(t, a, out, stderr, "", command...); !r.OK {
			t.Fatal("profile edit failed")
		}
	}
}

func TestProfileCLIRejectsUnsafeOrAmbiguousInputs(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		args        []string
	}{
		{"confirmation", "", []string{"profile", "set", "dev", "--url", "https://gateway.invalid"}},
		{"both confirmation", "", []string{"profile", "set", "dev", "--url", "https://gateway.invalid", "--yes", "--dry-run"}},
		{"empty stdin", "\n", []string{"profile", "set", "dev", "--token-stdin", "--yes"}},
		{"oversized stdin", strings.Repeat("x", (16<<10)+1), []string{"profile", "set", "dev", "--token-stdin", "--yes"}},
		{"newline token", "private-token\nheader:value", []string{"profile", "set", "dev", "--token-stdin", "--yes"}},
		{"invalid unicode", "\xff", []string{"profile", "set", "dev", "--token-stdin", "--yes"}},
		{"token conflict", "private-token", []string{"profile", "set", "dev", "--token-stdin", "--clear-token", "--yes"}},
		{"token argv", "", []string{"profile", "set", "dev", "--token", "anything", "--yes"}},
		{"runtime URL", "", []string{"profile", "set", "dev", "--gateway-url", "https://gateway.invalid", "--yes"}},
		{"runtime profile", "", []string{"profile", "use", "dev", "--profile", "other", "--yes"}},
		{"URL credentials", "", []string{"profile", "set", "dev", "--url", "https://user:private-token@gateway.invalid", "--yes"}},
		{"URL query", "", []string{"profile", "set", "dev", "--url", "https://gateway.invalid?token=private-token", "--yes"}},
		{"default conflict", "", []string{"profile", "use", "dev", "--default", "--yes"}},
		{"empty change", "", []string{"profile", "set", "dev", "--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, out, stderr := profileApp(t)
			r := runProfile(t, a, out, stderr, tc.input, tc.args...)
			if r.OK || r.Error.Code != 2 {
				t.Fatalf("expected usage refusal: %+v", r)
			}
			if _, err := os.Stat(a.ConfigStore.Dir); !os.IsNotExist(err) {
				t.Fatal("invalid input created configuration")
			}
		})
	}
}

func TestProfileCLIMigrationAndRollback(t *testing.T) {
	a, out, stderr := profileApp(t)
	if err := os.MkdirAll(a.ConfigStore.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"gatewayURL":"https://gateway.invalid","token":"private-token"}`)
	path := filepath.Join(a.ConfigStore.Dir, config.LegacyFilename)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"profile", "migrate", "--dry-run"}, {"profile", "migrate", "--yes"}} {
		if r := runProfile(t, a, out, stderr, "", args...); !r.OK {
			t.Fatalf("migration: %+v", r)
		}
	}
	state, _ := a.ConfigStore.Read()
	for _, mode := range []string{"--dry-run", "--yes"} {
		if r := runProfile(t, a, out, stderr, "", "profile", "rollback", "--if-revision", state.Revision, mode); !r.OK {
			t.Fatalf("rollback: %+v", r)
		}
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, legacy) {
		t.Fatal("legacy file was changed")
	}
}

func TestConfiguredProfileDrivesTargetBoundHTTP(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("stored token not used")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/proxy/openapi.json":
			fmt.Fprint(w, fixtureSpec)
		case "/proxy/data/api/v1/gateway-info":
			fmt.Fprint(w, `{"status":"fixture-ok"}`)
		default:
			t.Error("unexpected target path")
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	a, out, stderr := profileApp(t)
	setup := runProfile(t, a, out, stderr, "private-token", "profile", "set", "dev", "--url", srv.URL+"/proxy", "--token-stdin", "--use", "--yes")
	if !setup.OK || requests.Load() != 0 {
		t.Fatal("setup contacted server or failed")
	}
	a.HTTP = srv.Client()
	a.Getenv = func(string) string { return "" }
	r := runProfile(t, a, out, stderr, "", "api", "request", "GET /data/api/v1/gateway-info")
	if !r.OK || requests.Load() != 2 || r.Data.(map[string]any)["status"] != "fixture-ok" {
		t.Fatalf("configured runtime: %+v (%d requests)", r, requests.Load())
	}
}

func TestProfileTokenDeadlineAndCancellation(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		a, out, stderr := profileApp(t)
		reader, writer := io.Pipe()
		a.In = reader
		ctx, cancel := context.WithCancel(context.Background())
		if cancelFirst {
			cancel()
		}
		start := time.Now()
		err := a.Run(ctx, []string{"profile", "set", "dev", "--token-stdin", "--yes", "--timeout", "10ms", "--json"})
		cancel()
		reader.Close()
		writer.Close()
		got := decodeResult(t, out)
		if err == nil || got.Error.Code != 7 || time.Since(start) > time.Second || stderr.Len() != 0 {
			t.Fatalf("stdin cancellation: %+v", got)
		}
		kind := "timeout"
		if cancelFirst {
			kind = "canceled"
		}
		if got.Error.Kind != kind {
			t.Fatalf("wrong cancellation kind: %+v", got)
		}
		if _, err = os.Stat(a.ConfigStore.Dir); !os.IsNotExist(err) {
			t.Fatal("canceled token read wrote configuration")
		}
	}
}

type forbiddenTokenRead struct{ t *testing.T }

func (r forbiddenTokenRead) Read([]byte) (int, error) {
	r.t.Fatal("read stdin before confirmation")
	return 0, io.EOF
}

func TestProfileCLIConfirmationAndDiscoveryDoNotReadInputs(t *testing.T) {
	a, out, stderr := profileApp(t)
	a.In = forbiddenTokenRead{t}
	if err := a.Run(context.Background(), []string{"profile", "set", "dev", "--token-stdin", "--json"}); err == nil {
		t.Fatal("missing confirmation accepted")
	}
	out.Reset()
	stderr.Reset()
	a.ReadConfig = func() (config.File, error) { t.Fatal("schema read configuration"); return config.File{}, nil }
	if err := a.Run(context.Background(), []string{"schema", "--json"}); err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"token-stdin", "if-revision", "migrate", "rollback", "clear-token"} {
		if !strings.Contains(out.String(), name) {
			t.Fatal("profile command missing from generated schema")
		}
	}
}

func TestProfileShellCompletionUsesOnlyLocalNames(t *testing.T) {
	a, out, stderr := profileApp(t)
	for _, name := range []string{"dev-b", "dev-a", "prod"} {
		if r := runProfile(t, a, out, stderr, "private-token", "profile", "set", name, "--url", "https://gateway.invalid", "--token-stdin", "--yes"); !r.OK {
			t.Fatal("setup failed")
		}
	}
	for _, args := range [][]string{
		{"__complete", "--profile", "dev-"},
		{"__complete", "profile", "use", "dev-"},
		{"__complete", "profile", "set", "dev-"},
		{"__complete", "profile", "remove", "dev-"},
	} {
		out.Reset()
		stderr.Reset()
		if err := a.Run(context.Background(), args); err != nil {
			t.Fatal(err)
		}
		if out.String() != "dev-a\ndev-b\n:4\n" {
			t.Fatalf("completion: %q", out.String())
		}
		if strings.Contains(stderr.String(), "private-token") || strings.Contains(stderr.String(), "gateway.invalid") {
			t.Fatal("completion exposed profile fields")
		}
	}
	out.Reset()
	stderr.Reset()
	if err := a.Run(context.Background(), []string{"__complete", "profile", "use", "dev-a", ""}); err != nil {
		t.Fatal(err)
	}
	if out.String() != ":4\n" {
		t.Fatal("completion offered a second profile argument")
	}
}
