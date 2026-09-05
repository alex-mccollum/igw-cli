package tag

import (
	"net/url"
	"strconv"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

type ExportRequest struct {
	Provider, Path, Format, Out       string
	Recursive, IncludeUDTs, Overwrite bool
}

func (r ExportRequest) Validate() error {
	if err := ValidateTarget(r.Provider, r.Path); err != nil {
		return err
	}
	if r.Format != "json" && r.Format != "xml" {
		return result.Usage("tag export type must be json or xml")
	}
	if r.Out == "" {
		return result.Usage("tag export requires a destination file")
	}
	return nil
}

func Export(runner Runner, input ExportRequest) result.Result {
	if err := input.Validate(); err != nil {
		return result.Failure(err)
	}
	if required := runner.Require(workflow.TagExport()); !required.OK {
		return required
	}
	query := url.Values{"provider": {input.Provider}, "type": {input.Format}, "recursive": {strconv.FormatBool(input.Recursive)}, "includeUdts": {strconv.FormatBool(input.IncludeUDTs)}}
	if input.Path != "" {
		query.Set("path", input.Path)
	}
	return runner.Run(execute.Request{Operation: workflow.TagExportOperation, Query: query, Out: input.Out, Overwrite: input.Overwrite})
}
