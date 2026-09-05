package project

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type runFunc func(execute.Request) result.Result

func (f runFunc) Run(r execute.Request) result.Result { return f(r) }

func projectSource(t *testing.T, title string) *artifact.Upload {
	t.Helper()
	return archiveFixture(t, []struct{ name, body string }{{"project.json", `{"title":"` + title + `","enabled":false}`}, {"data.bin", "project content"}}, time.Unix(0, 0))
}

func downloaded(t *testing.T, r execute.Request, source *artifact.Upload) result.Result {
	t.Helper()
	reader, err := source.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	w, err := artifact.New(r.Out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Abort()
	if _, err := io.Copy(w, reader); err != nil {
		t.Fatal(err)
	}
	info, err := w.Commit()
	if err != nil {
		t.Fatal(err)
	}
	out := result.Success(nil)
	out.Artifact = &info
	return out
}

func missing() result.Result {
	return result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 404}})
}

func TestImportVerifiesFilesAndNeverReplays(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "different readback"}[mismatch], func(t *testing.T) {
			source := projectSource(t, "intended")
			observed := source
			if mismatch {
				observed = projectSource(t, "changed")
			}
			writes, reads := 0, 0
			runner := runFunc(func(r execute.Request) result.Result {
				switch r.Operation {
				case "GET /data/api/v1/projects/find/{name}":
					return missing()
				case "POST /data/api/v1/projects/import/{name}":
					writes++
					if r.Upload != source || !r.Yes || r.Query.Get("overwrite") != "false" {
						t.Fatal("incorrect import intent")
					}
					return result.Success(json.RawMessage(`{"success":true}`))
				case "GET /data/api/v1/projects/export/{name}":
					reads++
					return downloaded(t, r, observed)
				default:
					t.Fatal(r.Operation)
					return result.Result{}
				}
			})
			out := Import(context.Background(), runner, ImportRequest{Name: "copy", Source: source, Yes: true})
			if writes != 1 || reads != 1 {
				t.Fatal("write replayed or verification skipped")
			}
			if mismatch {
				if out.OK || out.Outcome != "uncertain" || out.Error.Code != 7 {
					t.Fatalf("%+v", out)
				}
			} else if !out.OK || out.Outcome != "completed" || out.Meta.Verification != "verified" {
				t.Fatalf("%+v", out)
			}
		})
	}
}

func TestReplacementPreconditionsAndPreview(t *testing.T) {
	source, before := projectSource(t, "next"), projectSource(t, "current")
	manifest, err := Inspect(context.Background(), before)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"preview", "stale", "missing digest", "replace", "wrong identity", "no overwrite"} {
		t.Run(scenario, func(t *testing.T) {
			writes := 0
			runner := runFunc(func(r execute.Request) result.Result {
				switch r.Operation {
				case "GET /data/api/v1/projects/find/{name}":
					if scenario == "wrong identity" {
						return result.Success(json.RawMessage(`{"name":"other"}`))
					}
					return result.Success(json.RawMessage(`{"name":"copy"}`))
				case "GET /data/api/v1/projects/export/{name}":
					observed := before
					if writes > 0 {
						observed = source
					}
					return downloaded(t, r, observed)
				case "POST /data/api/v1/projects/import/{name}":
					if r.DryRun {
						out := result.Success(execute.Preview{})
						out.Outcome = "preview"
						return out
					}
					writes++
					return result.Success(json.RawMessage(`{"success":true}`))
				default:
					t.Fatal(r.Operation)
					return result.Result{}
				}
			})
			input := ImportRequest{Name: "copy", Source: source, Overwrite: true, IfProjectSHA256: manifest.SHA256, Yes: true}
			switch scenario {
			case "preview":
				input.DryRun = true
				input.Yes = false
				input.IfProjectSHA256 = ""
			case "stale":
				input.IfProjectSHA256 = strings.Repeat("0", 64)
			case "missing digest":
				input.IfProjectSHA256 = ""
			case "no overwrite":
				input.Overwrite = false
				input.IfProjectSHA256 = ""
			}
			out := Import(context.Background(), runner, input)
			switch scenario {
			case "preview":
				if !out.OK || writes != 0 || out.Data.(ImportEvidence).BeforeSHA256 != manifest.SHA256 || out.Outcome != "preview" || len(out.Meta.Warnings) == 0 {
					t.Fatalf("%+v", out)
				}
			case "replace":
				if !out.OK || writes != 1 || out.Meta.Verification != "verified" || len(out.Meta.Warnings) == 0 {
					t.Fatalf("%+v", out)
				}
			default:
				if out.OK || writes != 0 {
					t.Fatalf("unsafe replacement: %+v", out)
				}
			}
		})
	}
}

func TestExportPreservesDestinationOnInvalidArchive(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "existing.zip")
	if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := runFunc(func(r execute.Request) result.Result {
		if err := os.WriteFile(r.Out, []byte("invalid zip"), 0600); err != nil {
			t.Fatal(err)
		}
		out := result.Success(nil)
		out.Artifact = &artifact.Info{Path: r.Out}
		return out
	})
	out := Export(context.Background(), runner, "source", destination, true)
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "keep" || out.OK || out.Artifact != nil {
		t.Fatalf("invalid export published: %+v", out)
	}
}

func TestCanceledAndInvalidProjectInputsNeverDispatch(t *testing.T) {
	source := projectSource(t, "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := runFunc(func(execute.Request) result.Result { t.Fatal("dispatched canceled input"); return result.Result{} })
	out := Import(ctx, runner, ImportRequest{Name: "copy", Source: source, Yes: true})
	if out.OK || out.Error.Kind != "canceled" || out.Error.Code != 7 {
		t.Fatalf("%+v", out)
	}
}

func TestImportRequiresAcknowledgementAndReadback(t *testing.T) {
	for _, scenario := range []string{"false acknowledgement", "transport", "read auth"} {
		t.Run(scenario, func(t *testing.T) {
			source := projectSource(t, "test")
			writes := 0
			runner := runFunc(func(r execute.Request) result.Result {
				switch r.Operation {
				case "GET /data/api/v1/projects/find/{name}":
					return missing()
				case "POST /data/api/v1/projects/import/{name}":
					writes++
					if scenario == "transport" {
						return result.Failure(&result.Problem{Kind: "transport", Code: 7})
					}
					if scenario == "false acknowledgement" {
						return result.Success(json.RawMessage(`{"success":false}`))
					}
					return result.Success(json.RawMessage(`{"success":true}`))
				case "GET /data/api/v1/projects/export/{name}":
					if scenario == "read auth" {
						return result.Failure(&result.Problem{Kind: "auth", Code: 6})
					}
					return downloaded(t, r, source)
				default:
					t.Fatal(r.Operation)
					return result.Result{}
				}
			})
			out := Import(context.Background(), runner, ImportRequest{Name: "copy", Source: source, Yes: true})
			code := 7
			if scenario == "read auth" {
				code = 6
			}
			if out.OK || out.Outcome != "uncertain" || out.Error.Code != code || writes != 1 {
				t.Fatalf("%+v", out)
			}
		})
	}
}
