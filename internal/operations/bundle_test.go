package operations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type runFunc func(execute.Request) result.Result

func (f runFunc) Run(r execute.Request) result.Result { return f(r) }
func statusResult(state string, size int64) result.Result {
	b, _ := json.Marshal(map[string]any{"state": state, "fileSize": size})
	return result.Success(json.RawMessage(b))
}

func TestBundleStatesRequireQualifiedStateAndSize(t *testing.T) {
	for _, raw := range []string{`{}`, `{"state":"Valid"}`, `{"state":"Valid","fileSize":0}`, `{"state":"Valid","fileSize":null}`, `{"state":"Unknown","fileSize":5}`, `{"state":"Valid","fileSize":-1}`, `{"state":"Invalid","fileSize":null}`, `{"state":"Generating","fileSize":-1}`, `{"state":"Valid","fileSize":5,"state":"Generating"}`} {
		if _, err := ParseBundleStatus([]byte(raw)); err == nil {
			t.Fatalf("invalid status accepted: %s", raw)
		}
	}
	for raw, want := range map[string]string{`{"state":"Invalid"}`: "empty", `{"state":"Generating"}`: "generating"} {
		state, err := ParseBundleStatus([]byte(raw))
		if err != nil || state.State != want || state.FileSize != 0 {
			t.Fatalf("absent non-ready size rejected: %s", raw)
		}
	}
	state, err := ParseBundleStatus([]byte(`{"state":"Generating","fileSize":500}`))
	if err != nil || state.State != "generating" {
		t.Fatal("positive old size made a generating bundle ready")
	}
}

func TestBundleWorkflowOutcomesAndAtomicPublication(t *testing.T) {
	for _, scenario := range []string{"complete", "ready acknowledgement", "transient", "timeout", "busy", "preview", "size mismatch", "state changed", "auth", "unknown", "generate failure", "existing output", "download"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			outPath := filepath.Join(dir, "bundle.zip")
			if err := os.WriteFile(outPath, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			input := BundleRequest{Generate: true, Yes: true, Out: outPath, Overwrite: true, MaxBytes: 1024, Interval: 100 * time.Millisecond}
			if scenario == "preview" {
				input.Yes = false
				input.DryRun = true
				input.Out = filepath.Join(dir, "missing", "bundle.zip")
			}
			if scenario == "existing output" {
				input.Overwrite = false
			}
			if scenario == "download" {
				input.Generate = false
			}
			writes, reads, downloads := 0, 0, 0
			runner := runFunc(func(r execute.Request) result.Result {
				switch r.Operation {
				case "GET /data/api/v1/diagnostics/bundle/status":
					reads++
					if reads == 1 {
						if scenario == "busy" {
							return statusResult("Generating", 5)
						}
						if scenario == "download" {
							return statusResult("Valid", 5)
						}
						return result.Success(json.RawMessage(`{"state":"Invalid"}`))
					}
					if scenario == "timeout" {
						cancel()
						return statusResult("Generating", 5)
					}
					if reads == 2 && scenario == "transient" {
						return result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 503}})
					}
					if reads == 2 && scenario == "complete" {
						return statusResult("Generating", 500)
					}
					if scenario == "unknown" {
						return statusResult("Maybe", 5)
					}
					if scenario == "state changed" && downloads > 0 {
						return statusResult("Generating", 5)
					}
					return statusResult("Valid", 5)
				case "POST /data/api/v1/diagnostics/bundle/generate":
					if r.DryRun {
						out := result.Success(execute.Preview{})
						out.Outcome = "preview"
						return out
					}
					writes++
					if scenario == "generate failure" {
						return result.Failure(&result.Problem{Kind: "http", Code: 7, Details: map[string]int{"httpStatus": 500}})
					}
					if scenario == "ready acknowledgement" {
						return result.Success(json.RawMessage(`{"state":"Valid"}`))
					}
					return result.Success(json.RawMessage(`{"state":"Generating"}`))
				case "GET /data/api/v1/diagnostics/bundle/download":
					downloads++
					if scenario == "auth" {
						return result.Failure(&result.Problem{Kind: "auth", Code: 6})
					}
					if r.Out == input.Out {
						t.Fatal("download published without verification")
					}
					body := []byte("bytes")
					if scenario == "size mismatch" {
						body = []byte("bad")
					}
					if err := os.WriteFile(r.Out, body, 0600); err != nil {
						t.Fatal(err)
					}
					out := result.Success(nil)
					out.Artifact = &artifact.Info{Path: r.Out, Bytes: int64(len(body))}
					return out
				default:
					t.Fatal(r.Operation)
					return result.Result{}
				}
			})
			out := Bundle(ctx, runner, input)
			if writes > 1 || downloads > 1 {
				t.Fatal("generation or download replayed")
			}
			body, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "complete", "ready acknowledgement", "transient", "download":
				if !out.OK || out.Outcome != "completed" || out.Meta.Verification != "size_matched" || string(body) != "bytes" || out.Artifact == nil || len(out.Meta.Warnings) == 0 {
					t.Fatalf("%+v", out)
				}
				if scenario == "complete" && reads != 4 {
					t.Fatal("stale size bypassed state polling")
				}
				if scenario == "download" && writes != 0 {
					t.Fatal("download started generation")
				}
				if scenario == "ready acknowledgement" && (out.Data.(BundleEvidence).GenerationState != "Valid" || len(out.Meta.Warnings) < 2) {
					t.Fatal("ready acknowledgement implied a new generation job")
				}
			case "preview":
				if !out.OK || out.Outcome != "preview" || writes != 0 || downloads != 0 || string(body) != "keep" {
					t.Fatalf("%+v", out)
				}
				if _, err := os.Stat(filepath.Dir(input.Out)); !os.IsNotExist(err) {
					t.Fatal("preview created output directory")
				}
			default:
				if out.OK || string(body) != "keep" || out.Artifact != nil {
					t.Fatalf("failed bundle replaced output: %+v", out)
				}
				code := 7
				if scenario == "auth" {
					code = 6
				}
				if scenario == "existing output" {
					code = 2
				}
				if out.Error.Code != code {
					t.Fatalf("wrong exit: %+v", out.Error)
				}
				if scenario == "busy" || scenario == "existing output" {
					if writes != 0 || downloads != 0 {
						t.Fatal("refused request started job")
					}
				} else if out.Outcome != "uncertain" {
					t.Fatalf("unverified generation claimed certainty: %+v", out)
				}
				if scenario == "timeout" && (out.Error.Kind != "canceled" || downloads != 0) {
					t.Fatal("canceled poll downloaded old bundle")
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatal("private output staging leaked")
			}
		})
	}
}
