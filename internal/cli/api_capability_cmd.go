package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runAPICapability(args []string) error {
	jsonRequested := argsWantJSON(args)
	capability := ""
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		capability = strings.TrimSpace(args[0])
		args = args[1:]
	}
	fs := flag.NewFlagSet("api capability", flag.ContinueOnError)
	fs.SetOutput(c.Err)
	if jsonRequested {
		fs.SetOutput(io.Discard)
	}

	var common wrapperCommon
	var specFile string
	bindWrapperCommonWithDefaults(fs, &common, 8*time.Second, false)
	fs.StringVar(&specFile, "spec-file", apidocs.DefaultSpecFile, "Path to OpenAPI JSON file")

	if err := fs.Parse(args); err != nil {
		return c.printAPICapabilityError(jsonRequested, jsonSelectOptions{}, &igwerr.UsageError{Msg: err.Error()})
	}

	selectOpts, selectErr := newJSONSelectOptions(common.jsonOutput, common.compactJSON, common.rawOutput, common.selectors)
	if selectErr != nil {
		return c.printAPICapabilityError(common.jsonOutput, selectionErrorOptions(selectOpts), selectErr)
	}

	if fs.NArg() == 1 {
		if capability != "" {
			return c.printAPICapabilityError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "unexpected positional arguments"})
		}
		capability = strings.TrimSpace(fs.Arg(0))
	} else if fs.NArg() > 1 {
		return c.printAPICapabilityError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "unexpected positional arguments"})
	}
	if capability == "" {
		return c.printAPICapabilityError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "required capability (supported: file-write)"})
	}
	if !strings.EqualFold(capability, "file-write") {
		return c.printAPICapabilityError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: fmt.Sprintf("unsupported capability %q", capability)})
	}

	if common.apiKeyStdin {
		if common.apiKey != "" {
			return c.printAPICapabilityError(common.jsonOutput, selectOpts, &igwerr.UsageError{Msg: "use only one of --api-key or --api-key-stdin"})
		}
		tokenBytes, err := io.ReadAll(c.In)
		if err != nil {
			return c.printAPICapabilityError(common.jsonOutput, selectOpts, igwerr.NewTransportError(err))
		}
		common.apiKey = strings.TrimSpace(string(tokenBytes))
	}

	start := time.Now()
	ops, err := c.loadAPIOperations(specFile, apiSyncRuntime{
		Profile:    common.profile,
		GatewayURL: common.gatewayURL,
		APIKey:     common.apiKey,
		Timeout:    common.timeout,
	})
	if err != nil {
		return c.printAPICapabilityError(common.jsonOutput, selectOpts, err)
	}

	result := buildFileWriteCapability(ops)
	elapsedMs := time.Since(start).Milliseconds()
	if common.jsonOutput {
		payload := map[string]any{
			"ok":                  true,
			"capability":          "file-write",
			"classification":      result.Classification,
			"operationCount":      result.OperationCount,
			"writeOperationCount": result.WriteOperationCount,
			"resourceWriteCount":  result.ResourceWriteCount,
			"scanApplyCount":      result.ScanApplyCount,
		}
		if common.timing || common.jsonStats {
			payload["stats"] = map[string]any{"elapsedMs": elapsedMs}
		}
		if selectWriteErr := printJSONSelection(c.Out, payload, selectOpts); selectWriteErr != nil {
			return c.printAPICapabilityError(common.jsonOutput, selectionErrorOptions(selectOpts), selectWriteErr)
		}
		return nil
	}

	fmt.Fprintf(c.Out, "capability\tfile-write\n")
	fmt.Fprintf(c.Out, "classification\t%s\n", result.Classification)
	fmt.Fprintf(c.Out, "operations\t%d\n", result.OperationCount)
	fmt.Fprintf(c.Out, "write_operations\t%d\n", result.WriteOperationCount)
	fmt.Fprintf(c.Out, "resource_write_operations\t%d\n", result.ResourceWriteCount)
	fmt.Fprintf(c.Out, "scan_apply_operations\t%d\n", result.ScanApplyCount)
	if common.timing {
		fmt.Fprintf(c.Err, "timing\telapsedMs=%d\n", elapsedMs)
	}
	return nil
}

type fileWriteCapabilityResult struct {
	Classification      string
	OperationCount      int
	WriteOperationCount int
	ResourceWriteCount  int
	ScanApplyCount      int
}

func buildFileWriteCapability(ops []apidocs.Operation) fileWriteCapabilityResult {
	writeMethods := map[string]struct{}{
		"POST":   {},
		"PUT":    {},
		"PATCH":  {},
		"DELETE": {},
	}
	resourceKeywords := []string{"project", "resource", "config", "scan", "apply"}

	result := fileWriteCapabilityResult{
		Classification: "api_unsupported",
		OperationCount: len(ops),
	}

	for _, op := range ops {
		pathLower := strings.ToLower(strings.TrimSpace(op.Path))
		_, isWrite := writeMethods[op.Method]
		if isWrite {
			result.WriteOperationCount++
			for _, keyword := range resourceKeywords {
				if strings.Contains(pathLower, keyword) {
					result.ResourceWriteCount++
					break
				}
			}
		}
		if strings.Contains(pathLower, "/scan/") || strings.HasSuffix(pathLower, "/scan") || strings.Contains(pathLower, "/apply") {
			result.ScanApplyCount++
		}
	}

	switch {
	case result.ResourceWriteCount > 0 && result.ScanApplyCount > 0:
		result.Classification = "api_supported"
	case result.WriteOperationCount > 0 || result.ScanApplyCount > 0:
		result.Classification = "api_partial"
	default:
		result.Classification = "api_unsupported"
	}

	return result
}

func (c *CLI) printAPICapabilityError(jsonOutput bool, selectOpts jsonSelectOptions, err error) error {
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
