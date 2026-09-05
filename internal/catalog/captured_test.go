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
	identicalImages := make(map[string]string)
	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var receipt struct {
				Version         int            `json:"version"`
				Image           string         `json:"image"`
				Modules         []string       `json:"moduleWhitelist"`
				RawSHA256       string         `json:"rawSha256"`
				ContractSHA256  string         `json:"contractSha256"`
				ContractPolicy  string         `json:"contractPolicy"`
				Operations      int            `json:"operations"`
				Validated       bool           `json:"validated"`
				Cleanup         bool           `json:"cleanup"`
				ValidationError string         `json:"validationError"`
				Compatibility   *Compatibility `json:"compatibility"`
			}
			if json.Unmarshal(b, &receipt) != nil {
				t.Fatal("invalid capture evidence")
			}
			// A captured document can predate its reviewed parser adapter. Keep
			// the failed capture receipt intact; qualification.json below must
			// independently match every current parser identity and adjustment.
			if !receipt.Validated && (receipt.Version != 3 || !receipt.Cleanup || receipt.ValidationError == "" || receipt.Operations != 0 || receipt.ContractSHA256 != "") {
				t.Fatal("invalid historical capture failure")
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
			capturedHash := c.ContractHash()
			if receipt.Version == 1 {
				capturedHash = c.DocumentHash()
			} else if receipt.Validated {
				original, err := c.IdentityForPolicy(receipt.ContractPolicy)
				if err != nil {
					t.Fatal(err)
				}
				capturedHash = original.ContractSHA256
			}
			if c.RawHash() != receipt.RawSHA256 || (receipt.Validated && (capturedHash != receipt.ContractSHA256 || c.OperationCount() != receipt.Operations)) {
				t.Fatal("capture checksum or qualification drift")
			}
			b, err = os.ReadFile(filepath.Join(filepath.Dir(path), "qualification.json"))
			if err != nil {
				t.Fatal(err)
			}
			var qualified struct {
				Identity
				ParserVersion  string         `json:"parserVersion"`
				OperationCount int            `json:"operationCount"`
				Compatibility  *Compatibility `json:"compatibility"`
			}
			if json.Unmarshal(b, &qualified) != nil || qualified.Identity != c.Identity() || qualified.ParserVersion != ParserVersion || qualified.OperationCount != c.OperationCount() || !reflect.DeepEqual(qualified.Compatibility, c.Compatibility()) {
				t.Fatal("current parser qualification drift")
			}
			modules, _ := json.Marshal(receipt.Modules)
			group := receipt.Image + string(modules)
			if previous, ok := identicalImages[group]; ok && previous != c.ContractHash() {
				t.Fatal("identical image/module captures have unstable contract identities")
			}
			identicalImages[group] = c.ContractHash()
			for _, route := range []string{"/data/api/v1/projects/list", "/data/api/v1/logs"} {
				for _, query := range []string{"limit=10&offset=0", "limit=invalid", "filter=invalid"} {
					req, _ := http.NewRequest("GET", "http://gateway.test"+route+"?"+query, nil)
					issues, err := c.Validate("GET "+route, req)
					if err != nil || (len(issues) == 0) != (query == "limit=10&offset=0") {
						t.Fatalf("captured %s list query: %v %v", route, issues, err)
					}
				}
			}
			for _, tc := range []struct {
				query string
				valid bool
			}{
				{"limit=10&offset=0", true},
				{"search=Example", true},
				{"limit=10&name%5Beq%5D=Example", true},
				{"name%5Beq%5D=A%2BB%26C%23D%25", true},
				{"name%5Beq%5D=Example&name%5Beq%5D=Other", false},
				{"name%5Bunknown%5D=Example", false},
				{"limit=invalid", false},
				{"unexpected=invalid", false},
				{"filter=invalid", false},
				{"limit=10&offset=0", true}, // A prior call must not mutate the model.
			} {
				req, _ := http.NewRequest("GET", "http://gateway.test/data/api/v1/resources/list/ignition/schedule?"+tc.query, nil)
				issues, err := c.Validate("GET /data/api/v1/resources/list/ignition/schedule", req)
				if err != nil || (len(issues) == 0) != tc.valid {
					t.Fatalf("captured list query %s: %v %v", tc.query, issues, err)
				}
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
			for _, tc := range []struct {
				method, body string
				valid        bool
			}{
				{"POST", `[{"name":"fixture","config":{"profile":{"type":"basic schedule"},"settings":{"allDays":true}}}]`, true},
				{"POST", `[{"name":"fixture","config":{"profile":{"type":"unknown"}}}]`, false},
				{"POST", `[{"name":"fixture","config":{"settings":{"repeatOn":"bad"}}}]`, false},
				{"POST", `[{"name":"fixture","backupConfig":{"settings":{"repeatOn":"bad"}}}]`, false},
				{"PUT", `[{"name":"fixture","signature":"observed","config":{"settings":{"allDays":true}}}]`, true},
				{"PUT", `[{"name":"fixture","config":{"settings":{"allDays":true}}}]`, false},
			} {
				req, _ := http.NewRequest(tc.method, "http://gateway.test/data/api/v1/resources/ignition/schedule", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
				issues, err := c.Validate(tc.method+" /data/api/v1/resources/ignition/schedule", req)
				if err != nil || (len(issues) == 0) != tc.valid {
					t.Fatalf("captured schedule contract: %v %v", issues, err)
				}
			}
			// Tag-provider variants contain references, so their duplicate IDs
			// cannot use the reference-free adapter. Preserve a schema error.
			req, _ := http.NewRequest("POST", "http://gateway.test/data/api/v1/resources/ignition/tag-provider", strings.NewReader(`[{"name":"fixture"}]`))
			req.Header.Set("Content-Type", "application/json")
			if _, err := c.Validate("POST /data/api/v1/resources/ignition/tag-provider", req); !errors.Is(err, ErrSchemaCompilation) {
				t.Fatalf("unqualified vendor schema was misrepresented: %v", err)
			}
		})
	}
}
