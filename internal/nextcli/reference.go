package nextcli

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/reference"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) referenceCommands() *cobra.Command {
	group := &cobra.Command{Use: "references", Short: "Discover and preserve qualified offline API references"}
	group.AddCommand(&cobra.Command{Use: "list", Short: "List bundled references without Gateway configuration", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
			if err != nil {
				return err
			}
			defer cancel()
			items, err := reference.List(ctx)
			if err != nil {
				return referenceProblem(err)
			}
			i.output = result.Success(items)
			return nil
		}})
	group.AddCommand(&cobra.Command{Use: "inspect REFERENCE", Short: "Verify and inspect a bundled name or local bundle directory", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
			if err != nil {
				return err
			}
			defer cancel()
			bundle := reference.Select(args[0])
			m, err := bundle.Read(ctx)
			if err != nil {
				return referenceProblem(err)
			}
			if err := i.referencePin(m); err != nil {
				return err
			}
			summary := bundle.Summary(m)
			i.output = result.Success(m)
			i.output.Meta.Reference = &summary
			return nil
		}})
	var out string
	export := &cobra.Command{Use: "export REFERENCE", Short: "Copy a complete reference into a new directory", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				return result.Usage("--out requires a new directory")
			}
			ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
			if err != nil {
				return err
			}
			defer cancel()
			bundle := reference.Select(args[0])
			m, err := bundle.Read(ctx)
			if err != nil {
				return referenceProblem(err)
			}
			if err := i.referencePin(m); err != nil {
				return err
			}
			m, err = bundle.Export(ctx, out, m.Catalog)
			if errors.Is(err, os.ErrExist) {
				return result.Usage("reference export requires a new directory; previous bundles are preserved")
			}
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return result.FromError(err)
				}
				return &result.Problem{Kind: "artifact", Message: "could not export reference bundle; inspect the destination directory", Code: 7}
			}
			summary := bundle.Summary(m)
			i.output = result.Success(referenceExport{Directory: out, FileCount: len(m.Files) + 1})
			i.output.Meta.Reference = &summary
			return nil
		}}
	export.Flags().StringVar(&out, "out", "", "New directory for the manifest, exact document, and qualification evidence")
	_ = export.MarkFlagRequired("out")
	group.AddCommand(export)
	return group
}

type referenceExport struct {
	Directory string `json:"directory"`
	FileCount int    `json:"fileCount"`
}

// discovery is deliberately reachable only from API list/describe. References
// are never installed in the target cache or passed to request execution.
func (i *invocation) discovery(cmd *cobra.Command) (*catalog.Catalog, result.Metadata, error) {
	if !cmd.Flags().Changed("reference") {
		snapshot, err := i.snapshot(cmd, false)
		if err != nil {
			return nil, result.Metadata{}, err
		}
		return snapshot.Catalog, result.Metadata{Target: &snapshot.Metadata.Target, Catalog: &snapshot.Metadata, Stale: snapshot.Stale, Warnings: snapshot.Warnings}, nil
	}
	selector, _ := cmd.Flags().GetString("reference")
	if selector == "" {
		return nil, result.Metadata{}, result.Usage("--reference requires a bundled name or local bundle directory")
	}
	ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
	if err != nil {
		return nil, result.Metadata{}, err
	}
	defer cancel()
	bundle := reference.Select(selector)
	m, err := bundle.Read(ctx)
	if err != nil {
		return nil, result.Metadata{}, referenceProblem(err)
	}
	if err := i.referencePin(m); err != nil {
		return nil, result.Metadata{}, err
	}
	m, c, err := bundle.OpenCatalog(ctx)
	if err != nil {
		return nil, result.Metadata{}, referenceProblem(err)
	}
	if err := i.referencePin(m); err != nil {
		c.Close()
		return nil, result.Metadata{}, err
	}
	summary := bundle.Summary(m)
	summary.InspectionParserVersion = catalog.ParserVersion
	return c, result.Metadata{Reference: &summary}, nil
}

func (i *invocation) referencePin(m reference.Manifest) error {
	if i.pin != "" && i.pin != m.Catalog.ContractSHA256 {
		return result.Usage("reference contract differs from --spec-pin; inspect the selected reference")
	}
	return nil
}

func referenceProblem(err error) *result.Problem {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return result.FromError(err)
	}
	return &result.Problem{Kind: "reference", Message: "reference unavailable or invalid; use spec references list or select a complete bundle directory", Code: 2}
}
