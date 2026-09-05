package gateway

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func validHTTPURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") &&
		u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && originPort(a) == originPort(b)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func clientForOrigin(original *http.Client, origin *url.URL) *http.Client {
	client := *original
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !validHTTPURL(req.URL) || !sameOrigin(origin, req.URL) {
			return http.ErrUseLastResponse
		}
		if original.CheckRedirect != nil {
			if err := original.CheckRedirect(req, via); err != nil {
				return err
			}
			// A caller-supplied hook can rewrite the request URL.
			if !validHTTPURL(req.URL) || !sameOrigin(origin, req.URL) {
				return http.ErrUseLastResponse
			}
		} else if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}
