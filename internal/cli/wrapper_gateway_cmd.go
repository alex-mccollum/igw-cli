package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runGatewayInfo(args []string) error {
	fs := flag.NewFlagSet("gateway info", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var retry int
	var retryBackoff time.Duration
	var outPath string
	var overwrite bool
	bindWrapperCommon(fs, &common)
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")
	fs.BoolVar(&overwrite, "overwrite", false, "Replace an existing output file after a complete download")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	callArgs := []string{
		"--method", "GET",
		"--path", "/data/api/v1/gateway-info",
		"--timeout", common.timeout.String(),
		"--retry", fmt.Sprintf("%d", retry),
		"--retry-backoff", retryBackoff.String(),
	}
	callArgs = append(callArgs, common.callArgsExcludingTimeout()...)
	if outPath != "" {
		if overwrite {
			callArgs = append(callArgs, "--overwrite")
		}
		callArgs = append(callArgs, "--out", outPath)
	}

	if overwrite && strings.TrimSpace(outPath) == "" {
		return &igwerr.UsageError{Msg: "--overwrite requires --out"}
	}
	return c.runCall(callArgs)
}
