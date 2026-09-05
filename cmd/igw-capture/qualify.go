package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/referencebuild"
)

func runQualify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("igw-capture qualify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var in referencebuild.Inputs
	var timeout time.Duration
	flags.StringVar(&in.ResolutionDir, "resolution", "", "Directory containing verified registry resolution evidence")
	flags.StringVar(&in.CaptureDir, "capture", "", "Directory containing exact OpenAPI JSON and capture receipt")
	flags.StringVar(&in.Lifecycle, "lifecycle", "", "Passing lifecycle receipt for this image and binary")
	flags.StringVar(&in.Resources, "resources", "", "Passing resource workflow receipt")
	flags.StringVar(&in.Transfers, "transfers", "", "Passing project/tag workflow receipt")
	flags.StringVar(&in.Operations, "operations", "", "Passing operational workflow receipt")
	flags.StringVar(&in.TestBinary, "test-binary", "", "Exact executable that produced all workflow/lifecycle receipts")
	flags.StringVar(&in.Baseline, "baseline", "", "Previous qualified OpenAPI JSON or JSON.gz for comparison")
	flags.StringVar(&in.Out, "out", "", "New directory for the independently distributable reference")
	flags.DurationVar(&timeout, "timeout", time.Minute, "Qualification deadline, at most eight minutes")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || in.ResolutionDir == "" || in.CaptureDir == "" || in.Lifecycle == "" || in.Resources == "" || in.Transfers == "" || in.Operations == "" || in.TestBinary == "" || in.Baseline == "" || in.Out == "" || timeout <= 0 || timeout > 8*time.Minute {
		fmt.Fprintln(stderr, "required: --resolution --capture --lifecycle --resources --transfers --operations --test-binary --baseline --out; timeout must be positive and at most eight minutes")
		return 2
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	m, err := referencebuild.Build(ctx, in)
	if err == nil {
		err = json.NewEncoder(stdout).Encode(m)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
