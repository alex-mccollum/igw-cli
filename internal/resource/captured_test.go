package resource

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

// Captured contracts establish request compatibility, not Gateway mutation or
// acknowledgement semantics. Real execution remains a separate live gate.
func TestCapturedSingletonRequests(t *testing.T) {
	for _, version := range []string{"8.3.0", "8.3.9"} {
		t.Run(version, func(t *testing.T) {
			file, err := os.Open(filepath.Join("..", "catalog", "testdata", "ignition-"+version+"-defaults", "openapi.json.gz"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			reader, err := gzip.NewReader(file)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			raw, err := io.ReadAll(io.LimitReader(reader, catalog.MaxDocumentBytes+1))
			if err != nil || len(raw) > catalog.MaxDocumentBytes {
				t.Fatal("invalid captured document")
			}
			c, err := catalog.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			count := 0
			for _, kind := range Types(c) {
				if !kind.Singleton {
					continue
				}
				count++
				for _, action := range []string{"create", "update", "delete"} {
					t.Run(kind.ID+"/"+action, func(t *testing.T) {
						calls := 0
						change := Change{Action: action, Type: kind.ID, Collection: "core", Singleton: true, DryRun: true}
						if action != "delete" {
							change.Body = []byte(`{"description":"proposed description"}`)
						}
						got := Apply(runFunc(func(input execute.Request) result.Result {
							calls++
							method, path, _ := strings.Cut(input.Operation, " ")
							for name, value := range input.PathParams {
								path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
							}
							var body io.Reader
							if input.Body != nil {
								body = bytes.NewReader(input.Body)
							}
							req, err := http.NewRequest(method, "http://gateway.test"+path, body)
							if err != nil {
								t.Fatal(err)
							}
							req.URL.RawQuery = input.Query.Encode()
							if input.Body != nil {
								req.Header.Set("Content-Type", "application/json")
							}
							checked, err := c.ValidateRequest(input.Operation, req)
							if err != nil || len(checked.Issues) != 0 {
								t.Fatalf("captured request contract: %+v %v", checked, err)
							}
							if calls == 1 {
								if action == "create" {
									return missing()
								}
								state, _ := json.Marshal(map[string]any{"type": kind.ID, "collection": "core", "signature": "reviewed-signature", "description": "old"})
								return response(string(state))
							}
							if calls != 2 || !input.DryRun {
								t.Fatal("unexpected dispatch")
							}
							out := result.Success(execute.Preview{Validation: checked.Coverage})
							out.Outcome = "preview"
							return out
						}), change)
						if !got.OK || got.Outcome != "preview" || calls != 2 {
							t.Fatalf("capture preview: %+v calls=%d", got, calls)
						}
					})
				}
			}
			if count != 17 {
				t.Fatalf("review singleton inventory change: %d", count)
			}
		})
	}
}
