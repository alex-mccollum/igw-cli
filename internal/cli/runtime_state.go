package cli

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alex-mccollum/igw-cli/internal/apidocs"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

type runtimeConfigKey struct {
	profile    string
	gatewayURL string
	apiKey     string
	envGateway string
	envToken   string
}

type cachedRuntimeConfig struct {
	effective config.Effective
}

type cachedOpenAPIOperations struct {
	modTimeUnixNano int64
	size            int64
	ops             []apidocs.Operation
}

type runtimeState struct {
	mu sync.RWMutex

	resolvedConfig map[runtimeConfigKey]cachedRuntimeConfig
	openAPIOps     map[string]cachedOpenAPIOperations

	httpClient *http.Client
}

func newRuntimeState() *runtimeState {
	return &runtimeState{
		resolvedConfig: make(map[runtimeConfigKey]cachedRuntimeConfig),
		openAPIOps:     make(map[string]cachedOpenAPIOperations),
	}
}

func (c *CLI) invalidateRuntimeCaches() {
	if c.runtime == nil {
		return
	}

	c.runtime.mu.Lock()
	defer c.runtime.mu.Unlock()

	c.runtime.resolvedConfig = make(map[runtimeConfigKey]cachedRuntimeConfig)
	c.runtime.openAPIOps = make(map[string]cachedOpenAPIOperations)
}

func (c *CLI) runtimeHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	if c.runtime == nil {
		c.runtime = newRuntimeState()
	}

	c.runtime.mu.RLock()
	client := c.runtime.httpClient
	c.runtime.mu.RUnlock()
	if client != nil {
		return client
	}

	c.runtime.mu.Lock()
	defer c.runtime.mu.Unlock()
	if c.runtime.httpClient != nil {
		return c.runtime.httpClient
	}

	c.runtime.httpClient = &http.Client{
		Transport: c.newRuntimeTransport(),
	}
	return c.runtime.httpClient
}

func (c *CLI) newRuntimeTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          envIntWithDefault(c.Getenv, "IGW_MAX_IDLE_CONNS", 64),
		MaxIdleConnsPerHost:   envIntWithDefault(c.Getenv, "IGW_MAX_IDLE_CONNS_PER_HOST", 16),
		MaxConnsPerHost:       envIntWithDefault(c.Getenv, "IGW_MAX_CONNS_PER_HOST", 64),
		IdleConnTimeout:       envDurationWithDefault(c.Getenv, "IGW_IDLE_CONN_TIMEOUT", 90*time.Second),
		TLSHandshakeTimeout:   envDurationWithDefault(c.Getenv, "IGW_TLS_HANDSHAKE_TIMEOUT", 8*time.Second),
		ExpectContinueTimeout: envDurationWithDefault(c.Getenv, "IGW_EXPECT_CONTINUE_TIMEOUT", 1*time.Second),
		ResponseHeaderTimeout: envDurationWithDefault(c.Getenv, "IGW_RESPONSE_HEADER_TIMEOUT", 0),
	}
}

func envIntWithDefault(getenv func(string) string, key string, fallback int) int {
	if getenv == nil {
		return fallback
	}

	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envDurationWithDefault(getenv func(string) string, key string, fallback time.Duration) time.Duration {
	if getenv == nil {
		return fallback
	}

	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func (c *CLI) loadCachedAPIOperations(specFile string) ([]apidocs.Operation, string, []string, error) {
	resolvedSpecFile, candidates := resolveSpecFile(specFile)
	stat, err := os.Stat(resolvedSpecFile)
	if err != nil {
		return nil, resolvedSpecFile, candidates, err
	}

	if c.runtime == nil {
		c.runtime = newRuntimeState()
	}

	c.runtime.mu.RLock()
	cached, ok := c.runtime.openAPIOps[resolvedSpecFile]
	c.runtime.mu.RUnlock()
	if ok && cached.modTimeUnixNano == stat.ModTime().UnixNano() && cached.size == stat.Size() {
		return cached.ops, resolvedSpecFile, candidates, nil
	}

	indexPath := apidocs.IndexPathForSpec(resolvedSpecFile)
	ops, loadErr := apidocs.LoadOperationIndex(indexPath, resolvedSpecFile, stat.Size(), stat.ModTime().UnixNano())
	if loadErr != nil {
		ops, loadErr = apidocs.LoadOperations(resolvedSpecFile)
		if loadErr != nil {
			return nil, resolvedSpecFile, candidates, loadErr
		}
		_ = apidocs.WriteOperationIndex(indexPath, resolvedSpecFile, stat.Size(), stat.ModTime().UnixNano(), ops)
	}

	c.runtime.mu.Lock()
	c.runtime.openAPIOps[resolvedSpecFile] = cachedOpenAPIOperations{
		modTimeUnixNano: stat.ModTime().UnixNano(),
		size:            stat.Size(),
		ops:             ops,
	}
	c.runtime.mu.Unlock()

	return ops, resolvedSpecFile, candidates, nil
}

func (c *CLI) resolveRuntimeConfigCached(profile string, gatewayURL string, apiKey string) (config.Effective, error) {
	if c.runtime == nil {
		c.runtime = newRuntimeState()
	}

	key := runtimeConfigKey{
		profile:    strings.TrimSpace(profile),
		gatewayURL: strings.TrimSpace(gatewayURL),
		apiKey:     strings.TrimSpace(apiKey),
	}
	if c.Getenv != nil {
		key.envGateway = strings.TrimSpace(c.Getenv(config.EnvGatewayURL))
		key.envToken = strings.TrimSpace(c.Getenv(config.EnvToken))
	}

	c.runtime.mu.RLock()
	cached, ok := c.runtime.resolvedConfig[key]
	c.runtime.mu.RUnlock()
	if ok {
		return cached.effective, nil
	}

	cfg, err := c.ReadConfig()
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: fmt.Sprintf("load config: %v", err)}
	}

	resolved, err := config.ResolveWithProfile(cfg, c.Getenv, gatewayURL, apiKey, profile)
	if err != nil {
		return config.Effective{}, &igwerr.UsageError{Msg: err.Error()}
	}

	c.runtime.mu.Lock()
	c.runtime.resolvedConfig[key] = cachedRuntimeConfig{effective: resolved}
	c.runtime.mu.Unlock()

	return resolved, nil
}
