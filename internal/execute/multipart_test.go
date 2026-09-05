package execute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/artifact"
	"github.com/alex-mccollum/igw-cli/internal/catalog"
)

func TestMultipartPreparedFramingAndIdentityAreBound(t *testing.T) {
	var observed atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := sha256.New()
		n, err := io.Copy(hash, r.Body)
		if err != nil || n != r.ContentLength {
			t.Error("multipart body framing was incomplete")
		}
		observed.Store([2]string{r.Header.Get("Content-Type"), hex.EncodeToString(hash.Sum(nil))})
		_, _ = io.WriteString(w, `{"accepted":true}`)
	}))
	defer srv.Close()
	text := "exact input"
	u, err := artifact.SnapshotMultipart(context.Background(), []artifact.MultipartPart{{Name: "field", Text: &text}}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	target, _ := catalog.NewTarget("test", srv.URL)
	engine := Engine{HTTP: srv.Client()}
	input := Request{Method: "PUT", Path: "/upload", Upload: u, Yes: true}
	prepared, err := engine.Prepare(context.Background(), target, "token", input)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.preview.BodySHA256 != u.SHA256() || prepared.preview.BodyBytes != u.Bytes() || prepared.preview.ContentType != u.ContentType() || len(prepared.preview.Parts) != 1 || observed.Load() != nil {
		t.Fatal("preparation changed the snapshot or dispatched")
	}
	input.ContentType = strings.Replace(u.ContentType(), "boundary=", "boundary=changed", 1)
	if _, err := engine.Prepare(context.Background(), target, "token", input); err == nil || observed.Load() != nil {
		t.Fatal("prepared multipart framing could be replaced")
	}
	if got := engine.Execute(context.Background(), prepared, "token"); !got.OK || observed.Load() != [2]string{prepared.preview.ContentType, prepared.preview.BodySHA256} {
		t.Fatal("transmission differed from prepared encoded identity")
	}
}
