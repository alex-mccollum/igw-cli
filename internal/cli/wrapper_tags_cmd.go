package cli

import (
	"flag"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runTagsExport(args []string) error {
	fs := flag.NewFlagSet("tags export", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var provider string
	var exportType string
	var rootPath string
	var recursive string
	var includeUdts string
	var outPath string
	bindWrapperCommon(fs, &common)
	fs.StringVar(&provider, "provider", "default", "Tag provider name")
	fs.StringVar(&exportType, "type", "json", "Export type: json|xml")
	fs.StringVar(&rootPath, "path", "", "Root tag path")
	fs.StringVar(&recursive, "recursive", "", "Set recursive query to true/false")
	fs.StringVar(&includeUdts, "include-udts", "", "Set includeUdts query to true/false")
	fs.StringVar(&outPath, "out", "", "Write tag export to file")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}

	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "default"
	}
	normalizedType, err := parseRequiredEnumFlag("type", exportType, []string{"json", "xml"})
	if err != nil {
		return err
	}
	normalizedRecursive, err := parseOptionalBoolFlag("recursive", recursive)
	if err != nil {
		return err
	}
	normalizedIncludeUdts, err := parseOptionalBoolFlag("include-udts", includeUdts)
	if err != nil {
		return err
	}

	callArgs := []string{
		"--method", "GET",
		"--path", "/data/api/v1/tags/export",
		"--query", "provider=" + provider,
		"--query", "type=" + normalizedType,
	}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(rootPath) != "" {
		callArgs = append(callArgs, "--query", "path="+strings.TrimSpace(rootPath))
	}
	if normalizedRecursive != "" {
		callArgs = append(callArgs, "--query", "recursive="+normalizedRecursive)
	}
	if normalizedIncludeUdts != "" {
		callArgs = append(callArgs, "--query", "includeUdts="+normalizedIncludeUdts)
	}
	if strings.TrimSpace(outPath) != "" {
		callArgs = append(callArgs, "--out", outPath)
	}
	return c.runCall(callArgs)
}

func (c *CLI) runTagsImport(args []string) error {
	fs := flag.NewFlagSet("tags import", flag.ContinueOnError)
	fs.SetOutput(c.Err)

	var common wrapperCommon
	var provider string
	var importType string
	var collisionPolicy string
	var rootPath string
	var inPath string
	var yes bool
	bindWrapperCommon(fs, &common)
	fs.StringVar(&provider, "provider", "default", "Tag provider name")
	fs.StringVar(&importType, "type", "", "Import type: json|xml|csv (default: infer from --in extension, fallback json)")
	fs.StringVar(&collisionPolicy, "collision-policy", "Abort", "Collision policy: Abort|Overwrite|Rename|Ignore|MergeOverwrite")
	fs.StringVar(&rootPath, "path", "", "Root tag path")
	fs.StringVar(&inPath, "in", "", "Path to tag import file")
	fs.BoolVar(&yes, "yes", false, "Confirm mutating request")

	if err := parseWrapperFlagSet(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(inPath) == "" {
		return &igwerr.UsageError{Msg: "required: --in"}
	}

	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "default"
	}
	resolvedType := strings.TrimSpace(importType)
	if resolvedType == "" {
		resolvedType = inferTagImportType(inPath)
		if resolvedType == "" {
			resolvedType = "json"
		}
	}
	normalizedType, err := parseRequiredEnumFlag("type", resolvedType, []string{"json", "xml", "csv"})
	if err != nil {
		return err
	}
	normalizedCollisionPolicy, err := parseRequiredEnumFlag("collision-policy", collisionPolicy, []string{"Abort", "Overwrite", "Rename", "Ignore", "MergeOverwrite"})
	if err != nil {
		return err
	}
	if !yes {
		return &igwerr.UsageError{Msg: "required: --yes"}
	}

	callArgs := []string{
		"--method", "POST",
		"--path", "/data/api/v1/tags/import",
		"--query", "provider=" + provider,
		"--query", "type=" + normalizedType,
		"--query", "collisionPolicy=" + normalizedCollisionPolicy,
		"--body", "@" + inPath,
		"--content-type", "application/octet-stream",
		"--yes",
	}
	callArgs = append(callArgs, common.callArgs()...)
	if strings.TrimSpace(rootPath) != "" {
		callArgs = append(callArgs, "--query", "path="+strings.TrimSpace(rootPath))
	}
	return c.runCall(callArgs)
}
