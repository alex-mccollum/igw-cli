package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestQueryInputPreservesNames(t *testing.T) {
	for _, name := range []string{" name ", "\u00a0name\u00a0", " ", "+&%#[]日本"} {
		t.Run(name, func(t *testing.T) {
			want := url.Values{name: {"one=value", " + & % # / = 日本 "}, "present": {""}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !reflect.DeepEqual(r.URL.Query(), want) {
					t.Error("query name, value, or repeated-value order changed")
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
			_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/query?present=", Query: []string{name + "=one=value", name + "= + & % # / = 日本 "}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestQueryInputRedactsMalformedPairs(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
	for _, pair := range []string{"private-query-value", "=private-query-value"} {
		_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/query", Query: []string{pair}})
		if err == nil || igwerr.ExitCode(err) != 2 || strings.Contains(err.Error(), "private-query-value") || calls.Load() != 0 {
			t.Fatal("malformed query reached transport or exposed input")
		}
	}
}

func TestQueryInputRejectsMalformedPathEncoding(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
	for _, path := range []string{"/query?valid=1&value=private-query-value%", "/query?valid=1&value=private-query-value;discarded=1"} {
		_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: path})
		if err == nil || igwerr.ExitCode(err) != 2 || strings.Contains(err.Error(), "private-query-value") || calls.Load() != 0 {
			t.Fatal("malformed query was partly discarded, dispatched, or exposed")
		}
	}
}
