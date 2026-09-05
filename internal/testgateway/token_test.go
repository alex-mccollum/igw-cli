package testgateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvisionTokenUsesCSRFAndStoresOnlyHash(t *testing.T) {
	for _, success := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if (r.Method == "POST" || r.Method == "PUT") && r.Header.Get("X-CSRF-Token") != "private-csrf" {
				t.Error("mutation omitted CSRF protection")
			}
			switch r.URL.Path {
			case "/data/app/session":
				_, _ = io.WriteString(w, `{"csrfToken":"private-csrf"}`)
			case "/data/api/v1/api-token/generate":
				_, _ = io.WriteString(w, `{"key":"private-key","hash":"stored-hash"}`)
			case "/data/api/v1/resources/singleton/ignition/security-levels":
				_, _ = io.WriteString(w, `{"signature":"levels-signature","collection":"core","config":{"securityLevels":[{"name":"Authenticated","children":[{"name":"Roles","children":[{"name":"Administrator","children":[]}]}]}]}}`)
			case "/data/api/v1/resources/singleton/ignition/security-properties":
				_, _ = io.WriteString(w, `{"signature":"properties-signature","collection":"core","config":{"systemAuthProfile":"default","readPermissions":{"type":"AnyOf","securityLevels":[{"name":"Administrator"}]},"writePermissions":{"type":"AnyOf","securityLevels":[{"name":"Administrator"}]}}}`)
			case "/data/api/v1/resources/ignition/security-levels", "/data/api/v1/resources/ignition/security-properties":
				body, _ := io.ReadAll(r.Body)
				if r.Method != "PUT" || !strings.Contains(string(body), `"signature":`) || !strings.Contains(string(body), `"Administrator"`) || !strings.Contains(string(body), qualificationSecurityLevel) || strings.Contains(string(body), `"securityLevels":[]`) {
					t.Error("bootstrap changed existing permissions or omitted preconditions")
				}
				_, _ = io.WriteString(w, `{"success":true}`)
			case "/data/api/v1/resources/ignition/api-token":
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "private-key") || !strings.Contains(string(body), `"tokenHash":"stored-hash"`) {
					t.Error("incorrect token storage")
				}
				if success {
					_, _ = io.WriteString(w, `{"success":true}`)
				} else {
					_, _ = io.WriteString(w, `{"success":false,"problem":{"message":"private-error"}}`)
				}
			default:
				t.Error("unexpected bootstrap request")
				w.WriteHeader(404)
			}
		}))
		s := &Session{URL: srv.URL}
		key, err := s.provisionAPIToken(context.Background(), srv.Client())
		srv.Close()
		if success && (err != nil || key != qualificationTokenName+":private-key") {
			t.Fatal("token bootstrap failed")
		}
		if !success && (err == nil || key != "" || strings.Contains(err.Error(), "private-")) {
			t.Fatal("unconfirmed token or private response escaped")
		}
	}
}

func TestTokenPermissionsRefusePublicOrDifferentPolicies(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"type":"AnyOf","securityLevels":[]}`, `{"type":"AnyOf","securityLevels":[{"name":"Public"}]}`, `{"type":"AllOf","securityLevels":[{"name":"Administrator"}]}`} {
		if _, err := extendTestPermission([]byte(raw)); err == nil {
			t.Fatal("unreviewed permission policy was rewritten")
		}
	}
}

func TestTokenBootstrapRefusesUnownedOrForeignGateway(t *testing.T) {
	for _, address := range []string{"http://gateway.test", "http://127.0.0.1:8088", "http://user@127.0.0.1:8088", "http://127.0.0.1:8088?secret=yes"} {
		s := &Session{URL: address}
		if _, err := s.ProvisionAPIToken(context.Background()); err == nil {
			t.Fatal("token bootstrap accepted an unowned Gateway")
		}
	}
}
