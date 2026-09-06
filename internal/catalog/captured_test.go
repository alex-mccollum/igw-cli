package catalog

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapturedIgnitionCatalogs(t *testing.T) {
	paths, err := filepath.Glob("../reference/bundles/*/reference.json")
	if err != nil || len(paths) != 4 {
		t.Fatal("canonical vendor references are missing")
	}
	counts := map[string]int{"ignition-8.3.0-core": 446, "ignition-8.3.0-defaults": 672, "ignition-8.3.9-core": 454, "ignition-8.3.9-defaults": 687}
	// One canonical fixture per version/profile. Keep vendor behavior assertions
	// here; historical run transcripts are archived in Git, not parsed by tests.
	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Catalog Identity `json:"catalog"`
			}
			if json.Unmarshal(b, &manifest) != nil {
				t.Fatal("invalid reference manifest")
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
				t.Fatal("invalid compressed reference")
			}
			c, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if c.Identity() != manifest.Catalog || c.OperationCount() != counts[filepath.Base(filepath.Dir(path))] {
				t.Fatal("canonical reference identity or inventory drift")
			}
			for _, tc := range []struct {
				operation, media, encoding, coverage string
			}{
				{"POST /data/api/v1/encryption/encrypt", "text/plain", "utf8", ValidationSchema},
				{"POST /data/api/v1/encryption/encrypt", "application/octet-stream", "binary", ValidationTransport},
				{"POST /data/api/v1/activation/offline/activate", "application/octet-stream", "binary", ValidationTransport},
			} {
				description, err := c.Describe(tc.operation)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, input := range description.BodyInputs {
					if input.MediaType == tc.media {
						found = input.Encoding == tc.encoding && input.Validation == tc.coverage && input.SchemaDeclared
					}
				}
				if !found {
					t.Fatalf("captured body coverage differs for %s: %+v", tc.operation, description.BodyInputs)
				}
			}
			if !c.OpaqueUpload("PUT /data/api/v1/resources/datafile/com.inductiveautomation.opcua/device/{name}/{filename}", "application/octet-stream") {
				t.Fatal("captured wildcard datafile body could not stream")
			}
			for _, tc := range []struct {
				query string
				valid bool
			}{
				{"sessionId=one&sessionId=two", true},
				{"sessionId=one%2Ctwo&message=text%20%2B%20%26%20%23%20%25", true},
				{"message=no-session", false},
				{"sessionId=one&message=one&message=two", false},
			} {
				const route = "/data/perspective/api/v1/sessions"
				if _, err := c.Resolve("DELETE " + route); err != nil {
					continue
				}
				req, _ := http.NewRequest("DELETE", "http://gateway.test"+route+"?"+tc.query, nil)
				issues, err := c.Validate("DELETE "+route, req)
				if err != nil || (len(issues) == 0) != tc.valid || req.URL.RawQuery != tc.query {
					t.Fatalf("captured session array valid=%t: %+v %v", tc.valid, issues, err)
				}
			}
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
