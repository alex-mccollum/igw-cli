package reference

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func TestQualifiedReferenceRetainsCompleteEvidence(t *testing.T) {
	m, err := Read(context.Background(), "bundles/ignition-8.3.9-defaults")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Modules) != 32 || len(m.Files) != 9 || m.Image.GatewayVersion != "8.3.9 (b2026082511)" || m.Catalog.ContractSHA256 != "fce0593c41f1d0ae31c0647c34bb10ccebf6f44958c3c55913c887654ff9ccbc" {
		t.Fatal("qualified reference scope or identity changed without review")
	}
}

func TestHistoricalReferenceRejectsPolicyRelabeling(t *testing.T) {
	ctx := context.Background()
	bundle := Select("ignition-8.3.9-defaults")
	m, err := bundle.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"unknown-policy", catalog.ContractPolicy} {
		changed := m
		changed.Catalog.ContractPolicy = policy
		changed.Comparison.AfterIdentity = changed.Catalog
		changed.Comparison.BeforeIdentity.ContractPolicy = policy
		b, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		modified := bundle
		modified.read = func(ctx context.Context, name string, limit int64) ([]byte, error) {
			if name == "reference.json" {
				return b, nil
			}
			return bundle.read(ctx, name, limit)
		}
		if _, c, err := modified.OpenCatalog(ctx); err == nil {
			c.Close()
			t.Fatal("historical hash was accepted under a different identity policy")
		}
	}
	// Comparison flags cannot legitimize hashes calculated under two policies.
	m.Comparison.BeforeIdentity.ContractPolicy = catalog.ContractPolicy
	b, _ := json.Marshal(m)
	if _, err := readBundle(ctx, func(ctx context.Context, name string, limit int64) ([]byte, error) {
		if name == "reference.json" {
			return b, nil
		}
		return bundle.read(ctx, name, limit)
	}); err == nil {
		t.Fatal("cross-policy comparison was accepted as qualification evidence")
	}
}
