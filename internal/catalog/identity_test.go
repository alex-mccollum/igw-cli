package catalog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func identityValue(t *testing.T, raw string) any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	v, err := decodeUniqueJSON(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func schemaIdentity(t *testing.T, schema string) string {
	t.Helper()
	v := identityValue(t, `{"openapi":"3.1.0","info":{"title":"Fixture","version":"test"},"paths":{},"components":{"schemas":{"Input":`+schema+`}}}`)
	h, err := contractDigest(v)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestContractProjectionPreservesInputSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		equal      bool
	}{
		{"annotations", `{"type":"string","examples":["a"],"description":"old"}`, `{"type":"string","examples":["b"],"description":"new"}`, true},
		{"alternatives", `{"oneOf":[{"type":"string"},{"type":"number"}]}`, `{"oneOf":[{"type":"number"},{"type":"string"}]}`, true},
		{"enum", `{"enum":["a","b"]}`, `{"enum":["b","a"]}`, true},
		{"required", `{"required":["a","b"]}`, `{"required":["b","a"]}`, true},
		{"type-union", `{"type":["null","string"]}`, `{"type":["string","null"]}`, true},
		{"constraint", `{"maximum":9007199254740993}`, `{"maximum":9007199254740992}`, false},
		{"number-spelling", `{"minimum":1}`, `{"minimum":1.0}`, false},
		{"enum-member", `{"enum":[[1,2]]}`, `{"enum":[[2,1]]}`, false},
		{"const-data", `{"const":{"description":"old","oneOf":[1,2]}}`, `{"const":{"description":"new","oneOf":[2,1]}}`, false},
		{"default-data", `{"default":[1,2]}`, `{"default":[2,1]}`, false},
		{"tuple", `{"prefixItems":[{"type":"string"},{"type":"number"}]}`, `{"prefixItems":[{"type":"number"},{"type":"string"}]}`, false},
		{"legacy-tuple", `{"items":[{"type":"string"},{"type":"number"}]}`, `{"items":[{"type":"number"},{"type":"string"}]}`, false},
		{"duplicate-alternative", `{"oneOf":[{"type":"string"}]}`, `{"oneOf":[{"type":"string"},{"type":"string"}]}`, false},
		{"named-property", `{"properties":{"description":{"const":"a"}}}`, `{"properties":{"description":{"const":"b"}}}`, false},
		{"extension", `{"x-input":{"description":"a","enum":[1,2]}}`, `{"x-input":{"description":"b","enum":[2,1]}}`, false},
		{"unknown-keyword", `{"future":{"examples":[1]}}`, `{"future":{"examples":[2]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if (schemaIdentity(t, tc.a) == schemaIdentity(t, tc.b)) != tc.equal {
				t.Fatal("contract identity changed input semantics")
			}
		})
	}
}

func TestContractProjectionPreservesReferenceTargetsAndPositions(t *testing.T) {
	for _, tc := range []struct{ name, a, b string }{
		{"array-index", `{"allOf":[{"$ref":"#/components/schemas/Input/oneOf/0"}],"oneOf":[{"type":"string"},{"type":"number"}]}`, `{"allOf":[{"$ref":"#/components/schemas/Input/oneOf/0"}],"oneOf":[{"type":"number"},{"type":"string"}]}`},
		{"annotation-target", `{"$ref":"#/components/schemas/Input/examples/0","examples":[{"const":"a"}]}`, `{"$ref":"#/components/schemas/Input/examples/0","examples":[{"const":"b"}]}`},
		{"embedded-resource", `{"$id":"urn:test","$ref":"#/oneOf/0","oneOf":[{"type":"string"},{"type":"number"}]}`, `{"$id":"urn:test","$ref":"#/oneOf/0","oneOf":[{"type":"number"},{"type":"string"}]}`},
		{"anchor", `{"$id":"urn:test","$ref":"#here","examples":[1]}`, `{"$id":"urn:test","$ref":"#here","examples":[2]}`},
		{"dynamic", `{"$dynamicRef":"#here","examples":[1]}`, `{"$dynamicRef":"#here","examples":[2]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if schemaIdentity(t, tc.a) == schemaIdentity(t, tc.b) {
				t.Fatal("referenced constraint was hidden by normalization")
			}
		})
	}
}

func TestProjectionCoversParametersHeadersBodiesAndCallbacks(t *testing.T) {
	raw := `{
  "openapi": "3.1.0",
  "info": {
    "title": "Fixture",
    "version": "test"
  },
  "paths": {
    "/items": {
      "parameters": [
        {
          "in": "query",
          "name": "q",
          "schema": {
            "type": "string",
            "examples": [
              "old"
            ]
          }
        }
      ],
      "post": {
        "summary": "old",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "properties": {
                  "summary": {
                    "const": "keep"
                  }
                },
                "examples": [
                  "old"
                ]
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "old",
            "headers": {
              "X-Version": {
                "schema": {
                  "type": "string",
                  "examples": [
                    "old"
                  ]
                }
              }
            }
          }
        },
        "callbacks": {
          "notice": {
            "{$request.body#/url}": {
              "post": {
                "requestBody": {
                  "content": {
                    "application/json": {
                      "schema": {
                        "type": "string",
                        "examples": [
                          "old"
                        ]
                      }
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`
	a := identityValue(t, raw)
	before, _ := json.Marshal(a)
	h1, err := contractDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := contractDigest(identityValue(t, strings.ReplaceAll(raw, `"old"`, `"new"`)))
	if err != nil || h1 != h2 {
		t.Fatal("documentation drift affected contract")
	}
	after, _ := json.Marshal(a)
	if !bytes.Equal(before, after) {
		t.Fatal("identity projection mutated vendor data")
	}
	h3, _ := contractDigest(identityValue(t, strings.ReplaceAll(raw, `"keep"`, `"changed"`)))
	if h1 == h3 {
		t.Fatal("a property named summary was treated as annotation")
	}
}

func TestLegacyReceiptRequalifiesWithoutRewritingHistory(t *testing.T) {
	c := testCatalog(t)
	target, _ := NewTarget("test", "http://gateway.test")
	store := Store{Dir: t.TempDir()}
	verified := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	dir := filepath.Join(store.Dir, "targets", target.Key())
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, "blobs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "blobs", c.RawHash()+".json"), c.Raw(), 0600); err != nil {
		t.Fatal(err)
	}
	receipt := Metadata{Version: 1, Target: target, SourceKind: "gateway", Source: "http://gateway.test/openapi.json", VerifiedAt: verified, RawSHA256: c.RawHash(), ContractSHA256: c.DocumentHash(), ParserVersion: "previous-parser"}
	b, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "0001.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	m := loaded.Metadata
	if m.Version != SnapshotVersion || m.ContractSHA256 != c.ContractHash() || m.DocumentSHA256 != c.DocumentHash() || m.ContractPolicy != ContractPolicy || !m.VerifiedAt.Equal(verified) || m.LegacyIdentity == nil || m.LegacyIdentity.ContractSHA256 != receipt.ContractSHA256 || len(loaded.Warnings) == 0 {
		t.Fatalf("incorrect legacy requalification: %+v", m)
	}
	forPin, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkPin(forPin, receipt.ContractSHA256); err == nil {
		t.Fatal("legacy pin was silently accepted as new identity")
	}
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, b) {
		t.Fatal("immutable receipt was rewritten")
	}
	for _, tc := range []struct {
		name    string
		receipt Metadata
	}{
		{"legacy-checksum", func() Metadata { m := receipt; m.ContractSHA256 = strings.Repeat("0", 64); return m }()},
		{"document-checksum", func() Metadata { m := loaded.Metadata; m.DocumentSHA256 = strings.Repeat("0", 64); return m }()},
		{"contract-checksum", func() Metadata { m := loaded.Metadata; m.ContractSHA256 = strings.Repeat("0", 64); return m }()},
		{"unrecognized-policy", func() Metadata { m := loaded.Metadata; m.ContractPolicy = "future/unknown"; return m }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			if invalid, err := store.Load(target); err == nil {
				invalid.Close()
				t.Fatal("invalid identity was accepted")
			}
		})
	}
}
