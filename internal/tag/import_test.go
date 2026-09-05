package tag

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type runFunc func(execute.Request) result.Result

func (f runFunc) Run(r execute.Request) result.Result      { return f(r) }
func (f runFunc) Require(catalog.Capability) result.Result { return result.Success(nil) }
func tagSource(t *testing.T, body string) *artifact.Upload {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	u, err := artifact.SnapshotUpload(context.Background(), path, MaxJSONBytes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { u.Close() })
	return u
}

func TestReportContracts(t *testing.T) {
	for _, raw := range []string{`[]`, `[{"code":1}]`, `{"successCount":2,"failureCount":0,"failures":[]}`, `{"successCount":1,"failureCount":1,"failures":[{"diagnostic":"private"}]}`} {
		if _, err := ParseReport([]byte(raw)); err != nil {
			t.Fatalf("valid report %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `true`, `{"successCount":1,"failureCount":0}`, `{"successCount":-1,"failureCount":0,"failures":[]}`, `{"successCount":1,"failureCount":1,"failures":[]}`, `{"successCount":1,"failureCount":0,"failures":null}`, `{"successCount":1,"successCount":0,"failureCount":0,"failures":[]}`, `{"successCount":1,"SuccessCount":0,"failureCount":0,"failures":[]}`, `{"SuccessCount":1,"failureCount":0,"failures":[]}`} {
		if _, err := ParseReport([]byte(raw)); err == nil {
			t.Fatalf("ambiguous report accepted: %s", raw)
		}
	}
}

const tree = `{"tags":[{"name":"Folder","tagType":"Folder","tags":[{"name":"Counter","value":9007199254740993,"tagType":"AtomicTag"}]}]}`
const observed = `{"name":"Folder","tagType":"Folder","tags":[{"name":"Counter","value":9007199254740993,"tagType":"AtomicTag","defaultValue":7}]}`

func TestDocumentIdentityAndExactProperties(t *testing.T) {
	want, err := Decode([]byte(tree))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode([]byte(observed))
	if err != nil {
		t.Fatal(err)
	}
	if want.Count != 2 || !Matches(want.Roots[0], got.Roots[0]) {
		t.Fatal("defaults or export shape changed comparison")
	}
	changed, err := Decode([]byte(strings.ReplaceAll(observed, "9007199254740993", "9007199254740992")))
	if err != nil {
		t.Fatal(err)
	}
	if Matches(want.Roots[0], changed.Roots[0]) {
		t.Fatal("numeric precision lost")
	}
	for _, raw := range []string{`{"tags":[{"name":"same"},{"name":"same"}]}`, `{"name":"Folder","tags":[{"name":"same"},{"name":"same"}]}`, `{"tags":[{"name":"../other"}]}`, `{"tags":null}`, `{"tags":[null]}`, `{"name":"x","tags":null}`, `{"name":"x","name":"y"}`, `[]`} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("invalid tag structure accepted: %s", raw)
		}
	}
}

func TestImportOutcomesAndSingleDispatch(t *testing.T) {
	for _, scenario := range []string{"verified", "legacy", "mismatch", "failure", "partial", "unknown", "read auth", "preview", "rename", "xml"} {
		t.Run(scenario, func(t *testing.T) {
			input := ImportRequest{Provider: "default", Path: "base", Format: "json", CollisionPolicy: "Abort", Source: tagSource(t, tree), Yes: true}
			if scenario == "preview" {
				input.DryRun = true
				input.Yes = false
			}
			if scenario == "rename" {
				input.CollisionPolicy = "Rename"
			}
			if scenario == "xml" {
				input.Format = "xml"
				input.Source = tagSource(t, "<Tags/>")
			}
			writes, reads := 0, 0
			runner := runFunc(func(r execute.Request) result.Result {
				switch r.Operation {
				case "POST /data/api/v1/tags/import":
					if r.DryRun {
						out := result.Success(execute.Preview{})
						out.Outcome = "preview"
						return out
					}
					writes++
					raw := `{"successCount":2,"failureCount":0,"failures":[]}`
					switch scenario {
					case "legacy":
						raw = `[]`
					case "failure":
						raw = `{"successCount":0,"failureCount":1,"failures":[{"diagnostic":"never-expose-this"}]}`
					case "partial":
						raw = `{"successCount":1,"failureCount":1,"failures":[{"diagnostic":"never-expose-this"}]}`
					case "unknown":
						raw = `{"success":true}`
					}
					out := result.Success(json.RawMessage(raw))
					out.Outcome = "accepted"
					return out
				case "GET /data/api/v1/tags/export":
					reads++
					if r.Query.Get("path") != "base/Folder" || r.Query.Get("provider") != "default" {
						t.Fatal("wrong verification target")
					}
					if scenario == "read auth" {
						return result.Failure(&result.Problem{Kind: "auth", Code: 6})
					}
					raw := observed
					if scenario == "mismatch" {
						raw = strings.ReplaceAll(raw, "9007199254740993", "9007199254740992")
					}
					return result.Success(json.RawMessage(raw))
				default:
					t.Fatal(r.Operation)
					return result.Result{}
				}
			})
			out := Import(context.Background(), runner, input)
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "never-expose-this") {
				t.Fatal("diagnostic leaked")
			}
			if scenario == "preview" {
				if !out.OK || out.Outcome != "preview" || writes != 0 || reads != 0 || out.Data.(Evidence).Request == nil {
					t.Fatalf("%+v", out)
				}
				return
			}
			if writes != 1 {
				t.Fatal("mutation replayed")
			}
			switch scenario {
			case "verified", "legacy":
				if !out.OK || out.Outcome != "completed" || out.Meta.Verification != "verified" || reads != 1 || out.Data.(Evidence).VerifiedRoots != 1 {
					t.Fatalf("%+v", out)
				}
			case "rename", "xml":
				if !out.OK || out.Outcome != "accepted" || out.Meta.Verification != "unavailable" || reads != 0 || len(out.Meta.Warnings) == 0 {
					t.Fatalf("%+v", out)
				}
			case "failure", "partial":
				want := "failed"
				if scenario == "partial" {
					want = "partial"
				}
				if out.OK || out.Outcome != want || out.Error.Code != 7 || reads != 0 {
					t.Fatalf("%+v", out)
				}
			default:
				code := 7
				if scenario == "read auth" {
					code = 6
				}
				if out.OK || out.Outcome != "uncertain" || out.Error.Code != code {
					t.Fatalf("%+v", out)
				}
			}
		})
	}
}
