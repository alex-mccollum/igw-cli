package nextcli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/resource"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) resourceCommands() *cobra.Command {
	group := &cobra.Command{Use: "resource", Short: "Inspect and change named configuration resources"}
	types := &cobra.Command{Use: "types", Short: "List resource type IDs from the target catalog", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshot, err := i.snapshot(cmd, false)
			if err != nil {
				return err
			}
			defer snapshot.Close()
			i.withSnapshot(snapshot, resource.Types(snapshot.Catalog))
			return nil
		}}
	describe := &cobra.Command{Use: "describe TYPE", Short: "Read a type's defaults, extension points, and resource counts", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := resource.ValidateType(args[0]); err != nil {
				return err
			}
			return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/resources/type/" + args[0]})
		}}
	var collection string
	get := &cobra.Command{Use: "get TYPE NAME", Short: "Read a resource including its signature and configuration", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := resource.ValidateType(args[0]); err != nil {
				return err
			}
			if collection == "" {
				return result.Usage("collection must be explicit; the default is core")
			}
			return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/resources/find/" + args[0] + "/{name}", PathParams: map[string]string{"name": args[1]}, Query: url.Values{"collection": {collection}}})
		}}
	get.Flags().StringVar(&collection, "collection", "core", "Configuration collection to read")
	var limit, offset int
	var search string
	var filters []string
	list := &cobra.Command{Use: "list TYPE", Short: "Read one page of resources from the active collection", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := resource.ValidateType(args[0]); err != nil {
				return err
			}
			if limit < 1 || limit > 1000 || offset < 0 {
				return result.Usage("limit must be 1..1000 and offset must be nonnegative")
			}
			query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}
			if search != "" {
				query.Set("search", search)
			}
			if err := addFilters(query, filters); err != nil {
				return err
			}
			return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/resources/list/" + args[0], Query: query})
		}}
	list.Flags().IntVar(&limit, "limit", 50, "Maximum items in this page (1..1000)")
	list.Flags().IntVar(&offset, "offset", 0, "Items to skip")
	list.Flags().StringVar(&search, "search", "", "Filter resources by search terms")
	list.Flags().StringArrayVar(&filters, "filter", nil, "Filter field[operator]=value; repeat for different keys")
	group.AddCommand(types, describe, get, list)
	for _, action := range []string{"create", "update", "delete"} {
		group.AddCommand(i.resourceChangeCommand(action))
	}
	return group
}

func (i *invocation) resourceChangeCommand(action string) *cobra.Command {
	change := resource.Change{Action: action}
	var body string
	cmd := &cobra.Command{Use: action + " TYPE NAME", Short: action + " a resource and verify its resulting state", Args: cobra.ExactArgs(2)}
	f := cmd.Flags()
	f.StringVar(&change.Collection, "collection", "core", "Configuration collection to change")
	f.BoolVar(&change.DryRun, "dry-run", false, "Read current state and validate the proposed change without mutating")
	f.BoolVar(&change.Yes, "yes", false, "Confirm this resource change")
	if action != "create" {
		f.StringVar(&change.Signature, "if-signature", "", "Require the signature from the reviewed get or dry-run; required with --yes")
	}
	if action != "delete" {
		f.StringVar(&body, "body", "", "Writable fields as a JSON object, @file, or - for stdin")
		_ = cmd.MarkFlagRequired("body")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		change.Type, change.Name = args[0], args[1]
		if i.offline {
			return result.Usage("resource changes and their previews require current state; use api describe --offline for contract inspection")
		}
		var err error
		change.Body, err = i.readInput(body, 32<<20)
		if err != nil {
			return err
		}
		if _, err := change.Validate(); err != nil {
			return err
		}
		target, token, err := i.runtime()
		if err != nil {
			return err
		}
		svc, err := i.service()
		if err != nil {
			return err
		}
		ctx, cancel, err := execute.Deadline(cmd.Context(), i.timeout)
		if err != nil {
			return err
		}
		defer cancel()
		engine := execute.Engine{Catalog: svc, HTTP: i.app.HTTP}
		scope, err := engine.Open(ctx, target, token, catalog.Policy{ForWrite: !change.DryRun, AllowStale: i.allowStale, Pin: i.pin})
		if err != nil {
			return sourceProblem(err)
		}
		defer scope.Close()
		i.output = resource.Apply(scope, change)
		return nil
	}
	return cmd
}
