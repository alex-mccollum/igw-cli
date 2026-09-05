package nextcli

import (
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/operations"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

func (i *invocation) backupCommands() *cobra.Command {
	group := &cobra.Command{Use: "backup", Short: "Export a Gateway backup"}
	var peer bool
	export := i.downloadCommand("export", "Download a Gateway backup to an atomic file", "GET /data/api/v1/backup", func() url.Values { return url.Values{"includePeerLocal": {strconv.FormatBool(peer)}} })
	export.Flags().BoolVar(&peer, "include-peer-local", false, "Include local files from a redundant peer")
	group.AddCommand(export)
	return group
}

func (i *invocation) logsCommands() *cobra.Command {
	group := &cobra.Command{Use: "logs", Short: "Read and download Gateway logs"}
	var input operations.LogQuery
	var since, until string
	list := &cobra.Command{Use: "list", Short: "Read one page of logs with optional level, logger, and time filters", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		if i.app.Now != nil {
			now = i.app.Now()
		}
		var err error
		input.Since, err = operations.LogTime(since, now, true)
		if err != nil {
			return err
		}
		input.Until, err = operations.LogTime(until, now, false)
		if err != nil {
			return err
		}
		query, err := input.Values()
		if err != nil {
			return err
		}
		return i.runRequest(cmd, execute.Request{Operation: "GET /data/api/v1/logs", Query: query})
	}}
	f := list.Flags()
	f.IntVar(&input.Limit, "limit", 50, "Maximum events in this page (1..1000)")
	f.IntVar(&input.Offset, "offset", 0, "Events to skip")
	f.StringVar(&input.MinLevel, "min-level", "", "Minimum severity: TRACE, DEBUG, INFO, WARN, ERROR, FATAL, or OFF")
	f.StringVar(&input.Logger, "logger", "", "Filter by logger name")
	f.StringVar(&input.Search, "search", "", "Search terms")
	f.StringVar(&since, "since", "", "RFC3339 start time or a duration ago, such as 1h")
	f.StringVar(&until, "until", "", "RFC3339 end time")
	group.AddCommand(list, i.downloadCommand("download", "Download the complete system log database", "GET /data/api/v1/logs/download", nil))
	return group
}

func (i *invocation) downloadCommand(use, short, operation string, query func() url.Values) *cobra.Command {
	var out string
	var overwrite bool
	var maxBytes int64
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if out == "" || maxBytes <= 0 {
			return result.Usage("download requires --out and a positive --max-bytes")
		}
		request := execute.Request{Operation: operation, Out: out, Overwrite: overwrite, MaxBodyBytes: maxBytes}
		if query != nil {
			request.Query = query()
		}
		return i.runRequest(cmd, request)
	}}
	f := cmd.Flags()
	f.StringVar(&out, "out", "", "Destination file")
	_ = cmd.MarkFlagRequired("out")
	f.BoolVar(&overwrite, "overwrite", false, "Replace an existing file after the complete download")
	f.Int64Var(&maxBytes, "max-bytes", 1<<30, "Maximum download size in bytes (default 1 GiB)")
	return cmd
}
