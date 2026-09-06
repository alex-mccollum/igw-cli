package cli

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/reference"
)

type forbiddenReferenceTransport struct{ t *testing.T }

func (f forbiddenReferenceTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Fatal("reference command contacted the network")
	return nil, nil
}

const builtinReference = "ignition-8.3.9-defaults"
const builtinContract = "fce0593c41f1d0ae31c0647c34bb10ccebf6f44958c3c55913c887654ff9ccbc"

// All offline paths must tolerate missing/broken configuration, credentials,
// cache, and Gateway connectivity, including negative command paths.
func forbidReferenceRuntime(t *testing.T, app *App) {
	t.Helper()
	app.ReadConfig = func() (config.File, error) {
		t.Fatal("reference command loaded Gateway configuration")
		return config.File{}, nil
	}
	app.Getenv = func(string) string {
		t.Fatal("reference command read credentials or Gateway environment")
		return ""
	}
	app.HTTP = &http.Client{Transport: forbiddenReferenceTransport{t}}
	app.CacheDir = filepath.Join(t.TempDir(), "unused-cache")
}

func TestReferenceCommandsWorkWithoutGatewayAndPreserveEvidence(t *testing.T) {
	app, out, _ := testApp(t, nil)
	forbidReferenceRuntime(t, &app)
	ctx := context.Background()
	if err := app.Run(ctx, []string{"spec", "references", "list", "--json"}); err != nil {
		t.Fatal(err)
	}
	r := decodeResult(t, out)
	items := r.Data.([]any)
	if len(items) != 4 || items[3].(map[string]any)["selector"] != builtinReference || r.Meta.Target != nil || r.Meta.Catalog != nil {
		t.Fatalf("unexpected reference list: %+v", r)
	}
	if err := app.Run(ctx, []string{"spec", "references", "inspect", builtinReference, "--spec-pin", builtinContract, "--json"}); err != nil {
		t.Fatal(err)
	}
	r = decodeResult(t, out)
	if r.Meta.Reference == nil || r.Meta.Reference.ModuleCount != 32 || r.Meta.Reference.ActiveModuleCount != 32 || r.Meta.Reference.InspectionParserVersion != "" || len(r.Data.(map[string]any)["modules"].([]any)) != 32 {
		t.Fatalf("reference metadata missing or overstated: %+v", r.Meta)
	}
	dir := filepath.Join(t.TempDir(), "reference")
	if err := app.Run(ctx, []string{"spec", "references", "export", builtinReference, "--out", dir, "--json"}); err != nil {
		t.Fatal(err)
	}
	r = decodeResult(t, out)
	if !r.OK || r.Data.(map[string]any)["directory"] != dir {
		t.Fatal("export did not identify its directory")
	}
	m, err := reference.Read(ctx, dir)
	if err != nil || m.Catalog.ContractSHA256 != builtinContract {
		t.Fatalf("CLI export is not independently valid: %v", err)
	}
	if err := app.Run(ctx, []string{"spec", "references", "inspect", dir, "--json"}); err != nil {
		t.Fatal(err)
	}
	r = decodeResult(t, out)
	if r.Meta.Reference == nil || r.Meta.Reference.Origin != "directory" || r.Meta.Reference.Name != m.Name {
		t.Fatal("directory reference lost provenance")
	}
	err = app.Run(ctx, []string{"spec", "references", "export", builtinReference, "--out", dir, "--json"})
	r = decodeResult(t, out)
	if igwerr.ExitCode(err) != 2 || r.OK {
		t.Fatal("existing reference output was replaced")
	}
	if _, err := os.Stat(app.CacheDir); !os.IsNotExist(err) {
		t.Fatal("reference command touched target cache")
	}
}

// Real-document parses qualify both embedded and independently exported
// selection. This is serial within the package to bound schema-model memory.
func TestAPIDiscoveryUsesQualifiedReferenceWithoutTarget(t *testing.T) {
	bundle := reference.Select(builtinReference)
	m, err := bundle.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "reference")
	if _, err := bundle.Export(context.Background(), dir, m.Catalog); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"api", "list", "--reference", builtinReference, "--search", "gateway-info"},
		{"api", "describe", "GET /data/api/v1/gateway-info", "--reference", dir},
		{"api", "capabilities", "--reference", builtinReference},
	} {
		app, out, _ := testApp(t, nil)
		forbidReferenceRuntime(t, &app)
		if err := app.Run(context.Background(), append(args, "--spec-pin", builtinContract, "--json")); err != nil {
			t.Fatal(err)
		}
		r := decodeResult(t, out)
		ref := r.Meta.Reference
		if ref == nil || ref.SourceKind != "reference" || ref.Catalog.ContractSHA256 != builtinContract || ref.InspectionParserVersion != catalog.ParserVersion || r.Meta.Catalog != nil || r.Meta.Target != nil || r.Meta.Stale {
			t.Fatalf("reference was confused with live target evidence: %+v", r.Meta)
		}
		if ref.Catalog.ContractPolicy != "igw-contract/1" || ref.InspectionCatalog == nil || ref.InspectionCatalog.ContractPolicy != catalog.ContractPolicy || ref.InspectionCatalog.ContractSHA256 == ref.Catalog.ContractSHA256 || ref.InspectionCatalog.RawSHA256 != ref.Catalog.RawSHA256 || ref.InspectionCatalog.DocumentSHA256 != ref.Catalog.DocumentSHA256 {
			t.Fatal("historical qualification and current inspection identities were conflated")
		}
		if args[1] == "list" {
			items := r.Data.([]any)
			if len(items) != 1 || items[0].(map[string]any)["key"] != "GET /data/api/v1/gateway-info" {
				t.Fatalf("reference search did not discover operation: %+v", items)
			}
		}
		if _, err := os.Stat(app.CacheDir); !os.IsNotExist(err) {
			t.Fatal("reference discovery wrote target cache")
		}
	}
}

func TestReferenceMatrixCapabilitiesAndModuleInventory(t *testing.T) {
	for _, tt := range []struct {
		name   string
		active int
		tags   bool
	}{
		{"ignition-8.3.0-defaults", 32, false},
		{"ignition-8.3.0-core", 1, false},
		{"ignition-8.3.9-core", 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, out, _ := testApp(t, nil)
			forbidReferenceRuntime(t, &app)
			if err := app.Run(context.Background(), []string{"api", "capabilities", "--reference", tt.name, "--json"}); err != nil {
				t.Fatal(err)
			}
			r := decodeResult(t, out)
			ref := r.Meta.Reference
			if !r.OK || ref == nil || ref.Selector != tt.name || ref.ModuleCount != 32 || ref.ActiveModuleCount != tt.active || r.Meta.Target != nil || r.Meta.Catalog != nil || r.Meta.Stale {
				t.Fatalf("incorrect offline reference provenance: %+v", r)
			}
			if tt.active == 1 && (ref.ModuleProfile == nil || ref.ModuleProfile.Name != "core-opcua") {
				t.Fatal("missing core profile evidence")
			}
			status, unavailable := "advertised", 0
			if !tt.tags {
				status, unavailable = "unavailable", 1
			}
			items := r.Data.([]any)
			if len(items) != 4 || len(ref.Qualification.UnavailableScopes) != unavailable {
				t.Fatal("unexpected capability coverage")
			}
			for _, item := range items {
				want := status
				if item.(map[string]any)["id"] == "gateway.restart.verified" {
					want = "advertised"
				}
				if item.(map[string]any)["status"] != want {
					t.Fatalf("incorrect tag capability: %+v", item)
				}
			}
			if _, err := os.Stat(app.CacheDir); !os.IsNotExist(err) {
				t.Fatal("offline reference inspection touched target cache")
			}
		})
	}
}

func TestReferenceFailuresNeverFallBackToGateway(t *testing.T) {
	for _, args := range [][]string{
		{"api", "list", "--reference", ""},
		{"api", "list", "--reference", "missing-reference"},
		{"api", "capabilities", "--reference", "missing-reference"},
		{"api", "list", "--reference", builtinReference, "--spec-pin", strings.Repeat("0", 64)},
		{"api", "describe", "ignored", "--reference", builtinReference, "--spec-pin", "malformed"},
		{"spec", "references", "inspect", builtinReference, "--spec-pin", strings.Repeat("0", 64)},
		{"spec", "references", "export", builtinReference, "--out", filepath.Join(t.TempDir(), "refused"), "--spec-pin", "wrong"},
		{"api", "request", "GET /data/api/v1/gateway-info", "--reference", builtinReference},
		{"api", "request", "POST /anything", "--reference", builtinReference, "--yes", "--allow-stale-spec", "--offline"},
		{"api", "raw", "--path", "/anything", "--reference", builtinReference},
		{"resource", "create", "basic-schedule", "ignored", "--reference", builtinReference, "--yes"},
	} {
		t.Run(strings.Join(args[:2], " ")+"/"+args[len(args)-1], func(t *testing.T) {
			app, out, _ := testApp(t, nil)
			forbidReferenceRuntime(t, &app)
			err := app.Run(context.Background(), append(args, "--json"))
			r := decodeResult(t, out)
			if igwerr.ExitCode(err) != 2 || r.OK || r.Error.Code != 2 {
				t.Fatalf("reference failure contract: %v, %+v", err, r)
			}
		})
	}
}

func TestReferenceExportReportsOutputFailure(t *testing.T) {
	app, out, _ := testApp(t, nil)
	forbidReferenceRuntime(t, &app)
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	err := app.Run(context.Background(), []string{"spec", "references", "export", builtinReference, "--out", filepath.Join(parent, "bundle"), "--json"})
	r := decodeResult(t, out)
	if igwerr.ExitCode(err) != 7 || r.Error.Kind != "artifact" {
		t.Fatalf("output failure misclassified: %v, %+v", err, r)
	}
	data, err := os.ReadFile(parent)
	if err != nil || string(data) != "existing" {
		t.Fatal("failed export changed existing file")
	}
}

func TestReferenceCancellationAndHumanProvenance(t *testing.T) {
	for _, args := range [][]string{
		{"spec", "references", "list"},
		{"spec", "references", "inspect", builtinReference},
		{"spec", "references", "export", builtinReference, "--out", filepath.Join(t.TempDir(), "canceled")},
		{"api", "list", "--reference", builtinReference},
		{"api", "capabilities", "--reference", builtinReference},
	} {
		app, out, _ := testApp(t, nil)
		forbidReferenceRuntime(t, &app)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		err := app.Run(ctx, append(args, "--json"))
		cancel()
		r := decodeResult(t, out)
		if igwerr.ExitCode(err) != 7 || r.Error.Kind != "timeout" {
			t.Fatalf("canceled reference command: %v, %+v", err, r)
		}
	}
	app, out, _ := testApp(t, nil)
	forbidReferenceRuntime(t, &app)
	if err := app.Run(context.Background(), []string{"spec", "references", "list"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), builtinReference+"\t8.3.9") || !strings.Contains(out.String(), "32 modules (32 active)") || !strings.Contains(out.String(), "core-opcua\t32 modules (1 active)") {
		t.Fatal("human listing omitted identity")
	}
	out.Reset()
	if err := app.Run(context.Background(), []string{"spec", "references", "inspect", builtinReference}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "Reference: "+builtinReference+" (8.3.9") {
		t.Fatal("human inspection omitted reference source")
	}
}
