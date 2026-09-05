package testgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureRetainsRejectedVendorDocumentAsUnvalidated(t *testing.T) {
	t.Parallel()
	s := &Session{Image: fixtureImage, ImageID: fixtureImageID, Platform: "linux/amd64"}
	dir := t.TempDir()
	raw := []byte(`{"openapi":"3.1.0","paths":{"/health":{}}}`)
	evidence, err := s.Save(dir, raw)
	if err == nil || evidence.Validated || evidence.ValidationError == "" {
		t.Fatalf("invalid spec qualified: %+v %v", evidence, err)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "openapi.json"))
	if err != nil || string(stored) != string(raw) {
		t.Fatalf("vendor evidence lost: %s %v", stored, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "capture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt Evidence
	if err := json.Unmarshal(b, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Validated || receipt.RawSHA256 == "" || receipt.Cleanup {
		t.Fatal("unqualified capture was misrepresented")
	}
	if receipt.Version != 3 || receipt.Platform != s.Platform || receipt.ImageID != s.ImageID {
		t.Fatal("unvalidated capture lost its image provenance")
	}
}

func TestCaptureDeadlineIncludesUnreachableGateway(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	s := &Session{URL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.WaitOpenAPI(ctx); err == nil {
		t.Fatal("capture ignored deadline")
	}
	if time.Since(start) > time.Second {
		t.Fatal("capture exceeded invocation budget")
	}
}

func TestCaptureRejectsUnpinnedOrThirdPartyImagesBeforeDocker(t *testing.T) {
	t.Parallel()
	for _, image := range []string{"inductiveautomation/ignition:latest", "foreign/image@sha256:abc", "inductiveautomation/ignition@sha256:abc"} {
		if _, err := Start(context.Background(), Config{Docker: "must-not-execute", Image: image}); err == nil {
			t.Fatalf("unsafe capture image accepted: %s", image)
		}
	}
}

func TestPasswordOwnerUsesImageUser(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		user     string
		uid, gid int
	}{{"2003:2003", 2003, 2003}, {"2003", 2003, 0}, {"", 0, 0}} {
		uid, gid, err := numericOwner(tc.user)
		if err != nil || uid != tc.uid || gid != tc.gid {
			t.Fatalf("owner %q = %d:%d, %v", tc.user, uid, gid, err)
		}
	}
	for _, user := range []string{"ignition", "2003:ignition", "-1:0", "2003:-1", "2003:2003:0"} {
		if _, _, err := numericOwner(user); err == nil {
			t.Fatalf("unresolved or invalid owner %q accepted", user)
		}
	}
}
