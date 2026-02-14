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
	Method       string
	Path         string
	Query        []string
	Headers      []string
	Body         []byte
	ContentType  string
	Timeout      time.Duration
	Retry        int
	RetryBackoff time.Duration
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

	ctxReq := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		ctxReq, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}

	attempts := req.Retry + 1
	if attempts < 1 {
		attempts = 1
	}
	backoff := req.RetryBackoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}

	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		var bodyReader io.Reader
		if len(req.Body) > 0 {
			bodyReader = bytes.NewReader(req.Body)
		}

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

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = igwerr.NewTransportError(err)
			if attempt < attempts {
				if sleepErr := sleepWithContext(ctxReq, backoff); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			return nil, lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, igwerr.NewTransportError(readErr)
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			statusErr := &igwerr.StatusError{
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
				Hint:       statusHint(resp.StatusCode),
			}
			lastErr = statusErr
			if attempt < attempts && shouldRetryStatus(resp.StatusCode) {
				if sleepErr := sleepWithContext(ctxReq, backoff); sleepErr != nil {
					return nil, sleepErr
				}
				continue
			}
			return nil, statusErr
		}

		return &CallResponse{
			Method:     req.Method,
			URL:        parsedURL.String(),
			StatusCode: resp.StatusCode,
			Headers:    resp.Header.Clone(),
			Body:       respBody,
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, igwerr.NewTransportError(fmt.Errorf("request failed"))
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

func shouldRetryStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return igwerr.NewTransportError(ctx.Err())
	case <-timer.C:
		return nil
	}
}
