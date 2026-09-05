package project

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type Runner interface {
	Run(execute.Request) result.Result
}

type ImportRequest struct {
	Name            string
	Source          *artifact.Upload
	Overwrite       bool
	IfProjectSHA256 string
	Yes, DryRun     bool
}

type ImportEvidence struct {
	Name          string           `json:"name"`
	Overwrite     bool             `json:"overwrite"`
	UploadSHA256  string           `json:"uploadSha256"`
	ProjectSHA256 string           `json:"projectSha256"`
	BeforeSHA256  string           `json:"beforeSha256,omitempty"`
	AfterSHA256   string           `json:"afterSha256,omitempty"`
	FileCount     int              `json:"fileCount"`
	Precondition  string           `json:"precondition"`
	Request       *execute.Preview `json:"request,omitempty"`
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n") {
		return result.Usage("project name must be a nonempty name without path separators")
	}
	return nil
}

func (r ImportRequest) Validate() error {
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if !r.Yes && !r.DryRun {
		return result.Usage("project import requires --yes; inspect --dry-run first")
	}
	if r.Overwrite && !r.DryRun && r.IfProjectSHA256 == "" {
		return result.Usage("overwriting a project requires --if-project-sha256 from the reviewed preview")
	}
	if !r.Overwrite && r.IfProjectSHA256 != "" {
		return result.Usage("--if-project-sha256 applies only to --overwrite")
	}
	if r.IfProjectSHA256 != "" && (len(r.IfProjectSHA256) != 64 || strings.Trim(r.IfProjectSHA256, "0123456789abcdef") != "") {
		return result.Usage("project digest must be 64 lowercase SHA-256 hex characters")
	}
	return nil
}

func Import(ctx context.Context, runner Runner, input ImportRequest) result.Result {
	if err := input.Validate(); err != nil {
		return result.Failure(err)
	}
	if input.Source == nil {
		return result.Failure(result.Usage("project archive is required"))
	}
	manifest, err := Inspect(ctx, input.Source)
	if err != nil {
		if ctx.Err() != nil {
			return result.Failure(ctx.Err())
		}
		return result.Failure(result.Usage(err.Error()))
	}
	evidence := ImportEvidence{Name: input.Name, Overwrite: input.Overwrite, UploadSHA256: input.Source.SHA256(), ProjectSHA256: manifest.SHA256, FileCount: len(manifest.Files), Precondition: "destination_absent"}
	current := runner.Run(execute.Request{Operation: "GET /data/api/v1/projects/find/{name}", PathParams: map[string]string{"name": input.Name}})
	if !current.OK && !notFound(current) {
		return current
	}
	if current.OK {
		raw, _ := current.Data.(json.RawMessage)
		var metadata struct{ Name string }
		if jsonvalue.Validate(raw) != nil || json.Unmarshal(raw, &metadata) != nil || metadata.Name != input.Name {
			return fail(current, evidence, "project_response", "Gateway returned an unverified project identity", "failed")
		}
	}
	if current.OK && !input.Overwrite {
		return fail(current, evidence, "conflict", "project already exists; preview an explicit --overwrite or select another name", "failed")
	}
	if !current.OK && input.Overwrite {
		return fail(current, evidence, "conflict", "the project selected for replacement no longer exists", "failed")
	}
	if input.Overwrite {
		before, out, cleanup := download(ctx, runner, input.Name)
		defer cleanup()
		if !out.OK {
			return out
		}
		evidence.BeforeSHA256, evidence.Precondition = before.SHA256, "client_observation_only"
		if input.IfProjectSHA256 != "" && input.IfProjectSHA256 != before.SHA256 {
			return fail(out, evidence, "conflict", "project content differs from the reviewed version; prepare a new preview", "failed")
		}
	}
	request := execute.Request{Operation: "POST /data/api/v1/projects/import/{name}", PathParams: map[string]string{"name": input.Name}, Upload: input.Source, ContentType: "application/zip", Yes: input.Yes, DryRun: input.DryRun, Query: url.Values{"overwrite": {"false"}}}
	if input.Overwrite {
		request.Query.Set("overwrite", "true")
	}
	written := runner.Run(request)
	if input.Overwrite {
		written.Meta.Warnings = append(written.Meta.Warnings, "The Gateway project API has no atomic revision precondition. The digest check detects earlier changes but cannot prevent concurrent edits during import.")
	}
	if input.DryRun {
		if written.OK {
			preview, ok := written.Data.(execute.Preview)
			if !ok {
				return fail(written, evidence, "project_response", "project preview unavailable", "failed")
			}
			evidence.Request = &preview
			written.Data = evidence
		}
		return written
	}
	if !written.OK && written.Error != nil && (written.Error.Code == 2 || written.Error.Code == 6) {
		return written
	}
	var ack struct{ Success *bool }
	raw, _ := written.Data.(json.RawMessage)
	acknowledged := written.OK && jsonvalue.Validate(raw) == nil && json.Unmarshal(raw, &ack) == nil && ack.Success != nil && *ack.Success
	after, out, cleanup := download(ctx, runner, input.Name)
	defer cleanup()
	if out.OK {
		evidence.AfterSHA256 = after.SHA256
	}
	if acknowledged && out.OK && after.SHA256 == manifest.SHA256 {
		written.Outcome, written.Data, written.Meta.Verification = "completed", evidence, "verified"
		return written
	}
	failed := fail(written, evidence, "verification", "project import outcome could not be verified; inspect and export the current project before retrying", "uncertain")
	if out.Error != nil {
		failed.Error.Details = map[string]any{"readback": out.Error}
		if out.Error.Code == 6 {
			failed.Error.Code = 6
		}
	}
	return failed
}

// Export validates the complete private download before publishing a user's
// destination. Existing output survives network, ZIP, or validation failures.
func Export(ctx context.Context, runner Runner, name, outPath string, overwrite bool) result.Result {
	if err := ValidateName(name); err != nil {
		return result.Failure(err)
	}
	if outPath == "" {
		return result.Failure(result.Usage("project export requires --out"))
	}
	writer, err := artifact.New(outPath, overwrite)
	if err != nil {
		return result.Failure(result.Usage("could not create project output; an existing file requires --overwrite"))
	}
	defer writer.Abort()
	manifest, got, cleanup := download(ctx, runner, name)
	defer cleanup()
	if !got.OK {
		return got
	}
	file, err := os.Open(got.Artifact.Path)
	if err != nil {
		return result.Failure(&result.Problem{Kind: "artifact", Message: "validated project download is unavailable", Code: 7})
	}
	defer file.Close()
	if _, err := io.Copy(writer, contextReader{ctx, file}); err != nil {
		return result.Failure(&result.Problem{Kind: "artifact", Message: "could not publish project export", Code: 7})
	}
	info, err := writer.Commit()
	if err != nil {
		return result.Failure(&result.Problem{Kind: "artifact", Message: "could not publish project export", Code: 7})
	}
	got.Artifact = &info
	got.Data = map[string]any{"name": name, "projectSha256": manifest.SHA256, "fileCount": len(manifest.Files)}
	got.Meta.Verification = "archive_validated"
	return got
}

func download(ctx context.Context, runner Runner, name string) (Manifest, result.Result, func()) {
	dir, err := os.MkdirTemp("", "igw-project-*")
	if err != nil {
		return Manifest{}, result.Failure(&result.Problem{Kind: "artifact", Message: "could not create private project download", Code: 7}), func() {}
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "project.zip")
	got := runner.Run(execute.Request{Operation: "GET /data/api/v1/projects/export/{name}", PathParams: map[string]string{"name": name}, Out: path, MaxBodyBytes: artifact.DefaultUploadLimit})
	if !got.OK {
		return Manifest{}, got, cleanup
	}
	if got.Artifact == nil || got.Artifact.Path != path {
		return Manifest{}, fail(got, nil, "artifact", "project download receipt is unavailable", "failed"), cleanup
	}
	source, err := artifact.SnapshotUpload(ctx, path, artifact.DefaultUploadLimit)
	if err != nil {
		return Manifest{}, fail(got, nil, "artifact", "could not inspect downloaded project", "failed"), cleanup
	}
	defer source.Close()
	manifest, err := Inspect(ctx, source)
	if err != nil {
		return Manifest{}, fail(got, nil, "project_archive", "Gateway did not provide a supported project archive", "failed"), cleanup
	}
	return manifest, got, cleanup
}

func notFound(out result.Result) bool {
	if out.Error == nil || out.Error.Kind != "http" {
		return false
	}
	b, _ := json.Marshal(out.Error.Details)
	var details struct {
		HTTPStatus int `json:"httpStatus"`
	}
	return json.Unmarshal(b, &details) == nil && details.HTTPStatus == 404
}

func fail(out result.Result, data any, kind, message, outcome string) result.Result {
	out.OK, out.Outcome, out.Data, out.Artifact = false, outcome, data, nil
	out.Error = &result.Problem{Kind: kind, Message: message, Code: 7}
	out.Meta.Verification = "not_verified"
	return out
}
