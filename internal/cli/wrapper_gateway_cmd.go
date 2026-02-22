package cli

import (
	"flag"
	"fmt"
	"time"
)

func (c *CLI) runGatewayInfo(args []string) error {
	fs := flag.NewFlagSet("gateway info", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var retry int
	var retryBackoff time.Duration
	var outPath string
	bindWrapperCommon(fs, &common)
	fs.IntVar(&retry, "retry", 0, "Retry attempts for idempotent requests")
	fs.DurationVar(&retryBackoff, "retry-backoff", 250*time.Millisecond, "Retry backoff duration")
	fs.StringVar(&outPath, "out", "", "Write response body to file")

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
		callArgs = append(callArgs, "--out", outPath)
	}

	return c.runCall(callArgs)
}
