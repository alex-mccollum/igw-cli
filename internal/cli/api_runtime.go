package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/gateway"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

var defaultOpenAPIPaths = []string{
	"/openapi",
	"/openapi.json",
	"/data/openapi",
	"/data/openapi.json",
	"/system/openapi",
	"/system/openapi.json",
}

type apiSyncRequest struct {
	Resolved    config.Effective
	Timeout     time.Duration
	OpenAPIPath string
}

type apiSyncResult struct {
	SpecPath       string
	SourceURL      string
	OperationCount int
	Bytes          int
	Changed        bool
	AttemptedPaths []string
}

type apiSyncRuntime struct {
	Profile    string
	GatewayURL string
	APIKey     string
	Timeout    time.Duration
}

func (c *CLI) loadAPIOperations(specFile string, runtime apiSyncRuntime) ([]apidocs.Operation, error) {
	ops, resolvedSpecFile, candidates, err := c.loadCachedAPIOperations(specFile)
	if err == nil {
		return ops, nil
	}

	if !errors.Is(err, os.ErrNotExist) || !defaultSpecRequested(specFile) {
		return nil, openAPILoadError(resolvedSpecFile, candidates, err)
	}

	if runtime.Timeout <= 0 {
		runtime.Timeout = 8 * time.Second
	}

	resolved, resolveErr := c.resolveRuntimeConfig(runtime.Profile, runtime.GatewayURL, runtime.APIKey)
	if resolveErr != nil {
		return nil, &igwerr.UsageError{
			Msg: fmt.Sprintf("OpenAPI spec not found locally and auto-sync failed: %v", resolveErr),
		}
	}

	_, syncErr := c.syncOpenAPISpec(apiSyncRequest{
		Resolved: resolved,
		Timeout:  runtime.Timeout,
	})
	if syncErr != nil {
		return nil, &igwerr.UsageError{
			Msg: fmt.Sprintf("OpenAPI spec not found locally and auto-sync failed: %v", syncErr),
		}
	}

	ops, resolvedSpecFile, candidates, err = c.loadCachedAPIOperations(specFile)
	if err != nil {
		return nil, openAPILoadError(resolvedSpecFile, candidates, err)
	}
	return ops, nil
}

func defaultSpecRequested(specFile string) bool {
	specFile = strings.TrimSpace(specFile)
	return specFile == "" || specFile == apidocs.DefaultSpecFile
}

func (c *CLI) syncOpenAPISpec(req apiSyncRequest) (apiSyncResult, error) {
	if strings.TrimSpace(req.Resolved.GatewayURL) == "" {
		return apiSyncResult{}, &igwerr.UsageError{Msg: "required: --gateway-url (or IGNITION_GATEWAY_URL/config)"}
	}
	if strings.TrimSpace(req.Resolved.Token) == "" {
		return apiSyncResult{}, &igwerr.UsageError{Msg: "required: --api-key (or IGNITION_API_TOKEN/config)"}
	}
	if req.Timeout <= 0 {
		return apiSyncResult{}, &igwerr.UsageError{Msg: "--timeout must be positive"}
	}

	paths := candidateOpenAPIPaths(req.OpenAPIPath)
	client := &gateway.Client{
		BaseURL: req.Resolved.GatewayURL,
		Token:   req.Resolved.Token,
		HTTP:    c.runtimeHTTPClient(),
	}

	specBody, sourcePath, operationCount, attemptedPaths, fetchErr := fetchOpenAPISpec(context.Background(), client, req.Timeout, paths)
	if fetchErr != nil {
		return apiSyncResult{}, fetchErr
	}

	cfgDir, err := config.Dir()
	if err != nil {
		return apiSyncResult{}, &igwerr.UsageError{Msg: err.Error()}
	}
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return apiSyncResult{}, &igwerr.UsageError{Msg: fmt.Sprintf("create config dir: %v", err)}
	}

	specPath := filepath.Join(cfgDir, apidocs.DefaultSpecFile)
	changed := true
	if existing, readErr := os.ReadFile(specPath); readErr == nil && bytes.Equal(existing, specBody) {
		changed = false
	}
	if changed {
		tmpPath := specPath + ".tmp"
		if err := os.WriteFile(tmpPath, specBody, 0o600); err != nil {
			return apiSyncResult{}, &igwerr.UsageError{Msg: fmt.Sprintf("write spec temp: %v", err)}
		}
		if err := os.Rename(tmpPath, specPath); err != nil {
			return apiSyncResult{}, &igwerr.UsageError{Msg: fmt.Sprintf("save spec: %v", err)}
		}
	}
	c.invalidateRuntimeCaches()

	sourceURL, joinErr := gateway.JoinURL(req.Resolved.GatewayURL, sourcePath)
	if joinErr != nil {
		sourceURL = strings.TrimRight(req.Resolved.GatewayURL, "/") + sourcePath
	}

	return apiSyncResult{
		SpecPath:       specPath,
		SourceURL:      sourceURL,
		OperationCount: operationCount,
		Bytes:          len(specBody),
		Changed:        changed,
		AttemptedPaths: attemptedPaths,
	}, nil
}

func candidateOpenAPIPaths(explicit string) []string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if !strings.HasPrefix(explicit, "/") {
			return []string{"/" + explicit}
		}
		return []string{explicit}
	}

	out := make([]string, len(defaultOpenAPIPaths))
	copy(out, defaultOpenAPIPaths)
	return out
}

func fetchOpenAPISpec(ctx context.Context, client *gateway.Client, timeout time.Duration, paths []string) ([]byte, string, int, []string, error) {
	attempted := make([]string, 0, len(paths))
	var firstErr error

	for _, path := range paths {
		candidate := strings.TrimSpace(path)
		if candidate == "" {
			continue
		}
		attempted = append(attempted, candidate)

		resp, err := client.Call(ctx, gateway.CallRequest{
			Method:  "GET",
			Path:    candidate,
			Timeout: timeout,
		})
		if err != nil {
			var statusErr *igwerr.StatusError
			if errors.As(err, &statusErr) {
				if statusErr.AuthFailure() {
					return nil, "", 0, attempted, err
				}
				if statusErr.StatusCode == 404 || statusErr.StatusCode == 405 {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
			}

			var transportErr *igwerr.TransportError
			if errors.As(err, &transportErr) {
				return nil, "", 0, attempted, err
			}

			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		operationCount, validateErr := validateOpenAPISpecBody(resp.Body)
		if validateErr != nil {
			if firstErr == nil {
				firstErr = &igwerr.UsageError{
					Msg: fmt.Sprintf("invalid OpenAPI payload from %q: %v", candidate, validateErr),
				}
			}
			continue
		}

		return resp.Body, candidate, operationCount, attempted, nil
	}

	if firstErr != nil {
		return nil, "", 0, attempted, &igwerr.UsageError{
			Msg: fmt.Sprintf("failed to fetch OpenAPI spec from known endpoints (%s): %v", strings.Join(attempted, ", "), firstErr),
		}
	}

	return nil, "", 0, attempted, &igwerr.UsageError{Msg: "failed to fetch OpenAPI spec: no endpoints attempted"}
}

func validateOpenAPISpecBody(body []byte) (int, error) {
	ops, err := apidocs.LoadOperationsFromJSON(body)
	if err != nil {
		return 0, err
	}
	if len(ops) == 0 {
		return 0, fmt.Errorf("spec has no operations")
	}

	return len(ops), nil
}
