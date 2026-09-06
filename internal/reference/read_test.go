package reference

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func TestCompactReferencePreservesHistoricalProvenance(t *testing.T) {
	bundle := Select("ignition-8.3.9-defaults")
	m, c, err := bundle.OpenCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if len(m.Modules) != 32 || len(m.Files) != 1 || m.Catalog != c.Identity() || m.Catalog.ContractPolicy != catalog.ContractPolicy || m.Qualification.Catalog.ContractPolicy != "igw-contract/1" || m.Qualification.ParserVersion == catalog.ParserVersion || !strings.Contains(m.Qualification.Evidence.URI, "65e643d") {
		t.Fatal("format conversion relabeled qualification or lost original provenance")
	}
}

func TestManifestListingDoesNotReadPayload(t *testing.T) {
	bundle := Select("ignition-8.3.0-core")
	_, err := readManifest(context.Background(), func(ctx context.Context, name string, limit int64) ([]byte, error) {
		if name != "reference.json" {
			t.Fatalf("listing read payload %s", name)
		}
		return bundle.read(ctx, name, limit)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReferenceRejectsOldFormatsAndRelabeledIdentity(t *testing.T) {
	bundle := Select("ignition-8.3.0-core")
	m, err := bundle.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"old format", "old policy", "changed hash"} {
		changed := m
		switch kind {
		case "old format":
			changed.Version = "igw/reference/v1"
		case "old policy":
			changed.Catalog.ContractPolicy = "igw-contract/1"
		case "changed hash":
			changed.Catalog.ContractSHA256 = strings.Repeat("0", 64)
		}
		raw, _ := json.Marshal(changed)
		modified := bundle
		modified.read = func(ctx context.Context, name string, limit int64) ([]byte, error) {
			if name == "reference.json" {
				return raw, nil
			}
			return bundle.read(ctx, name, limit)
		}
		if _, c, err := modified.OpenCatalog(context.Background()); err == nil {
			c.Close()
			t.Fatalf("accepted %s", kind)
		} else if kind == "old format" && !strings.Contains(err.Error(), "import the raw OpenAPI") {
			t.Fatal("missing recovery instruction")
		}
	}
}

func TestCaptureDateProvenanceAndLegacyAbsence(t *testing.T) {
	bundle := Select("ignition-8.3.9-defaults")
	m, err := bundle.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.CapturedAt == nil || m.CapturedAt.Format(time.RFC3339Nano) != "2026-09-05T16:04:53.330033397Z" {
		t.Fatalf("original capture date missing: %v", m.CapturedAt)
	}
	for _, tc := range []struct {
		name  string
		date  *time.Time
		valid bool
	}{
		{"recorded", m.CapturedAt, true},
		{"unknown", nil, true},
		{"zero", new(time.Time), false},
		{"after assembly", timePointer(m.CreatedAt.Add(time.Second)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := m
			changed.CapturedAt = tc.date
			raw, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := readManifest(context.Background(), func(context.Context, string, int64) ([]byte, error) { return raw, nil })
			if (err == nil) != tc.valid {
				t.Fatalf("capture date validity: %v", err)
			}
			if !tc.valid {
				return
			}
			summary := bundle.Summary(loaded)
			if summary.CatalogParserVersion != m.ParserVersion || summary.ParserVersion != m.Qualification.ParserVersion || summary.InspectionParserVersion != "" || summary.InspectionCatalog != nil {
				t.Fatal("inspection and qualification conflated")
			}
			if tc.date == nil && summary.CapturedAt != nil {
				t.Fatal("unknown capture date inferred from assembly")
			}
			if tc.date == nil && strings.Contains(string(raw), "capturedAt") {
				t.Fatal("unknown capture date should be omitted")
			}
		})
	}
}

func timePointer(v time.Time) *time.Time { return &v }
