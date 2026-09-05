package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func TestHeaderInputPreservesWireValues(t *testing.T) {
	for _, value := range []string{"\u00a0literal\u00a0", "a,b%2Cc", "a\tb", "", " \tvalue\t "} {
		t.Run(value, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Values("X-Test"); len(got) != 2 || got[0] != strings.Trim(value, " \t") || got[1] != "second" {
					t.Error("header value or repeated-field order changed")
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer srv.Close()
			client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
			_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/headers", Headers: []string{"X-Test:" + value, "x-test:second"}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHeaderInputRejectsInvalidAndRedactsErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
	for _, pair := range []string{
		"private-header-value", "X-Test:private-header-value\x00", "X-Test:\vprivate-header-value",
		"X-Test:private-header-value\x7f", "X-\tTest:private-header-value", "X-日本:private-header-value",
		"X-[]:private-header-value", "X-Test:private-header-value\r\n", "\nX-Test:private-header-value",
	} {
		_, err := client.Call(context.Background(), CallRequest{Method: "GET", Path: "/headers", Headers: []string{pair}})
		if err == nil || igwerr.ExitCode(err) != 2 || strings.Contains(err.Error(), "private-header-value") || calls.Load() != 0 {
			t.Errorf("invalid header escaped usage validation or leaked its value: %T, calls=%d", err, calls.Load())
		}
	}
}

func TestHeaderInputManagedFields(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	for _, token := range []bool{false, true} {
		client := Client{BaseURL: srv.URL, Token: "private-token", HTTP: srv.Client()}
		request := CallRequest{Method: "GET", Path: "/headers"}
		if token {
			client.Token = "private-header-value\x00"
		} else {
			request.ContentType = "private-header-value\x00"
		}
		_, err := client.Call(context.Background(), request)
		if err == nil || igwerr.ExitCode(err) != 2 || strings.Contains(err.Error(), "private-header-value") || calls.Load() != 0 {
			t.Error("invalid managed field escaped usage validation or leaked input")
		}
	}
}
