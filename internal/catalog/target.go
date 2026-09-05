package catalog

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/alex-mccollum/igw-cli/internal/gateway"
)

type Target struct {
	Profile string `json:"profile"`
	URL     string `json:"url"`
}

func NewTarget(profile, gatewayURL string) (Target, error) {
	if _, err := gateway.JoinURL(gatewayURL, ""); err != nil {
		return Target{}, err
	}
	u, _ := url.Parse(strings.TrimRight(gatewayURL, "/"))
	if err := checkPath(u.Path); err != nil {
		return Target{}, err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	return Target{Profile: strings.TrimSpace(profile), URL: u.String()}, nil
}

func (t Target) Key() string { b, _ := json.Marshal(t); return digest(b) }

// Endpoint preserves the Gateway's reverse-proxy base path. OpenAPI servers
// never override the explicit target. API paths are relative to this Gateway.
func (t Target) Endpoint(apiPath string) (string, error) {
	u, err := url.Parse(apiPath)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(apiPath, "/") || u.Fragment != "" || u.RawQuery != "" {
		return "", errors.New("API path must start with / and contain no origin, query, or fragment")
	}
	if err := checkPath(u.Path); err != nil {
		return "", err
	}
	return gateway.JoinURL(t.URL, strings.TrimRight(t.URL, "/")+apiPath)
}

func checkPath(path string) error {
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." || strings.Contains(segment, "\\") {
			return errors.New("Gateway paths cannot contain traversal segments or backslashes")
		}
	}
	return nil
}
