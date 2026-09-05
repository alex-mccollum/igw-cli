package testgateway_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type inputCloseObserver struct {
	io.Reader
	closed chan struct{}
	once   sync.Once
}

func (r *inputCloseObserver) Close() error { r.once.Do(func() { close(r.closed) }); return nil }

func TestBodyInputObservation(t *testing.T) {
	data := bytes.Repeat([]byte{0, 255, 1, 128, 10, 13, 44, 61}, 8192)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(got, data) || r.ContentLength != int64(len(data)) || r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Error("body observation changed actual HTTP input")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	client := srv.Client()
	observer := &inputObserver{base: client.Transport}
	client.Transport = observer
	body := &inputCloseObserver{Reader: bytes.NewReader(data), closed: make(chan struct{})}
	req, _ := http.NewRequest("POST", srv.URL+"/input?secret=private-value", body)
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Ignition-API-Token", "private-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("observation did not forward body closure")
	}
	observed := observer.snapshot()
	if len(observed) != 1 || observed[0].BodyBytes != int64(len(data)) || observed[0].BodySHA256 != inputDigest(data) || observed[0].ContentLength != int64(len(data)) || observed[0].Path != "/input" {
		t.Fatal("observation differs from the server's received bytes")
	}
	b, _ := json.Marshal(observed)
	if strings.Contains(string(b), "private-token") || strings.Contains(string(b), "private-value") {
		t.Fatal("observation included credentials or query values")
	}
}

func TestInputJWEEnvelope(t *testing.T) {
	encode := base64.RawURLEncoding.EncodeToString
	fields := map[string]string{"protected": encode([]byte(`{"enc":"A256GCM"}`)), "iv": encode([]byte("iv")), "tag": encode([]byte("tag")), "ciphertext": ""}
	marshal := func() []byte { b, _ := json.Marshal(fields); return b }
	if !inputJWE(marshal(), true) || inputJWE(marshal(), false) {
		t.Fatal("empty ciphertext presence was misinterpreted")
	}
	fields["ciphertext"] = encode([]byte("ciphertext"))
	if !inputJWE(marshal(), false) {
		t.Fatal("valid structural envelope rejected")
	}
	for _, key := range []string{"protected", "iv", "tag", "ciphertext"} {
		original := fields[key]
		delete(fields, key)
		if inputJWE(marshal(), true) {
			t.Fatal("missing envelope member accepted")
		}
		fields[key] = "invalid=="
		if inputJWE(marshal(), false) {
			t.Fatal("invalid base64url accepted")
		}
		fields[key] = original
	}
	for _, invalid := range []string{`{}`, `null`, `[]`, `{"enc":false}`, `{"enc":""}`} {
		fields["protected"] = encode([]byte(invalid))
		if inputJWE(marshal(), false) {
			t.Fatal("invalid protected header accepted")
		}
	}
}

func TestBodyInputEmptyObservation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil || len(data) != 0 || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Content-Type") != "text/plain" {
			t.Error("observation changed known empty-body framing")
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	client := srv.Client()
	observer := &inputObserver{base: client.Transport}
	client.Transport = observer
	req, _ := http.NewRequest("POST", srv.URL+"/input", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	observed := observer.snapshot()
	if len(observed) != 1 || observed[0].BodyBytes != 0 || observed[0].BodySHA256 != inputDigest(nil) || observed[0].ContentLength != 0 || req.Body != http.NoBody {
		t.Fatal("empty-body observation changed presence or identity")
	}
}
