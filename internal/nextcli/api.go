package nextcli

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/result"
	"github.com/alex-mccollum/igw-cli/internal/workflow"
)

func (i *invocation) apiCommands() *cobra.Command {
	group := &cobra.Command{Use: "api", Short: "Discover and call the selected Gateway's documented API"}
	var search string
	list := &cobra.Command{Use: "list", Short: "List operation keys and aliases", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, metadata, err := i.discovery(cmd)
			if err != nil {
				return err
			}
			defer c.Close()
			operations := make([]catalog.Operation, 0)
			for _, op := range c.Operations() {
				if strings.Contains(strings.ToLower(op.Key+" "+op.OperationID+" "+op.Summary+" "+strings.Join(op.Tags, " ")), strings.ToLower(search)) {
					op.Definition = nil
					operations = append(operations, op)
				}
			}
			i.output = result.Success(operations)
			i.output.Meta = metadata
			return nil
		}}
	list.Flags().StringVar(&search, "search", "", "Filter operation keys, names, summaries, or tags")
	list.Flags().String("reference", "", "Inspect a bundled name or local bundle directory without a Gateway")
	describe := &cobra.Command{Use: "describe OPERATION", Short: "Inspect input, response, reference, and security contracts", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, metadata, err := i.discovery(cmd)
			if err != nil {
				return err
			}
			defer c.Close()
			description, err := c.Describe(args[0])
			if err != nil {
				return result.Usage(err.Error())
			}
			i.output = result.Success(description)
			i.output.Meta = metadata
			return nil
		}}
	describe.Flags().String("reference", "", "Inspect a bundled name or local bundle directory without a Gateway")
	capabilities := &cobra.Command{Use: "capabilities", Short: "Check catalog prerequisites for tag workflows", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, metadata, err := i.discovery(cmd)
			if err != nil {
				return err
			}
			defer c.Close()
			assessments, err := workflow.AssessTags(c)
			if err != nil {
				return result.Usage(err.Error())
			}
			i.output = result.Success(assessments)
			i.output.Meta = metadata
			return nil
		}}
	capabilities.Flags().String("reference", "", "Inspect a bundled name or local bundle directory without a Gateway")
	group.AddCommand(list, describe, capabilities, i.requestCommand(false), i.requestCommand(true))
	return group
}

func (i *invocation) requestCommand(raw bool) *cobra.Command {
	var request execute.Request
	var body string
	var upload string
	var multipartInput string
	var formFields, formFiles []string
	var uploadLimit int64
	var query, headers, pathParams []string
	cmd := &cobra.Command{Use: "request OPERATION", Short: "Validate, preview, and execute an operation", Args: cobra.ExactArgs(1)}
	if raw {
		cmd.Use, cmd.Short, cmd.Args = "raw", "Send an explicit request without catalog validation", cobra.NoArgs
	}
	f := cmd.Flags()
	f.StringVar(&body, "body", "", "Body text, @file, or - for stdin")
	f.StringVar(&upload, "upload", "", "Stream a private snapshot of a regular file; requires --content-type")
	f.Int64Var(&uploadLimit, "max-upload-bytes", artifact.DefaultUploadLimit, "Maximum streamed upload size including multipart framing (default 1 GiB)")
	cmd.MarkFlagsMutuallyExclusive("body", "upload")
	f.StringVar(&multipartInput, "multipart", "", "Ordered JSON part array, @manifest, or - for stdin; see command docs")
	_ = f.SetAnnotation("multipart", inputSchemaAnnotation, []string{multipartManifestSchema})
	f.StringArrayVar(&formFields, "form-field", nil, "Multipart name=value; literal UTF-8 text, repeat for multiple fields")
	f.StringArrayVar(&formFiles, "form-file", nil, "Multipart name=path; stream a regular file, repeat for multiple files")
	f.StringVar(&request.ContentType, "content-type", "", "Request media type; defaults to application/json for a body")
	for _, name := range []string{"multipart", "form-field", "form-file"} {
		for _, other := range []string{"body", "upload", "content-type"} {
			cmd.MarkFlagsMutuallyExclusive(name, other)
		}
	}
	cmd.MarkFlagsMutuallyExclusive("multipart", "form-field")
	cmd.MarkFlagsMutuallyExclusive("multipart", "form-file")
	f.StringArrayVar(&query, "query", nil, "Query key=value; repeat for multiple values")
	f.StringArrayVar(&headers, "header", nil, "Request header name:value; authentication is managed")
	f.BoolVar(&request.DryRun, "dry-run", false, "Show a preview without sending the proposed request")
	f.BoolVar(&request.Yes, "yes", false, "Confirm a mutating request")
	f.StringVar(&request.Out, "out", "", "Stream a complete response to an atomic artifact")
	f.BoolVar(&request.Overwrite, "overwrite", false, "Replace an existing output artifact after success")
	f.Int64Var(&request.MaxBodyBytes, "max-body-bytes", 0, "Response size limit; default 16 MiB in memory, unlimited for artifacts")
	if raw {
		f.StringVar(&request.Method, "method", "GET", "HTTP method")
		f.StringVar(&request.Path, "path", "", "Path relative to the Gateway's configured base path")
		_ = cmd.MarkFlagRequired("path")
	} else {
		f.StringArrayVar(&pathParams, "path-param", nil, "Path parameter name=value")
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !raw {
			request.Operation = args[0]
		}
		request.Query = make(url.Values)
		request.Headers = make(http.Header)
		request.PathParams = make(map[string]string)
		for _, pair := range query {
			key, value, ok := strings.Cut(pair, "=")
			if !ok || key == "" {
				return result.Usage("query requires key=value")
			}
			request.Query.Add(key, value)
		}
		for _, pair := range headers {
			key, value, ok := strings.Cut(pair, ":")
			if !ok || key == "" {
				return result.Usage("header requires name:value")
			}
			request.Headers.Add(strings.TrimSpace(key), strings.TrimSpace(value))
		}
		for _, pair := range pathParams {
			key, value, ok := strings.Cut(pair, "=")
			if !ok || key == "" {
				return result.Usage("path-param requires name=value")
			}
			if _, exists := request.PathParams[key]; exists {
				return result.Usage("path parameters cannot be repeated")
			}
			request.PathParams[key] = value
		}
		var err error
		request.Body, err = i.readInput(body, 32<<20)
		if err != nil {
			return err
		}
		if f.Changed("body") && request.Body == nil {
			request.Body = []byte{}
		}
		parts, err := i.multipartParts(multipartInput, formFields, formFiles)
		if err != nil {
			return err
		}
		if (f.Changed("multipart") || f.Changed("form-field") || f.Changed("form-file")) && len(parts) == 0 {
			return result.Usage("multipart input requires at least one part")
		}
		return i.runRequestWithBodySource(cmd, request, upload, uploadLimit, parts)
	}
	return cmd
}

func (i *invocation) runRequest(cmd *cobra.Command, request execute.Request) error {
	return i.runRequestWithBodySource(cmd, request, "", 0, nil)
}

func (i *invocation) runRequestWithBodySource(cmd *cobra.Command, request execute.Request, upload string, uploadLimit int64, parts []artifact.MultipartPart) error {
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
	if len(parts) != 0 {
		source, err := artifact.SnapshotMultipart(ctx, parts, uploadLimit)
		if err != nil {
			return inputProblem(err)
		}
		defer source.Close()
		request.Upload = source
	}
	if upload != "" {
		if request.ContentType == "" {
			return result.Usage("--upload requires --content-type")
		}
		source, err := artifact.SnapshotUpload(ctx, upload, uploadLimit)
		if err != nil {
			if errors.Is(err, ctx.Err()) {
				return result.FromError(err)
			}
			return result.Usage(err.Error())
		}
		defer source.Close()
		request.Upload = source
	}
	request.Offline, request.AllowStale, request.Pin = i.offline, i.allowStale, i.pin
	engine := execute.Engine{Catalog: svc, HTTP: i.app.HTTP}
	prepared, err := engine.Prepare(ctx, target, token, request)
	if err != nil {
		return sourceProblem(err)
	}
	i.output = engine.Execute(ctx, prepared, token)
	return nil
}
