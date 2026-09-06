package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestAPIJSONBodyKeepsExactWireBytes(t *testing.T) {
	const spec = `{"openapi":"3.1.0","info":{"title":"Synthetic exact request body","version":"test"},"paths":{"/body/{id}":{"parameters":[{"in":"path","name":"id","required":true,"schema":{"type":"string"}}],"post":{"parameters":[{"in":"query","name":"enabled","required":true,"schema":{"type":"boolean","const":false}},{"in":"header","name":"If-Match","required":true,"schema":{"type":"string"}}],"requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["amount","samples","text","generated"],"properties":{"amount":{"type":"number","const":9007199254740993},"samples":{"type":"array","minItems":2,"uniqueItems":true,"items":{"type":"integer"}},"text":{"type":"string"},"generated":{"type":"integer","readOnly":true}}}}}},"responses":{"200":{"description":"OK"}}}}}}`
	const body = "{\n  \"amount\": 9007199254740993, \"samples\": [9007199254740992, 9007199254740993],\n  \"text\": \" private-body-text + & % # / = 日本 \"\n}\n"
	var writes atomic.Int32
	var received atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/proxy/openapi.json" {
			_, _ = io.WriteString(w, spec)
			return
		}
		writes.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error("could not read body")
		}
		received.Store(string(raw))
		if r.Method != "POST" || r.URL.Path != "/proxy/body/demo" || r.URL.Query().Get("enabled") != "false" || r.Header.Get("If-Match") != `"current"` || r.Header.Get("Content-Type") != "application/json; charset=utf-8" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
			t.Error("body validation changed the surrounding request")
		}
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	app, out, stderr := testApp(t, srv)
	args := func(body, mode string) []string {
		return []string{"api", "request", "POST /body/{id}", "--path-param", "id=demo", "--query", "enabled=false", "--header", `If-Match:"current"`, "--content-type", "application/json; charset=utf-8", "--body", body, mode, "--json"}
	}
	if err := app.Run(context.Background(), args(body, "--dry-run")); err != nil {
		t.Fatal(err)
	}
	preview := decodeResult(t, out)
	data := preview.Data.(map[string]any)
	sum := sha256.Sum256([]byte(body))
	if !preview.OK || preview.Outcome != "preview" || writes.Load() != 0 || data["bodySha256"] != hex.EncodeToString(sum[:]) || data["bodyBytes"] != json.Number(strconv.Itoa(len(body))) {
		t.Fatal("preview differs from the exact supplied body or dispatched a write")
	}
	if err := app.Run(context.Background(), args(body, "--yes")); err != nil {
		t.Fatal(err)
	}
	got := decodeResult(t, out)
	if !got.OK || got.Outcome != "accepted" || writes.Load() != 1 || received.Load() != body {
		t.Fatal("body bytes changed during validation or dispatch")
	}
	for _, invalid := range []string{
		strings.Replace(body, `"amount": 9007199254740993`, `"amount": 9007199254740992`, 1),
		strings.Replace(body, `"amount": 9007199254740993`, `"amount": 9007199254740993, "amount": 9007199254740993`, 1),
		strings.Replace(body, `"amount": 9007199254740993`, `"amount": 1e999999999999999999999999`, 1),
		strings.Replace(body, `9007199254740992, 9007199254740993`, `9007199254740993, 9007199254740993`, 1),
		strings.Replace(body, `9007199254740992, 9007199254740993`, `1.000000000000000000001, 2`, 1),
		`null`,
	} {
		for _, mode := range []string{"--dry-run", "--yes"} {
			err := app.Run(context.Background(), args(invalid, mode))
			if strings.Contains(out.String(), "private-body-text") || strings.Contains(out.String(), "private-token") || stderr.Len() != 0 {
				t.Fatal("body validation exposed instance values or credentials")
			}
			got = decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || got.OK || got.Error == nil || got.Error.Kind != "validation" || writes.Load() != 1 {
				t.Fatal("invalid exact body was not refused before dispatch")
			}
		}
	}
	// Removing the private body declaration must not remove header validation.
	missingHeader := args(body, "--yes")
	missingHeader = append(missingHeader[:7], missingHeader[9:]...)
	err := app.Run(context.Background(), missingHeader)
	got = decodeResult(t, out)
	if igwerr.ExitCode(err) != 2 || got.OK || got.Error == nil || got.Error.Kind != "validation" || writes.Load() != 1 {
		t.Fatal("body adapter bypassed the required header contract")
	}
}
