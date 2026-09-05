package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/imageref"
)

func runResolve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("igw-capture resolve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var tag, out string
	var timeout time.Duration
	flags.StringVar(&tag, "tag", "8.3", "Official Ignition 8.3 channel or 8.3.<patch> release tag")
	flags.StringVar(&out, "out", "", "New directory for exact registry manifests and resolution receipt")
	flags.DurationVar(&timeout, "timeout", 30*time.Second, "Registry resolution deadline (at most one minute)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || out == "" || timeout <= 0 || timeout > time.Minute {
		fmt.Fprintln(stderr, "required: --out; timeout must be positive and at most one minute")
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	candidate, err := imageref.Resolve(ctx, tag)
	if err == nil {
		err = candidate.Save(ctx, out)
	}
	if err == nil {
		err = json.NewEncoder(stdout).Encode(candidate.Resolution)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
