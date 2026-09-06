package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/catalog"
	"github.com/alex-mccollum/igw-cli/internal/config"
	"github.com/alex-mccollum/igw-cli/internal/execute"
	"github.com/alex-mccollum/igw-cli/internal/reference"
)

func BenchmarkCommandSchema(b *testing.B) {
	a := App{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard,
		Getenv: func(string) string { return "" }, ReadConfig: func() (config.File, error) { b.Fatal("schema loaded configuration"); return config.File{}, nil }}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if err := a.Run(context.Background(), []string{"schema", "--json"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTypedRequest(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()
	target, err := catalog.NewTarget("benchmark", srv.URL)
	if err != nil {
		b.Fatal(err)
	}
	engine := execute.Engine{HTTP: srv.Client()}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		prepared, err := engine.Prepare(context.Background(), target, "benchmark-token", execute.Request{Method: "GET", Path: "/status"})
		if err != nil {
			b.Fatal(err)
		}
		if out := engine.Execute(context.Background(), prepared, "benchmark-token"); !out.OK {
			b.Fatal(out.Error)
		}
	}
}

func BenchmarkCapturedCatalog(b *testing.B) {
	bundle := reference.Select("ignition-8.3.9-defaults")
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, c, err := bundle.OpenCatalog(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		if c.OperationCount() < 100 {
			c.Close()
			b.Fatal("benchmark did not load the actual captured catalog")
		}
		c.Close()
	}
}

type zeroStream struct{}

func (zeroStream) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func BenchmarkStreamedArtifact32MiB(b *testing.B) {
	const size = 32 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "33554432")
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.CopyN(w, zeroStream{}, size)
	}))
	defer srv.Close()
	target, err := catalog.NewTarget("benchmark", srv.URL)
	if err != nil {
		b.Fatal(err)
	}
	engine := execute.Engine{HTTP: srv.Client()}
	path := filepath.Join(b.TempDir(), "artifact.bin")
	b.ReportAllocs()
	b.SetBytes(size)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		prepared, err := engine.Prepare(context.Background(), target, "benchmark-token", execute.Request{Method: "GET", Path: "/artifact", Out: path, Overwrite: true, MaxBodyBytes: size})
		if err != nil {
			b.Fatal(err)
		}
		out := engine.Execute(context.Background(), prepared, "benchmark-token")
		if !out.OK || out.Artifact == nil || out.Artifact.Bytes != size {
			b.Fatal("streamed artifact benchmark did not publish the complete payload")
		}
	}
}
