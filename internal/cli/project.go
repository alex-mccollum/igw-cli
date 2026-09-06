package cli

import (
	"context"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/project"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) projectCommands() *cobra.Command {
	group := &cobra.Command{Use: "project", Short: "Inspect, export, and import complete project archives"}
	var limit, offset int
	var filters []string
	list := &cobra.Command{Use: "list", Short: "List one page of project metadata", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || limit > 1000 || offset < 0 {
			return result.Usage("limit must be 1..1000 and offset must be nonnegative")
		}
		query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
		if err := addFilters(query, filters); err != nil {
			return err
		}
		return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/projects/list", Query: query})
	}}
	list.Flags().IntVar(&limit, "limit", 50, "Maximum projects in this page (1..1000)")
	list.Flags().IntVar(&offset, "offset", 0, "Projects to skip")
	list.Flags().StringArrayVar(&filters, "filter", nil, "Filter field[operator]=value; repeat for different keys")
	get := &cobra.Command{Use: "get NAME", Short: "Read a project's metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := project.ValidateName(args[0]); err != nil {
			return err
		}
		return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/projects/find/{name}", PathParams: map[string]string{"name": args[0]}})
	}}
	inspect := &cobra.Command{Use: "inspect FILE", Short: "Validate a local project archive and fingerprint its files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
		if err != nil {
			return err
		}
		defer cancel()
		source, err := artifact.SnapshotUpload(ctx, args[0], artifact.DefaultUploadLimit)
		if err != nil {
			return inputProblem(err)
		}
		defer source.Close()
		manifest, err := project.Inspect(ctx, source)
		if err != nil {
			return inputProblem(err)
		}
		i.output = result.Success(manifest)
		return nil
	}}
	var out string
	var overwrite bool
	export := &cobra.Command{Use: "export NAME", Short: "Download and validate a project ZIP before publishing it", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := project.ValidateName(args[0]); err != nil {
			return err
		}
		return i.runWorkflow(cmd, false, func(ctx context.Context, scope *execute.Scope) result.Result {
			return project.Export(ctx, scope, args[0], out, overwrite)
		})
	}}
	export.Flags().StringVar(&out, "out", "", "Destination ZIP file")
	_ = export.MarkFlagRequired("out")
	export.Flags().BoolVar(&overwrite, "overwrite", false, "Replace an existing output file after archive validation")
	var input string
	var change project.ImportRequest
	importCmd := &cobra.Command{Use: "import NAME", Short: "Import a ZIP and verify the resulting project files", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		change.Name = args[0]
		if err := change.Validate(); err != nil {
			return err
		}
		return i.runWorkflow(cmd, !change.DryRun, func(ctx context.Context, scope *execute.Scope) result.Result {
			source, err := artifact.SnapshotUpload(ctx, input, artifact.DefaultUploadLimit)
			if err != nil {
				return result.Failure(inputProblem(err))
			}
			defer source.Close()
			change.Source = source
			return project.Import(ctx, scope, change)
		})
	}}
	f := importCmd.Flags()
	f.StringVar(&input, "in", "", "Source project ZIP file")
	_ = importCmd.MarkFlagRequired("in")
	f.BoolVar(&change.Overwrite, "overwrite", false, "Replace an existing Gateway project")
	f.StringVar(&change.IfProjectSHA256, "if-project-sha256", "", "Require the beforeSha256 from a reviewed replacement preview; this check is not atomic")
	f.BoolVar(&change.DryRun, "dry-run", false, "Inspect the archive and current project without importing")
	f.BoolVar(&change.Yes, "yes", false, "Confirm the project import")
	group.AddCommand(list, get, inspect, export, importCmd)
	return group
}
