package nextcli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHeaderInputCLI(t *testing.T) {
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		if r.Header.Get("X-Test") != "\u00a0literal\u00a0" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("CLI changed header value or managed credential")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	app, out, _ := testApp(t, srv)
	args := []string{"api", "raw", "--method", "POST", "--path", "/headers", "--header", "X-Test:\u00a0literal\u00a0", "--json"}
	if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil || writes.Load() != 0 {
		t.Fatal("header preview failed or dispatched")
	}
	if decodeResult(t, out).Outcome != "preview" {
		t.Fatal("header preview did not report its outcome")
	}
	if err := app.Run(context.Background(), append(args, "--yes")); err != nil || !decodeResult(t, out).OK || writes.Load() != 1 {
		t.Fatal("header execution failed")
	}
}

func TestHeaderInputCLIRefusesBeforeDiscovery(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	for _, prefix := range [][]string{{"api", "raw", "--method", "POST", "--path", "/headers"}, {"api", "request", "POST /headers"}} {
		for _, pair := range []string{"X-Test:private-header-value\x00", "X-Test:\vprivate-header-value", "X-Test:private-header-value\x7f", "X-\tTest:private-header-value", "X-日本:private-header-value", "X-[]:private-header-value"} {
			for _, mode := range []string{"--dry-run", "--yes"} {
				args := append(append([]string{}, prefix...), "--header", pair, mode, "--json")
				err := app.Run(context.Background(), args)
				got := decodeResult(t, out)
				if err == nil || got.Error == nil || got.Error.Code != 2 || calls.Load() != 0 || strings.Contains(stderr.String(), "private-header-value") {
					t.Error("invalid header reached discovery/dispatch or changed its error contract")
				}
			}
		}
	}
}

func TestContentTypeInputCLIRefusesBeforeDiscovery(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	for _, prefix := range [][]string{{"api", "raw", "--method", "POST", "--path", "/headers"}, {"api", "request", "POST /headers"}} {
		for _, value := range []string{"private-header-value\x00", "text/plain\v", " \t", "text/plain\x7f"} {
			for _, mode := range []string{"--dry-run", "--yes"} {
				err := app.Run(context.Background(), append(append([]string{}, prefix...), "--content-type", value, mode, "--json"))
				output := out.String() + stderr.String()
				got := decodeResult(t, out)
				if err == nil || got.Error == nil || got.Error.Code != 2 || calls.Load() != 0 || strings.Contains(output, "private-header-value") {
					t.Error("invalid media header passed preview or reached discovery/dispatch")
				}
			}
		}
	}
}
