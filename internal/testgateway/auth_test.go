package testgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFreshGatewayLoginCompletesBuiltinChallenge(t *testing.T) {
	var step atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/data/app/login":
			http.Redirect(w, r, "/idp/default/authn/login?token=initial&client_id=gateway", 302)
		case "/idp/default/authn/login":
			fmt.Fprint(w, "login")
		case "/idp/default/authn/next-challenge":
			var data map[string]string
			_ = json.NewDecoder(r.Body).Decode(&data)
			switch step.Add(1) {
			case 1:
				if data["token"] != "initial" {
					t.Error("initial query token lost")
				}
				fmt.Fprint(w, `{"token":"challenge"}`)
			case 2:
				if data["token"] != "authenticated" {
					t.Error("challenge token lost")
				}
				fmt.Fprint(w, `{"token":"complete","complete":true}`)
			default:
				t.Error("unexpected challenge retry")
			}
		case "/idp/default/authn/submit-challenge/basic":
			var data struct {
				Token     string
				Challenge map[string]string
			}
			_ = json.NewDecoder(r.Body).Decode(&data)
			if data.Token != "challenge" || data.Challenge["username"] != "admin" || data.Challenge["password"] != "private-password" {
				t.Error("incorrect credentials or challenge")
			}
			fmt.Fprint(w, `{"token":"authenticated","success":true}`)
		case "/idp/default/oidc/auth":
			if r.URL.Query().Get("token") != "complete" || r.URL.Query().Get("client_id") != "gateway" {
				t.Error("callback lost authentication parameters")
			}
			http.SetCookie(w, &http.Cookie{Name: "test-session", Value: "cookie", Path: "/"})
			fmt.Fprint(w, "authenticated")
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	s := &Session{URL: srv.URL, password: "private-password"}
	client := s.HTTPClient()
	if err := s.login(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if step.Load() != 2 || client.Jar == nil {
		t.Fatal("login incomplete")
	}
}

func TestLoginRejectsForeignRedirectWithoutLeakingQueryToken(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { foreignCalls.Add(1) }))
	defer foreign.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/?token=private-token", 302)
	}))
	defer srv.Close()
	s := &Session{URL: srv.URL, password: "private-password"}
	err := s.login(context.Background(), s.HTTPClient())
	if err == nil || strings.Contains(err.Error(), "private-") || foreignCalls.Load() != 0 {
		t.Fatalf("unsafe login redirect: %v", err)
	}
}
