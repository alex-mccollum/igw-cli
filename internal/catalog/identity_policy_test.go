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

func TestKeyboardIdentitySeparatesPolicyAndPreservesAssertions(t *testing.T) {
	for _, kind := range []string{"POST", "PUT", "find", "list"} {
		t.Run(kind, func(t *testing.T) {
			root := keyboardFixture(t, kind)
			paths := object(root["paths"])
			schema := map[string]any{"oneOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "number"}}, "examples": []any{"old"}}
			paths["/status"] = map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"description": "OK", "content": map[string]any{"application/json": map[string]any{"schema": schema}}}}}}
			before, _ := json.Marshal(root)
			current, err := contractDigest(root)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := contractDigestForPolicy(root, legacyContractPolicy)
			if err != nil || legacy != digest(append([]byte("igw-contract/1\n"), before...)) {
				t.Fatal("policy 1 no longer retains its original frozen-document behavior")
			}
			after, _ := json.Marshal(root)
			if !bytes.Equal(before, after) {
				t.Fatal("hashing mutated original evidence")
			}
			choices := schema["oneOf"].([]any)
			choices[0], choices[1] = choices[1], choices[0]
			schema["examples"] = []any{"new"}
			changedCurrent, _ := contractDigest(root)
			changedLegacy, _ := contractDigestForPolicy(root, legacyContractPolicy)
			if changedCurrent != current || changedLegacy == legacy {
				t.Fatal("reviewed reference scopes did not isolate documentation drift")
			}
			for path, rawItem := range paths {
				for method, rawOp := range object(rawItem) {
					pair := reviewedKeyboardDefinitions(object(rawOp), strings.ToUpper(method)+" "+path)
					if pair == nil {
						continue
					}
					for _, name := range []string{"config", "backupConfig"} {
						defs := object(object(pair.properties[name])["$defs"])
						variants := object(defs["keyVariant"])["oneOf"].([]any)
						object(variants[0])["maxLength"] = json.Number("1")
					}
				}
			}
			assertionHash, _ := contractDigest(root)
			if assertionHash == current || len(keyboardIdentityScopes(root)) != 2 {
				t.Fatal("new value assertion was hidden or reviewed scope stopped qualifying")
			}
			root["components"] = map[string]any{"schemas": map[string]any{"Unknown": map[string]any{"$ref": "#/missing"}}}
			unknownBefore, _ := contractDigest(root)
			schema["examples"] = []any{"changed again"}
			unknownAfter, _ := contractDigest(root)
			if unknownBefore == unknownAfter {
				t.Fatal("an unrelated unresolved reference no longer preserves its resource")
			}
		})
	}
}

func TestPreviousPolicyReceiptVerifiesBeforeMigration(t *testing.T) {
	c := testCatalog(t)
	original, err := c.IdentityForPolicy(legacyContractPolicy)
	if err != nil || original.ContractSHA256 == c.ContractHash() || original.RawSHA256 != c.RawHash() || original.DocumentSHA256 != c.DocumentHash() {
		t.Fatal("historical identity is mislabeled")
	}
	if _, err := c.IdentityForPolicy("unknown-policy"); err == nil {
		t.Fatal("unknown policy was accepted")
	}
	target, _ := NewTarget("test", "http://gateway.test")
	store := Store{Dir: t.TempDir()}
	verified := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	prior := &LegacyIdentity{ParserVersion: "version-1-parser", ContractSHA256: c.DocumentHash()}
	metadata := Metadata{Version: SnapshotVersion, Target: target, SourceKind: "gateway", Source: "http://gateway.test/openapi.json", FetchedAt: verified.Add(-time.Minute), VerifiedAt: verified, RawSHA256: c.RawHash(), ContractSHA256: c.ContractHash(), LegacyIdentity: prior}
	if err := store.Save(&Snapshot{Metadata: metadata, Catalog: c}); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(store.Dir, "targets", target.Key(), "*.json"))
	if len(paths) != 1 {
		t.Fatal("missing saved receipt")
	}
	data, _ := os.ReadFile(paths[0])
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata.ContractSHA256, metadata.ContractPolicy, metadata.ParserVersion = original.ContractSHA256, original.ContractPolicy, "previous-parser"
	originalBytes, _ := json.Marshal(metadata)
	if err := os.WriteFile(paths[0], originalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	m := loaded.Metadata
	if m.ContractPolicy != ContractPolicy || m.ContractSHA256 != c.ContractHash() || m.ParserVersion != ParserVersion || !m.VerifiedAt.Equal(verified) || !m.FetchedAt.Equal(metadata.FetchedAt) || len(loaded.Warnings) != 1 || m.LegacyIdentity == nil || m.LegacyIdentity.ContractPolicy != legacyContractPolicy || m.LegacyIdentity.ContractSHA256 != original.ContractSHA256 || m.LegacyIdentity.ParserVersion != "previous-parser" || m.LegacyIdentity.Previous == nil || *m.LegacyIdentity.Previous != *prior {
		t.Fatalf("incorrect policy migration: %+v", m)
	}
	stored, _ := os.ReadFile(paths[0])
	if !bytes.Equal(stored, originalBytes) {
		t.Fatal("migration rewrote an immutable receipt")
	}
	forPin, err := store.Load(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := checkPin(forPin, original.ContractSHA256); err == nil {
		t.Fatal("old live-target pin was silently accepted as a current identity")
	}
	for _, mutate := range []func(*Metadata){
		func(m *Metadata) { m.ContractSHA256 = strings.Repeat("0", 64) },
		func(m *Metadata) { m.DocumentSHA256 = strings.Repeat("0", 64) },
		func(m *Metadata) { m.ContractPolicy = "unknown-policy" },
	} {
		invalid := metadata
		mutate(&invalid)
		b, _ := json.Marshal(invalid)
		if err := os.WriteFile(paths[0], b, 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Load(target); err == nil {
			got.Close()
			t.Fatal("invalid historical identity was migrated")
		}
	}
}
