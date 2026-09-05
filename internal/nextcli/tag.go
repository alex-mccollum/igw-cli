package nextcli

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/tag"
)

func (i *invocation) tagCommands() *cobra.Command {
	group := &cobra.Command{Use: "tag", Short: "Export tags and import with explicit collision policies"}
	var provider, path, format, out string
	var recursive, udts, overwrite bool
	export := &cobra.Command{Use: "export", Short: "Stream a complete tag export to an atomic file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := tag.ValidateTarget(provider, path); err != nil {
			return err
		}
		if format != "json" && format != "xml" {
			return result.Usage("tag export type must be json or xml")
		}
		query := url.Values{"provider": {provider}, "type": {format}, "recursive": {strconv.FormatBool(recursive)}, "includeUdts": {strconv.FormatBool(udts)}}
		if path != "" {
			query.Set("path", path)
		}
		return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/tags/export", Query: query, Out: out, Overwrite: overwrite})
	}}
	f := export.Flags()
	f.StringVar(&provider, "provider", "default", "Tag provider")
	f.StringVar(&path, "path", "", "Root tag path within the provider")
	f.StringVar(&format, "type", "json", "Export format: json or xml")
	f.StringVar(&out, "out", "", "Destination file")
	_ = export.MarkFlagRequired("out")
	f.BoolVar(&recursive, "recursive", true, "Include child tags")
	f.BoolVar(&udts, "include-udts", true, "Include user-defined types")
	f.BoolVar(&overwrite, "overwrite", false, "Replace an existing output file after the complete download")
	var input string
	var change tag.ImportRequest
	importCmd := &cobra.Command{Use: "import", Short: "Import tags and verify supported JSON workflows", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if change.Format == "" {
			change.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(input)), ".")
			if change.Format == "" {
				change.Format = "json"
			}
		}
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
			return tag.Import(ctx, scope, change)
		})
	}}
	f = importCmd.Flags()
	f.StringVar(&input, "in", "", "Source JSON, XML, or CSV file")
	_ = importCmd.MarkFlagRequired("in")
	f.StringVar(&change.Provider, "provider", "default", "Tag provider")
	f.StringVar(&change.Path, "path", "", "Destination folder within the provider")
	f.StringVar(&change.Format, "type", "", "Import format; inferred from the file extension")
	f.StringVar(&change.CollisionPolicy, "collision-policy", "Abort", "Abort, Overwrite, Rename, Ignore, or MergeOverwrite")
	f.BoolVar(&change.DryRun, "dry-run", false, "Inspect the proposed import without mutating tags")
	f.BoolVar(&change.Yes, "yes", false, "Confirm the tag import")
	group.AddCommand(export, importCmd)
	return group
}
