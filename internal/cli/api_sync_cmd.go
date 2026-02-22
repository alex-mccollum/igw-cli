package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runAPISync(args []string) error {
	return c.runAPISyncLike(args, "sync")
}

func (c *CLI) runAPIRefresh(args []string) error {
	return c.runAPISyncLike(args, "refresh")
}

func (c *CLI) runAPISyncLike(args []string, mode string) error {
	jsonRequested := argsWantJSON(args)
	fs := flag.NewFlagSet("api "+mode, flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var common wrapperCommon
	var openAPIPath string
	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, false)
	fs.StringVar(&openAPIPath, "openapi-path", "", "Override OpenAPI endpoint path (default: auto-detect)")

	if err := fs.Parse(args); err != nil {
		return c.printAPISyncError(jsonRequested, jsonSelectOptions{}, &igwerr.UsageError{Msg: err.Error()})
	}
	if fs.NArg() > 0 {
		return c.printAPISyncError(common.jsonOutput, jsonSelectOptions{}, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, common.rawOutput, common.selectors)
	if selectErr != nil {
		return c.printAPISyncError(common.jsonOutput, selectionErrorOptions(selectOpts), selectErr)
	}

	if common.apiKeyStdin {
		if common.apiKey != "" {
			return c.printAPISyncError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printAPISyncError(common.jsonOutput, selectOpts, igwerr.NewTransportError(err))
		}
		common.apiKey = strings.TrimSpace(string(tokenBytes))
	}

	resolved, err := c.resolveRuntimeConfig(common.profile, common.gatewayURL, common.apiKey)
	if err != nil {
		return c.printAPISyncError(common.jsonOutput, selectOpts, err)
	}

	result, err := c.syncOpenAPISpec(apiSyncRequest{
		Resolved:    resolved,
		Timeout:     common.timeout,
		OpenAPIPath: strings.TrimSpace(openAPIPath),
	})
	if err != nil {
		return c.printAPISyncError(common.jsonOutput, selectOpts, err)
	}

	if common.jsonOutput {
		payload := map[string]any{
			"ok":             true,
			"specPath":       result.SpecPath,
			"sourceURL":      result.SourceURL,
			"operationCount": result.OperationCount,
			"bytes":          result.Bytes,
			"changed":        result.Changed,
			"attemptedPaths": result.AttemptedPaths,
		}
		if selectWriteErr := printJSONSelection(c.Out, payload, selectOpts); selectWriteErr != nil {
			return c.printAPISyncError(common.jsonOutput, selectionErrorOptions(selectOpts), selectWriteErr)
		}
		return nil
	}

	fmt.Fprintf(c.Out, "spec_path\t%s\n", result.SpecPath)
	fmt.Fprintf(c.Out, "source_url\t%s\n", result.SourceURL)
	fmt.Fprintf(c.Out, "operations\t%d\n", result.OperationCount)
	fmt.Fprintf(c.Out, "bytes\t%d\n", result.Bytes)
	fmt.Fprintf(c.Out, "changed\t%t\n", result.Changed)
	return nil
}

func (c *CLI) printAPISyncError(jsonOutput bool, selectOpts jsonSelectOptions, err error) error {
	if jsonOutput {
		payload := jsonErrorPayload(err)
		if selectErr := printJSONSelection(c.Out, payload, selectOpts); selectErr != nil {
			_ = writeJSONWithOptions(c.Out, jsonErrorPayload(selectErr), selectOpts.compact)
			return selectErr
		}
	} else {
		fmt.Fprintln(c.Err, err.Error())
	}

	return err
}
