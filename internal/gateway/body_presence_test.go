package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExplicitEmptyBodyRetainsWireContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil || len(raw) != 0 || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Content-Type") != "text/plain" {
			t.Error("empty body lost its media type or zero-length framing")
		}
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	client := Client{BaseURL: srv.URL, Token: "token", HTTP: srv.Client()}
	for _, body := range [][]byte{nil, {}} {
		if _, err := client.Call(context.Background(), CallRequest{Method: "POST", Path: "/body", Body: body, ContentType: "text/plain"}); err != nil {
			t.Fatal(err)
		}
	}
}
