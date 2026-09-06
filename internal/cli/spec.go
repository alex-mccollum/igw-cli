package cli

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) specCommands() *cobra.Command {
	group := &cobra.Command{Use: "spec", Short: "Manage complete Gateway OpenAPI snapshots"}
	group.AddCommand(i.referenceCommands())
	group.AddCommand(&cobra.Command{Use: "sync", Short: "Fetch and validate the selected Gateway's current contract", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := i.snapshot(cmd, true)
			if err != nil {
				return err
			}
			defer snapshot.Close()
			i.withSnapshot(snapshot, map[string]any{"operationCount": snapshot.Catalog.OperationCount(), "documentSha256": snapshot.Catalog.DocumentHash(), "contractPolicy": catalog.ContractPolicy, "rawSha256": snapshot.Catalog.RawHash(), "contractSha256": snapshot.Catalog.ContractHash()})
			return nil
		}})
	var summary bool
	inspect := &cobra.Command{Use: "inspect FILE", Short: "Inspect a local OpenAPI file without configuration or network", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := i.readCatalog(args[0])
			if err != nil {
				return err
			}
			defer c.Close()
			data := map[string]any{"operationCount": c.OperationCount(), "documentSha256": c.DocumentHash(), "contractPolicy": catalog.ContractPolicy, "parserVersion": catalog.ParserVersion, "rawSha256": c.RawHash(), "contractSha256": c.ContractHash(), "compatibility": c.Compatibility()}
			if !summary {
				data["operations"], data["adjustments"] = c.Operations(), c.Adjustments()
			}
			i.output = result.Success(data)
			return nil
		}}
	inspect.Flags().BoolVar(&summary, "summary", false, "Show identities, counts, and compatibility totals without operation definitions")
	group.AddCommand(inspect)
	group.AddCommand(&cobra.Command{Use: "import FILE", Short: "Store an explicit local reference for this target's offline inspection", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, _, err := i.runtime()
			if err != nil {
				return err
			}
			c, err := i.readCatalog(args[0])
			if err != nil {
				return err
			}
			defer c.Close()
			svc, err := i.service()
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if i.app.Now != nil {
				now = i.app.Now().UTC()
			}
			snapshot := &catalog.Snapshot{Metadata: catalog.Metadata{Version: catalog.SnapshotVersion, Target: target, Source: args[0], SourceKind: "import", VerifiedAt: now, RawSHA256: c.RawHash(), ContractSHA256: c.ContractHash(), ParserVersion: catalog.ParserVersion}, Catalog: c, Stale: true}
			if err := svc.Store.Save(snapshot); err != nil {
				return sourceProblem(err)
			}
			i.withSnapshot(snapshot, map[string]string{"contractSha256": c.ContractHash()})
			return nil
		}})
	var out string
	var overwrite bool
	export := &cobra.Command{Use: "export", Short: "Export exact vendor bytes from the selected target snapshot", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := i.snapshot(cmd, false)
			if err != nil {
				return err
			}
			defer snapshot.Close()
			writer, err := artifact.New(out, overwrite)
			if err != nil {
				return &result.Problem{Kind: "artifact", Message: "cannot create destination; use --overwrite for an existing file", Code: 2}
			}
			defer writer.Abort()
			if _, err := writer.Write(snapshot.Catalog.Raw()); err != nil {
				return &result.Problem{Kind: "artifact", Message: "could not write catalog artifact", Code: 7}
			}
			info, err := writer.Commit()
			if err != nil {
				return &result.Problem{Kind: "artifact", Message: "could not publish catalog artifact", Code: 7}
			}
			i.withSnapshot(snapshot, nil)
			i.output.Artifact = &info
			return nil
		}}
	export.Flags().StringVar(&out, "out", "", "Destination for the original OpenAPI JSON")
	export.Flags().BoolVar(&overwrite, "overwrite", false, "Replace an existing destination after success")
	_ = export.MarkFlagRequired("out")
	group.AddCommand(export)
	group.AddCommand(&cobra.Command{Use: "diff BEFORE AFTER", Short: "Compare local operation contracts and shared definitions", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			before, err := i.readCatalog(args[0])
			if err != nil {
				return err
			}
			defer before.Close()
			after, err := i.readCatalog(args[1])
			if err != nil {
				return err
			}
			defer after.Close()
			i.output = result.Success(catalog.Compare(before, after))
			return nil
		}})
	return group
}

func (i *invocation) readCatalog(path string) (*catalog.Catalog, error) {
	raw, err := i.readInput("@"+path, catalog.MaxDocumentBytes)
	if err != nil {
		return nil, err
	}
	c, err := catalog.Parse(raw)
	if err != nil {
		return nil, &result.Problem{Kind: "catalog", Message: err.Error(), Code: 2}
	}
	return c, nil
}

func (i *invocation) readInput(input string, limit int64) ([]byte, error) {
	if input == "" {
		return nil, nil
	}
	var reader io.Reader
	if input == "-" {
		reader = i.app.In
	} else if strings.HasPrefix(input, "@") {
		f, err := os.Open(strings.TrimPrefix(input, "@"))
		if err != nil {
			return nil, result.Usage("could not open input file")
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return nil, result.Usage("input file must be a regular file")
		}
		reader = f
	} else {
		reader = strings.NewReader(input)
	}
	b, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, result.Usage("could not read input")
	}
	if int64(len(b)) > limit {
		return nil, result.Usage("input exceeds size limit")
	}
	return b, nil
}
