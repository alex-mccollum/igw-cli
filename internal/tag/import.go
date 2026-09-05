// Package tag handles opaque tag transfers with explicit outcome verification.
package tag

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/jsonvalue"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type Runner interface {
	Run(execute.Request) result.Result
	Require(catalog.Capability) result.Result
}
type ImportRequest struct {
	Provider, Path, Format, CollisionPolicy string
	Source                                  *artifact.Upload
	DryRun, Yes                             bool
}
type Report struct {
	Format       string `json:"format"`
	SuccessCount *int   `json:"successCount,omitempty"`
	FailureCount int    `json:"failureCount"`
}
type Evidence struct {
	Provider        string           `json:"provider"`
	Path            string           `json:"path"`
	Format          string           `json:"format"`
	CollisionPolicy string           `json:"collisionPolicy"`
	UploadSHA256    string           `json:"uploadSha256"`
	TagCount        int              `json:"tagCount,omitempty"`
	VerifiedRoots   int              `json:"verifiedRoots"`
	Roots           []string         `json:"roots,omitempty"`
	Report          *Report          `json:"report,omitempty"`
	Request         *execute.Preview `json:"request,omitempty"`
}

func ValidateTarget(provider, path string) error {
	if strings.TrimSpace(provider) == "" || strings.ContainsAny(provider, "[]\x00\r\n") || strings.ContainsAny(path, "\x00\r\n") || strings.HasPrefix(path, "[") {
		return result.Usage("select a provider and a relative tag path without a provider prefix")
	}
	return nil
}
func (r ImportRequest) Validate() error {
	if err := ValidateTarget(r.Provider, r.Path); err != nil {
		return err
	}
	if r.Format != "json" && r.Format != "xml" && r.Format != "csv" {
		return result.Usage("tag import type must be json, xml, or csv")
	}
	switch r.CollisionPolicy {
	case "Abort", "Overwrite", "Rename", "Ignore", "MergeOverwrite":
	default:
		return result.Usage("collision policy must be Abort, Overwrite, Rename, Ignore, or MergeOverwrite")
	}
	if !r.Yes && !r.DryRun {
		return result.Usage("tag import requires --yes; inspect --dry-run first")
	}
	return nil
}

// ParseReport supports the documented legacy quality-code array and the
// observed 8.3.9 count envelope. Unknown, contradictory, and ambiguous results
// cannot establish success. Diagnostic strings stay out of CLI error output.
func ParseReport(raw []byte) (Report, error) {
	if err := jsonvalue.Validate(raw); err != nil {
		return Report{}, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var failures []json.RawMessage
		if json.Unmarshal(raw, &failures) != nil {
			return Report{}, result.Usage("invalid tag import report")
		}
		return Report{Format: "quality_codes", FailureCount: len(failures)}, nil
	}
	var fields map[string]json.RawMessage
	var success, failure *int
	var failures *[]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields["successCount"], &success) != nil || json.Unmarshal(fields["failureCount"], &failure) != nil || json.Unmarshal(fields["failures"], &failures) != nil || success == nil || failure == nil || failures == nil || *success < 0 || *failure < 0 || *failure != len(*failures) {
		return Report{}, result.Usage("unrecognized tag import report")
	}
	return Report{Format: "counts", SuccessCount: success, FailureCount: *failure}, nil
}

func Import(ctx context.Context, runner Runner, input ImportRequest) result.Result {
	if err := input.Validate(); err != nil {
		return result.Failure(err)
	}
	if input.Source == nil || input.Source.Bytes() == 0 {
		return result.Failure(result.Usage("a nonempty tag import file is required"))
	}
	if required := runner.Require(importCapability(input.verifiedJSON())); !required.OK {
		return required
	}
	evidence := Evidence{Provider: input.Provider, Path: input.Path, Format: input.Format, CollisionPolicy: input.CollisionPolicy, UploadSHA256: input.Source.SHA256()}
	var expected Document
	if input.Format == "json" {
		if input.Source.Bytes() > MaxJSONBytes {
			return result.Failure(result.Usage("verified JSON tag imports are limited to 32 MiB; use explicit api upload for larger opaque inputs"))
		}
		reader, err := input.Source.Open(ctx)
		if err != nil {
			return result.Failure(err)
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return result.Failure(err)
		}
		expected, err = Decode(raw)
		if err != nil {
			return result.Failure(err)
		}
		if len(expected.Roots) == 0 {
			return result.Failure(result.Usage("tag import requires at least one tag"))
		}
		evidence.TagCount = expected.Count
		for _, node := range expected.Roots {
			evidence.Roots = append(evidence.Roots, node.Name)
		}
	}
	query := url.Values{"provider": {input.Provider}, "type": {input.Format}, "collisionPolicy": {input.CollisionPolicy}}
	if input.Path != "" {
		query.Set("path", input.Path)
	}
	written := runner.Run(execute.Request{Operation: importOperation, Query: query, Upload: input.Source, ContentType: "application/octet-stream", DryRun: input.DryRun, Yes: input.Yes})
	if input.DryRun {
		if written.OK {
			preview, ok := written.Data.(execute.Preview)
			if !ok {
				return failed(written, evidence, "tag_response", "tag preview unavailable", "failed")
			}
			evidence.Request = &preview
			written.Data = evidence
		}
		return written
	}
	if !written.OK {
		written.Data = evidence
		return written
	}
	raw, _ := written.Data.(json.RawMessage)
	report, err := ParseReport(raw)
	if err != nil {
		return failed(written, evidence, "tag_response", "Gateway returned an unrecognized tag import report; inspect tags before retrying", "uncertain")
	}
	evidence.Report = &report
	if report.FailureCount > 0 {
		outcome := "failed"
		if report.SuccessCount != nil && *report.SuccessCount > 0 {
			outcome = "partial"
		}
		return failed(written, evidence, "tag_import", "Gateway reported tag import failures; inspect current tags before retrying", outcome)
	}
	if !input.verifiedJSON() {
		written.Data = evidence
		written.Outcome = "accepted"
		written.Meta.Verification = "unavailable"
		written.Meta.Warnings = append(written.Meta.Warnings, "The Gateway reported no import failures. Independent verification is available for JSON with Abort, Overwrite, or MergeOverwrite; inspect exported tags for this format or collision policy.")
		return written
	}
	for _, node := range expected.Roots {
		path := node.Name
		if input.Path != "" {
			path = strings.TrimRight(input.Path, "/") + "/" + path
		}
		read := runner.Run(execute.Request{Operation: exportOperation, Query: url.Values{"provider": {input.Provider}, "type": {"json"}, "path": {path}, "recursive": {"true"}, "includeUdts": {"true"}}, MaxBodyBytes: MaxJSONBytes})
		if !read.OK {
			out := failed(written, evidence, "verification", "tag import was acknowledged but readback failed; inspect tags before retrying", "uncertain")
			out.Error.Details = map[string]any{"readback": read.Error}
			if read.Error != nil && read.Error.Code == 6 {
				out.Error.Code = 6
			}
			return out
		}
		raw, _ := read.Data.(json.RawMessage)
		observed, err := Decode(raw)
		matched := false
		if err == nil {
			for _, root := range observed.Roots {
				if Matches(node, root) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return failed(written, evidence, "verification", "exported tags differ from supplied properties; inspect current tags before retrying", "uncertain")
		}
		evidence.VerifiedRoots++
	}
	written.Data, written.Outcome, written.Meta.Verification = evidence, "completed", "verified"
	return written
}

func failed(out result.Result, evidence Evidence, kind, message, outcome string) result.Result {
	out.OK, out.Outcome, out.Data = false, outcome, evidence
	out.Error = &result.Problem{Kind: kind, Message: message, Code: 7}
	out.Meta.Verification = "not_verified"
	return out
}
