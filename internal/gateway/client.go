package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

const tokenHeader = "X-Ignition-API-Token"

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type CallRequest struct {
	Method      string
	Path        string
	Query       []string
	Headers     []string
	Body        []byte
	ContentType string
	Timeout     time.Duration
}

type CallResponse struct {
	Method     string
	URL        string
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func JoinURL(baseURL string, apiPath string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}

	path, err := url.Parse(apiPath)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}

	return base.ResolveReference(path).String(), nil
}

func (c *Client) Call(ctx context.Context, req CallRequest) (*CallResponse, error) {
	fullURL, err := JoinURL(c.BaseURL, req.Path)
	if err != nil {
		return nil, &igwerr.UsageError{Msg: err.Error()}
	}

	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, &igwerr.UsageError{Msg: fmt.Sprintf("parse request url: %v", err)}
	}

	values := parsedURL.Query()
	if err := addQuery(values, req.Query); err != nil {
		return nil, err
	}
	parsedURL.RawQuery = values.Encode()

	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	ctxReq := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		ctxReq, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctxReq, req.Method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, &igwerr.UsageError{Msg: fmt.Sprintf("build request: %v", err)}
	}
	httpReq.Header.Set(tokenHeader, c.Token)

	if len(req.Body) > 0 && req.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.ContentType)
	}

	if err := addHeaders(httpReq.Header, req.Headers); err != nil {
		return nil, err
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, igwerr.NewTransportError(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, igwerr.NewTransportError(err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &igwerr.StatusError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Hint:       statusHint(resp.StatusCode),
		}
	}

	return &CallResponse{
		Method:     req.Method,
		URL:        parsedURL.String(),
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

func addQuery(values url.Values, pairs []string) error {
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return &igwerr.UsageError{Msg: fmt.Sprintf("invalid --query value %q (expected key=value)", pair)}
		}

		values.Add(strings.TrimSpace(key), value)
	}

	return nil
}

func addHeaders(headers http.Header, pairs []string) error {
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return &igwerr.UsageError{Msg: fmt.Sprintf("invalid --header value %q (expected key:value)", pair)}
		}

		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		if strings.EqualFold(key, tokenHeader) {
			return &igwerr.UsageError{Msg: fmt.Sprintf("header %q is managed by the CLI and cannot be overridden", tokenHeader)}
		}
		headers.Add(key, strings.TrimSpace(value))
	}

	return nil
}

func statusHint(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "token missing or invalid"
	case http.StatusForbidden:
		return "token authenticated but lacks permissions or requires secure connections"
	default:
		return ""
	}
}
