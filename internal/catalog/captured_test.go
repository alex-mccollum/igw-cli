package catalog

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCapturedIgnitionCatalogs(t *testing.T) {
	paths, err := filepath.Glob("testdata/*/capture.json")
	if err != nil || len(paths) == 0 {
		t.Fatal("captured Gateway contracts are missing")
	}
	// Large vendor schemas are deliberately tested sequentially to bound memory.
	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				RawSHA256      string         `json:"rawSha256"`
				ContractSHA256 string         `json:"contractSha256"`
				Operations     int            `json:"operations"`
				Validated      bool           `json:"validated"`
				Compatibility  *Compatibility `json:"compatibility"`
			}
			if json.Unmarshal(b, &receipt) != nil || !receipt.Validated {
				t.Fatal("fixture must have qualified capture evidence")
			}
			f, err := os.Open(filepath.Join(filepath.Dir(path), "openapi.json.gz"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			reader, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			raw, err := io.ReadAll(io.LimitReader(reader, MaxDocumentBytes+1))
			if err != nil || len(raw) > MaxDocumentBytes {
				t.Fatal("invalid compressed capture")
			}
			c, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if c.RawHash() != receipt.RawSHA256 || c.ContractHash() != receipt.ContractSHA256 || len(c.Operations()) != receipt.Operations || !reflect.DeepEqual(c.Compatibility(), receipt.Compatibility) {
				t.Fatal("capture checksum or qualification drift")
			}
			for _, tc := range []struct {
				body  string
				valid bool
			}{
				{`[{"name":"fixture","signature":"observed-signature"}]`, true},
				{`[{"name":"fixture"}]`, false},
			} {
				req, _ := http.NewRequest("POST", "http://gateway.test/data/api/v1/resources/delete/ignition/schedule", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				issues, err := c.Validate("POST /data/api/v1/resources/delete/ignition/schedule", req)
				if err != nil || (len(issues) == 0) != tc.valid {
					t.Fatalf("captured request contract: %v %v", issues, err)
				}
			}
			// The vendor repeats identical $id values in config and backupConfig.
			// Until a separate reviewed adapter exists, classify this as a schema
			// availability defect, not an invalid user payload or successful check.
			req, _ := http.NewRequest("POST", "http://gateway.test/data/api/v1/resources/ignition/schedule", strings.NewReader(`[{"name":"fixture"}]`))
			req.Header.Set("Content-Type", "application/json")
			if _, err := c.Validate("POST /data/api/v1/resources/ignition/schedule", req); !errors.Is(err, ErrSchemaCompilation) {
				t.Fatalf("unqualified vendor schema was misrepresented: %v", err)
			}
		})
	}
}
