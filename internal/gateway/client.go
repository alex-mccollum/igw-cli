package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
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
	Stream       io.Writer
	MaxBodyBytes int64
	EnableTiming bool
}

type CallResponse struct {
	Method     string
	URL        string
	StatusCode int
	Headers    http.Header
	Body       []byte
	BodyBytes  int64
	Truncated  bool
	Timing     *CallTiming
}

type CallTiming struct {
	TotalMs            int64 `json:"totalMs"`
	DNSMs              int64 `json:"dnsMs,omitempty"`
	ConnectMs          int64 `json:"connectMs,omitempty"`
	TLSHandshakeMs     int64 `json:"tlsHandshakeMs,omitempty"`
	FirstByteMs        int64 `json:"firstByteMs,omitempty"`
	RequestWriteDoneMs int64 `json:"requestWriteDoneMs,omitempty"`
	BodyReadMs         int64 `json:"bodyReadMs,omitempty"`
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

		startedAt := time.Now()
		timing := &callTimingTrace{}
		if req.EnableTiming {
			httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), timing.httpTrace(startedAt)))
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

		success := resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
		respBody, bodyBytes, truncated, readErr := readResponseBody(resp.Body, req.MaxBodyBytes, req.Stream, success)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, igwerr.NewTransportError(readErr)
		}
		timing.bodyReadDone = time.Now()

		if !success {
			statusErr := &igwerr.StatusError{
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
				Hint:       statusHint(resp.StatusCode),
			}
			lastErr = statusErr
			if attempt < attempts && shouldRetryStatus(resp.StatusCode) {
				retryDelay := retryDelayForResponse(resp.StatusCode, resp.Header, backoff, time.Now())
				if sleepErr := sleepWithContext(ctxReq, retryDelay); sleepErr != nil {
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
			BodyBytes:  bodyBytes,
			Truncated:  truncated,
			Timing:     timing.toEnvelope(startedAt),
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, igwerr.NewTransportError(fmt.Errorf("request failed"))
}

type callTimingTrace struct {
	dnsStart          time.Time
	dnsDone           time.Time
	connectStart      time.Time
	connectDone       time.Time
	tlsStart          time.Time
	tlsDone           time.Time
	gotConn           time.Time
	wroteRequest      time.Time
	firstResponseByte time.Time
	bodyReadDone      time.Time
}

func (t *callTimingTrace) httpTrace(start time.Time) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			t.dnsStart = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.dnsDone = time.Now()
		},
		ConnectStart: func(string, string) {
			t.connectStart = time.Now()
		},
		ConnectDone: func(string, string, error) {
			t.connectDone = time.Now()
		},
		TLSHandshakeStart: func() {
			t.tlsStart = time.Now()
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.tlsDone = time.Now()
		},
		GotConn: func(httptrace.GotConnInfo) {
			t.gotConn = time.Now()
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			t.wroteRequest = time.Now()
		},
		GotFirstResponseByte: func() {
			t.firstResponseByte = time.Now()
		},
	}
}

func (t *callTimingTrace) toEnvelope(start time.Time) *CallTiming {
	if start.IsZero() {
		return nil
	}

	out := &CallTiming{}
	now := time.Now()
	out.TotalMs = now.Sub(start).Milliseconds()

	if !t.dnsStart.IsZero() && !t.dnsDone.IsZero() && t.dnsDone.After(t.dnsStart) {
		out.DNSMs = t.dnsDone.Sub(t.dnsStart).Milliseconds()
	}
	if !t.connectStart.IsZero() && !t.connectDone.IsZero() && t.connectDone.After(t.connectStart) {
		out.ConnectMs = t.connectDone.Sub(t.connectStart).Milliseconds()
	}
	if !t.tlsStart.IsZero() && !t.tlsDone.IsZero() && t.tlsDone.After(t.tlsStart) {
		out.TLSHandshakeMs = t.tlsDone.Sub(t.tlsStart).Milliseconds()
	}
	if !t.firstResponseByte.IsZero() {
		out.FirstByteMs = t.firstResponseByte.Sub(start).Milliseconds()
	}
	if !t.wroteRequest.IsZero() {
		out.RequestWriteDoneMs = t.wroteRequest.Sub(start).Milliseconds()
	}
	if !t.bodyReadDone.IsZero() {
		readStart := t.firstResponseByte
		if readStart.IsZero() {
			readStart = t.gotConn
		}
		if readStart.IsZero() {
			readStart = start
		}
		if t.bodyReadDone.After(readStart) {
			out.BodyReadMs = t.bodyReadDone.Sub(readStart).Milliseconds()
		}
	}

	return out
}

func readResponseBody(body io.Reader, maxBytes int64, stream io.Writer, allowStream bool) ([]byte, int64, bool, error) {
	// Always keep non-2xx payloads in memory so callers can surface useful status errors.
	if stream == nil || !allowStream {
		return readLimited(body, maxBytes)
	}

	reader := body
	limited := reader
	if maxBytes > 0 {
		limited = io.LimitReader(reader, maxBytes)
	}

	n, err := io.Copy(stream, limited)
	if err != nil {
		return nil, 0, false, err
	}

	if maxBytes > 0 {
		var probe [1]byte
		extra, extraErr := body.Read(probe[:])
		if extra > 0 {
			return nil, n, true, nil
		}
		if extraErr != nil && extraErr != io.EOF {
			return nil, n, false, extraErr
		}
	}
	return nil, n, false, nil
}

func readLimited(body io.Reader, maxBytes int64) ([]byte, int64, bool, error) {
	reader := body
	if maxBytes > 0 {
		reader = io.LimitReader(body, maxBytes+1)
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, false, err
	}

	truncated := false
	if maxBytes > 0 && int64(len(respBody)) > maxBytes {
		respBody = respBody[:maxBytes]
		truncated = true
	}
	return respBody, int64(len(respBody)), truncated, nil
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
