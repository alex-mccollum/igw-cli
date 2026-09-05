package reference

import (
	"context"
	"testing"
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
