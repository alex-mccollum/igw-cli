package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestLegacyExecutionCoreRejectsDryRunWithoutDispatch(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	client := &gateway.Client{BaseURL: srv.URL, Token: "secret", HTTP: srv.Client()}
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		_, _, _, err := executeCallCore(client, callExecutionInput{Method: method, Path: "/mutation", Yes: true, DryRun: true, Timeout: time.Second})
		if igwerr.ExitCode(err) != 2 {
			t.Fatalf("%s accepted legacy dry-run: %v", method, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("legacy dry-run dispatched a request")
	}
}

func TestLegacyBatchCannotIgnoreTopLevelDryRun(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	c := newDoctorTestCLI(srv.Client(), new(bytes.Buffer))
	c.In = strings.NewReader(`{"method":"POST","path":"/mutation"}`)
	err := c.Execute([]string{"call", "--gateway-url", srv.URL, "--api-key", "secret", "--batch", "-", "--dry-run", "--yes", "--json"})
	if igwerr.ExitCode(err) != 2 || calls.Load() != 0 {
		t.Fatalf("batch ignored preview intent: %v, calls %d", err, calls.Load())
	}
}
