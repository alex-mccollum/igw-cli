package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestURLEncodedFormWireCLI(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		t.Run(version, func(t *testing.T) {
			spec := fmt.Sprintf(`{"openapi":%q,"info":{"title":"Synthetic form input","version":"test"},"servers":[{"url":"https://foreign.invalid"}],"paths":{"/form":{"post":{"requestBody":{"required":true,"content":{"application/x-www-form-urlencoded":{"schema":{"type":"object","required":["id","text","values"],"properties":{"id":{"type":"integer","minimum":9007199254740993},"text":{"type":"string"},"values":{"type":"array","items":{"type":"string"},"minItems":2,"uniqueItems":true}},"additionalProperties":false},"encoding":{"values":{"style":"form"}}}}},"responses":{"200":{"description":"OK"}}}}}}`, version)
			want := "id=9007199254740993&text=+%2B%26%3D%252F+%E6%97%A5%E6%9C%AC+&values=a%2Cb&values="
			var writes atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/openapi.json" {
					_, _ = io.WriteString(w, spec)
					return
				}
				writes.Add(1)
				body, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.URL.EscapedPath() != "/proxy/form" || r.URL.RawQuery != "" || string(body) != want || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Header.Get("X-Ignition-API-Token") != "private-token" {
					t.Error("form bytes, selected target, or credential changed")
				}
				_, _ = io.WriteString(w, `{"accepted":true}`)
			}))
			defer srv.Close()
			app, out, stderr := testApp(t, srv)
			args := []string{"api", "request", "POST /form", "--urlencoded", "values=a,b", "--urlencoded", "text= +&=%2F 日本 ", "--urlencoded", "values=", "--urlencoded", "id=9007199254740993", "--json"}
			if err := app.Run(context.Background(), append(args, "--dry-run")); err != nil {
				t.Fatal(err)
			}
			preview := decodeResult(t, out)
			data := preview.Data.(map[string]any)
			digest := sha256.Sum256([]byte(want))
			if preview.Outcome != "preview" || writes.Load() != 0 || data["validation"] != "declared_schema" || data["bodySha256"] != hex.EncodeToString(digest[:]) {
				t.Fatal("preview dispatched or changed wire coverage/digest")
			}
			if err := app.Run(context.Background(), append(args, "--yes")); err != nil {
				t.Fatal(err)
			}
			if !decodeResult(t, out).OK || writes.Load() != 1 {
				t.Fatal("form did not dispatch once")
			}
			for _, invalid := range [][]string{
				{"--body", "private-invalid-value"}, {"--content-type", "application/json"},
				{"--upload", "nonexistent"}, {"--multipart", "[]"},
				{"--form-field", "x=a"}, {"--form-file", "x=nonexistent"},
				{"--urlencoded", "values=a,b"}, {"--urlencoded", "id=private-invalid-value"},
			} {
				for _, mode := range []string{"--dry-run", "--yes"} {
					input := append(append([]string(nil), args...), invalid...)
					err := app.Run(context.Background(), append(input, mode))
					output := out.String() + stderr.String()
					got := decodeResult(t, out)
					if err == nil || got.Error == nil || got.Error.Code != 2 || writes.Load() != 1 {
						t.Fatal("invalid form reached transport or changed exit code")
					}
					if strings.Contains(output, "private-invalid-value") || strings.Contains(output, "private-token") {
						t.Fatal("form error disclosed values or credentials")
					}
				}
			}
		})
	}
}
