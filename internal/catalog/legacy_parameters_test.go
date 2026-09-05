package catalog

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Reduced from 8.3.0. The pattern is an added constraint to prove that the
// path-presence adapter preserves value validation as the vendor evolves.
const legacyPathFixture = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"/res/sys/license.html","name":"Inductive Automation EULA"}},"paths":{"/data/api/v1/entity/section/{section}":{"get":{"parameters":[{"name":"section","in":"path","description":"Section","required":false,"deprecated":false,"style":"simple","explode":false,"allowReserved":false,"schema":{"type":"string","pattern":"^[A-Z][a-z]+$"}}],"responses":{"200":{"description":"OK"}}}}}}`

const legacyCancelFixture = `{"openapi":"3.1.0","info":{"title":"Ignition HTTP API","version":"1.0.0","license":{"url":"/res/sys/license.html","name":"Inductive Automation EULA"}},"paths":{"/data/api/v1/scripts/cancel-script/{id}":{"delete":{"parameters":[{"name":"id","in":"path","description":"n/a","required":true,"deprecated":false,"style":"simple","explode":false,"allowReserved":false}],"responses":{"200":{"description":"OK"}}}},"/health":{"get":{"responses":{"200":{"description":"OK"}}}}}}`

func TestLegacySelectedPathPreservesValueConstraints(t *testing.T) {
	t.Parallel()
	for _, fixture := range []string{
		legacyPathFixture,
		strings.ReplaceAll(strings.ReplaceAll(legacyPathFixture, "/entity/section/{section}", "/scim/{profile-name}/{scim-version}/Users"), `"parameters":[{"name":"section"`, `"parameters":[{"name":"profile-name","in":"path","required":true,"schema":{"type":"string"}},{"name":"scim-version"`),
	} {
		c, err := Parse([]byte(fixture))
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer c.Close()
			op := c.Operations()[0]
			d, err := c.Describe(op.Key)
			if err != nil || len(d.Gaps) != 1 || !bytes.Contains(d.Operation.Definition, []byte(`"required":false`)) || !bytes.Equal(c.Raw(), []byte(fixture)) {
				t.Fatalf("original parameter contract or gap lost: %+v %v", d, err)
			}
			if c.Compatibility().Rules["selected-path-required"] != 1 {
				t.Fatal("path adjustment missing")
			}
			for _, value := range []string{"Platform", "invalid123"} {
				path := strings.NewReplacer("{section}", value, "{profile-name}", "profile", "{scim-version}", value).Replace(op.Path)
				req, _ := http.NewRequest("GET", "http://gateway.test"+path, nil)
				issues, err := c.Validate(op.Key, req)
				if err != nil || (len(issues) == 0) != (value == "Platform") {
					t.Fatalf("path value constraints changed: %v %v", issues, err)
				}
			}
		}()
	}
}

func TestLegacyCancellationGapBlocksOnlyUndocumentedOperation(t *testing.T) {
	t.Parallel()
	c, err := Parse([]byte(legacyCancelFixture))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	const key = "DELETE /data/api/v1/scripts/cancel-script/{id}"
	d, err := c.Describe(key)
	if err != nil || !bytes.Equal(c.Raw(), []byte(legacyCancelFixture)) || bytes.Contains(d.Operation.Definition, []byte(`"schema"`)) || !strings.Contains(strings.Join(d.Gaps, " "), "schema-assisted requests and previews are unavailable") {
		t.Fatalf("undocumented schema hidden: %+v %v", d, err)
	}
	if c.Compatibility().Rules["script-cancel-undocumented-id"] != 1 {
		t.Fatal("missing schema adjustment not reported")
	}
	req, _ := http.NewRequest("DELETE", "http://gateway.test/data/api/v1/scripts/cancel-script/example", nil)
	if _, err := c.Validate(key, req); !errors.Is(err, ErrIncompleteContract) {
		t.Fatalf("undocumented schema validated: %v", err)
	}
	req, _ = http.NewRequest("GET", "http://gateway.test/health", nil)
	if issues, err := c.Validate("GET /health", req); err != nil || len(issues) != 0 {
		t.Fatalf("unrelated operation blocked: %v %v", issues, err)
	}

	// A subsequently documented ID uses its supplied constraints normally.
	fixed, err := Parse([]byte(strings.Replace(legacyCancelFixture, `"allowReserved":false`, `"allowReserved":false,"schema":{"type":"string","enum":["known"]}`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	defer fixed.Close()
	for _, id := range []string{"known", "unknown"} {
		req, _ := http.NewRequest("DELETE", "http://gateway.test/data/api/v1/scripts/cancel-script/"+id, nil)
		issues, err := fixed.Validate(key, req)
		if err != nil || (len(issues) == 0) != (id == "known") {
			t.Fatalf("documented schema not enforced: %v %v", issues, err)
		}
	}
}

func TestLegacyParameterAdaptersRejectUnreviewedDefects(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		strings.Replace(legacyPathFixture, "Ignition HTTP API", "Other API", 1),
		strings.Replace(legacyPathFixture, "/res/sys/license.html", "/other/license.html", 1),
		strings.Replace(legacyPathFixture, "Inductive Automation EULA", "Other license", 1),
		strings.Replace(legacyPathFixture, "/entity/section/", "/other/section/", 1),
		strings.Replace(legacyPathFixture, `"get":`, `"post":`, 1),
		strings.Replace(legacyPathFixture, `"style":"simple"`, `"style":"label"`, 1),
		strings.Replace(legacyPathFixture, `"explode":false`, `"explode":true`, 1),
		strings.Replace(legacyPathFixture, `"type":"string"`, `"type":"integer"`, 1),
		strings.Replace(legacyPathFixture, `"allowReserved":false`, `"allowReserved":true`, 1),
		strings.Replace(legacyCancelFixture, "/scripts/cancel-script/", "/scripts/other/", 1),
		strings.Replace(legacyCancelFixture, `"description":"n/a"`, `"description":"changed"`, 1),
		strings.Replace(legacyCancelFixture, `"allowReserved":false`, `"allowReserved":false,"schema":null`, 1),
		strings.Replace(legacyCancelFixture, `"in":"path"`, `"in":"query"`, 1),
	} {
		if c, err := Parse([]byte(raw)); err == nil {
			c.Close()
			t.Fatalf("unreviewed defect accepted: %s", raw)
		}
	}
}
