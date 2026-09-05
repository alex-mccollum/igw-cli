// Package execute prepares and executes typed requests without entering a CLI
// parser. Workflow services and command handlers share this boundary.
package execute

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
	"github.com/alex-mccollum/igw-cli/internal/result"
)

type Request struct {
	Operation    string
	Method       string
	Path         string
	PathParams   map[string]string
	Query        url.Values
	Headers      http.Header
	Body         []byte
	Upload       *artifact.Upload
	ContentType  string
	DryRun       bool
	Yes          bool
	Offline      bool
	AllowStale   bool
	Pin          string
	Out          string
	Overwrite    bool
	MaxBodyBytes int64
}

type Preview struct {
	Method      string                       `json:"method"`
	Path        string                       `json:"path"`
	Mutating    bool                         `json:"mutating"`
	QueryKeys   []string                     `json:"queryKeys,omitempty"`
	HeaderKeys  []string                     `json:"headerKeys,omitempty"`
	ContentType string                       `json:"contentType,omitempty"`
	BodyBytes   int64                        `json:"bodyBytes"`
	BodySHA256  string                       `json:"bodySha256,omitempty"`
	Validation  string                       `json:"validation"`
	Parts       []artifact.MultipartPartInfo `json:"parts,omitempty"`
}

type Prepared struct {
	preview Preview
	meta    result.Metadata
	target  catalog.Target
	url     string
	input   Request
}

type Engine struct {
	Catalog catalog.Service
	HTTP    *http.Client
}

func mutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (e Engine) Prepare(ctx context.Context, target catalog.Target, token string, input Request) (*Prepared, error) {
	return e.prepare(ctx, target, token, input, nil)
}

// A supplied snapshot belongs to a workflow scope. The ordinary one-request
// entrypoint still acquires and closes its own catalog.
func (e Engine) prepare(ctx context.Context, target catalog.Target, token string, input Request, snapshot *catalog.Snapshot) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.MaxBodyBytes < 0 {
		return nil, result.Usage("--max-body-bytes must be nonnegative")
	}
	if input.Upload != nil && input.Upload.ContentType() != "" {
		if input.ContentType != "" && input.ContentType != input.Upload.ContentType() {
			return nil, result.Usage("multipart content type and boundary belong to the prepared upload")
		}
		input.ContentType = input.Upload.ContentType()
	}
	if input.Upload != nil && (len(input.Body) != 0 || input.ContentType == "") {
		return nil, result.Usage("upload requires an explicit content type and cannot be combined with an inline body")
	}
	if input.Overwrite && input.Out == "" {
		return nil, result.Usage("--overwrite requires --out")
	}
	if input.DryRun && input.Out != "" {
		return nil, result.Usage("--out downloads a response; omit it when previewing")
	}
	if !input.DryRun && input.Offline {
		return nil, result.Usage("--offline permits discovery and previews; executing a request requires connectivity")
	}
	method, path := strings.ToUpper(input.Method), input.Path
	meta := result.Metadata{Target: &target}
	validation := "not_requested"
	if input.Operation != "" {
		if method != "" || path != "" {
			return nil, result.Usage("select an operation or a raw method/path, not both")
		}
		owned := snapshot == nil
		policy := catalog.Policy{Offline: input.Offline, AllowStale: input.AllowStale, Pin: input.Pin}
		if owned {
			var err error
			snapshot, err = e.Catalog.Acquire(ctx, target, token, policy)
			if err != nil {
				return nil, catalogProblem(err)
			}
			defer func() { snapshot.Close() }()
		}
		op, err := snapshot.Catalog.Resolve(input.Operation)
		if err != nil {
			return nil, result.Usage(err.Error())
		}
		if owned && mutating(op.Method) && !input.DryRun {
			// Re-resolve by immutable method/path identity after fresh verification;
			// an alias changing meaning cannot silently select a different route.
			snapshot.Close()
			policy.ForWrite = true
			snapshot, err = e.Catalog.Acquire(ctx, target, token, policy)
			if err != nil {
				return nil, catalogProblem(err)
			}
			op, err = snapshot.Catalog.Resolve(op.Key)
			if err != nil {
				return nil, result.Usage("selected operation disappeared during contract refresh")
			}
		}
		method = op.Method
		path, err = fillPath(op.Path, input.PathParams)
		if err != nil {
			return nil, err
		}
		if input.ContentType == "" && len(input.Body) > 0 {
			input.ContentType = "application/json"
		}
		validationBody := input.Body
		if input.Upload != nil {
			if !snapshot.Catalog.OpaqueUpload(op.Key, input.ContentType) {
				return nil, result.Usage("streamed uploads require a declared opaque or unconstrained binary body; inspect api describe or use a bounded --body or explicit api raw")
			}
			// Opaque content has no value assertions. Represent only presence to
			// validate the remaining contract without reading a large upload.
			if input.Upload.Bytes() > 0 {
				validationBody = []byte{0}
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, "http://igw.invalid"+path, bytes.NewReader(validationBody))
		if err != nil {
			return nil, result.Usage("invalid encoded request")
		}
		request.URL.RawQuery = input.Query.Encode()
		request.Header = input.Headers.Clone()
		if request.Header == nil {
			request.Header = make(http.Header)
		}
		request.Header.Set("Content-Type", input.ContentType)
		// Some specs model the managed header as a required parameter. Its
		// presence is represented without giving credentials to the validator.
		if token != "" {
			request.Header.Set("X-Ignition-API-Token", "managed")
		}
		checked, err := snapshot.Catalog.ValidateRequest(op.Key, request)
		if err != nil {
			if errors.Is(err, catalog.ErrUnsupportedBodyEncoding) {
				return nil, &result.Problem{Kind: "unsupported_input", Message: "the CLI cannot validate this body encoding; inspect api describe or use api raw explicitly", Code: 2}
			}
			if errors.Is(err, catalog.ErrSchemaCompilation) || errors.Is(err, catalog.ErrIncompleteContract) {
				return nil, &result.Problem{Kind: "catalog_schema", Message: "the Gateway's schema cannot validate this operation; inspect api describe or use api raw explicitly", Code: 2}
			}
			return nil, result.Usage("could not validate request against the selected operation")
		}
		if len(checked.Issues) > 0 {
			return nil, &result.Problem{Kind: "validation", Message: "request does not satisfy the operation contract; inspect api describe", Code: 2, Details: checked.Issues}
		}
		description, err := snapshot.Catalog.Describe(op.Key)
		if err != nil {
			return nil, result.Usage(err.Error())
		}
		meta.Catalog = &snapshot.Metadata
		meta.Stale = snapshot.Stale
		meta.Warnings = append(append([]string(nil), snapshot.Warnings...), description.Gaps...)
		validation = checked.Coverage
	} else {
		if len(input.PathParams) != 0 {
			return nil, result.Usage("path parameters require a catalog operation")
		}
		if input.Pin != "" || input.AllowStale {
			return nil, result.Usage("spec policies require a catalog operation")
		}
		if method == "" {
			method = http.MethodGet
		}
	}
	if !knownMethod(method) {
		return nil, result.Usage("unsupported HTTP method")
	}
	if mutating(method) && !input.Yes && !input.DryRun {
		return nil, result.Usage("mutating requests require --yes; inspect --dry-run first")
	}
	endpoint, err := target.Endpoint(path)
	if err != nil {
		return nil, result.Usage(err.Error())
	}
	if len(input.Body) > 0 && input.ContentType == "" {
		input.ContentType = "application/json"
	}
	if strings.ContainsAny(input.ContentType, "\r\n") {
		return nil, result.Usage("invalid content type")
	}
	for key, values := range input.Headers {
		if strings.EqualFold(key, "X-Ignition-API-Token") || strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Cookie") || strings.EqualFold(key, "Host") {
			return nil, result.Usage("authentication and routing headers are managed by the CLI")
		}
		if strings.EqualFold(key, "Content-Type") || strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") {
			return nil, result.Usage("content type and body framing are managed by the CLI; use --content-type")
		}
		if strings.ContainsAny(key, ":\r\n ") || key == "" {
			return nil, result.Usage("invalid header name")
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				return nil, result.Usage("invalid header value")
			}
		}
	}
	meta.Validation = validation
	preview := Preview{Method: method, Path: path, Mutating: mutating(method), ContentType: input.ContentType, BodyBytes: int64(len(input.Body)), Validation: validation}
	if input.Upload != nil {
		preview.BodyBytes, preview.BodySHA256 = input.Upload.Bytes(), input.Upload.SHA256()
		preview.Parts = input.Upload.Parts()
	}
	if len(input.Body) > 0 {
		sum := sha256.Sum256(input.Body)
		preview.BodySHA256 = hex.EncodeToString(sum[:])
	}
	for key := range input.Query {
		preview.QueryKeys = append(preview.QueryKeys, key)
	}
	for key := range input.Headers {
		preview.HeaderKeys = append(preview.HeaderKeys, key)
	}
	sort.Strings(preview.QueryKeys)
	sort.Strings(preview.HeaderKeys)
	input.Body = bytes.Clone(input.Body)
	input.Headers = input.Headers.Clone()
	query := make(url.Values, len(input.Query))
	for key, values := range input.Query {
		query[key] = append([]string(nil), values...)
	}
	input.Query = query
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Prepared{preview: preview, meta: meta, target: target, url: endpoint, input: input}, nil
}

func catalogProblem(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var coded interface{ ExitCode() int }
	var usage *igwerr.UsageError
	var status *igwerr.StatusError
	var transport *igwerr.TransportError
	if errors.As(err, &coded) || errors.As(err, &usage) || errors.As(err, &status) || errors.As(err, &transport) {
		return err
	}
	return &result.Problem{Kind: "catalog", Message: "Gateway catalog unavailable or invalid; inspect spec sync and the selected target", Code: 7}
}

func (e Engine) Execute(ctx context.Context, prepared *Prepared, token string) result.Result {
	if prepared == nil {
		return result.Failure(result.Usage("prepared request is required"))
	}
	if err := ctx.Err(); err != nil {
		return result.Failure(err)
	}
	p := prepared
	if p.input.DryRun {
		out := result.Success(p.preview)
		out.Outcome, out.Meta = "preview", p.meta
		return out
	}
	if token == "" {
		return result.Failure(result.Usage("configure IGNITION_API_TOKEN or a profile token"))
	}
	var download *artifact.Writer
	if p.input.Out != "" {
		var err error
		download, err = artifact.New(p.input.Out, p.input.Overwrite)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return result.Failure(result.Usage("destination exists; use --overwrite"))
			}
			return result.Failure(&result.Problem{Kind: "artifact", Message: "could not create output artifact", Code: 7})
		}
		defer download.Abort()
	}
	request := gateway.CallRequest{Method: p.preview.Method, Path: p.url, Body: p.input.Body, ContentType: p.input.ContentType, MaxBodyBytes: p.input.MaxBodyBytes}
	if p.input.Upload != nil {
		reader, err := p.input.Upload.Open(ctx)
		if err != nil {
			return result.Failure(&result.Problem{Kind: "upload", Message: "upload snapshot is unavailable", Code: 2})
		}
		defer reader.Close()
		request.BodyReader, request.BodySize = reader, p.input.Upload.Bytes()
	}
	if request.MaxBodyBytes == 0 && download == nil {
		request.MaxBodyBytes = 16 << 20
	}
	if download != nil {
		request.Stream = download
	}
	for key, values := range p.input.Query {
		for _, value := range values {
			request.Query = append(request.Query, key+"="+value)
		}
	}
	for key, values := range p.input.Headers {
		for _, value := range values {
			request.Headers = append(request.Headers, key+":"+value)
		}
	}
	httpClient := e.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if p.preview.Mutating {
		clone := *httpClient
		clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		httpClient = &clone
	}
	client := &gateway.Client{BaseURL: p.target.URL, Token: token, HTTP: httpClient}
	resp, err := client.Call(ctx, request)
	if err != nil {
		out := result.Failure(err)
		out.Meta = p.meta
		var transport *igwerr.TransportError
		if p.preview.Mutating && errors.As(err, &transport) {
			out.Outcome = "uncertain"
			out.Meta.Warnings = append(out.Meta.Warnings, "The mutation may have reached the Gateway. Read current state before deciding whether to retry.")
		}
		return out
	}
	out := result.Success(nil)
	out.Meta = p.meta
	out.Meta.HTTPStatus = resp.StatusCode
	if p.preview.Mutating || resp.StatusCode == http.StatusAccepted {
		out.Outcome = "accepted"
		out.Meta.Verification = "not_performed"
	}
	if download != nil {
		info, err := download.Commit()
		if err != nil {
			failed := result.Failure(&result.Problem{Kind: "artifact", Message: "Gateway responded but output artifact publication failed", Code: 7})
			failed.Meta = out.Meta
			return failed
		}
		out.Artifact = &info
	} else if len(resp.Body) > 0 {
		if json.Valid(resp.Body) {
			out.Data = json.RawMessage(resp.Body)
		} else if !utf8.Valid(resp.Body) {
			out.Data = map[string]string{"encoding": "base64", "value": base64.StdEncoding.EncodeToString(resp.Body)}
		} else {
			out.Data = string(resp.Body)
		}
	}
	return out
}

func fillPath(template string, params map[string]string) (string, error) {
	var out strings.Builder
	used := make(map[string]bool)
	for {
		start := strings.IndexByte(template, '{')
		if start < 0 {
			out.WriteString(template)
			break
		}
		out.WriteString(template[:start])
		template = template[start+1:]
		end := strings.IndexByte(template, '}')
		if end < 0 {
			return "", result.Usage("invalid path template in catalog")
		}
		name := template[:end]
		value, ok := params[name]
		if !ok {
			return "", result.Usage("missing path parameter: " + name)
		}
		used[name] = true
		out.WriteString(url.PathEscape(value))
		template = template[end+1:]
	}
	if len(used) != len(params) {
		return "", result.Usage("unknown path parameter for selected operation")
	}
	return out.String(), nil
}

func knownMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// Deadline wraps discovery, validation, execution, and future workflow polling
// in one invocation budget. Callers should pass the returned context everywhere.
func Deadline(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if timeout <= 0 {
		return nil, nil, result.Usage("timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, nil
}
