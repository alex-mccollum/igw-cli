package imageref

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fixture(t *testing.T, indexType, manifestType string) ([]byte, []byte) {
	t.Helper()
	digest := "sha256:" + strings.Repeat("a", 64)
	body := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"digest":%q,"size":100},"layers":[{"digest":%q,"size":1024}]}`, manifestType, digest, digest))
	index := []byte(fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[{"mediaType":%q,"digest":%q,"size":%d,"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"unknown","architecture":"unknown"}}]}`, indexType, manifestType, hash(body), len(body)))
	return index, body
}

func response(body []byte, mediaType string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{mediaType}, "Docker-Content-Digest": []string{hash(body)}}, Body: io.NopCloser(bytes.NewReader(body))}
}

func registry(t *testing.T, index, body []byte, alter func(*http.Request, *http.Response) (*http.Response, error)) *http.Client {
	t.Helper()
	return &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		var resp *http.Response
		if req.URL.String() == authURL {
			if req.Header.Get("Authorization") != "" {
				t.Fatal("credential sent to authentication endpoint")
			}
			resp = response([]byte(`{"token":"private-pull-token"}`), "application/json")
		} else {
			if req.Header.Get("Authorization") != "Bearer private-pull-token" {
				t.Fatal("missing scoped registry token")
			}
			if !strings.Contains(req.Header.Get("Accept"), ociIndex) {
				t.Fatal("missing content negotiation")
			}
			var kind struct {
				MediaType string `json:"mediaType"`
			}
			switch req.URL.String() {
			case manifestURL + "8.3":
				_ = json.Unmarshal(index, &kind)
				resp = response(index, kind.MediaType)
			case manifestURL + hash(body):
				_ = json.Unmarshal(body, &kind)
				resp = response(body, kind.MediaType)
			default:
				t.Fatalf("unexpected registry endpoint: %s", req.URL)
			}
		}
		if alter != nil {
			return alter(req, resp)
		}
		return resp, nil
	})}
}

func TestResolvePreservesIndexAndPlatformIdentity(t *testing.T) {
	for _, media := range [][2]string{{ociIndex, ociManifest}, {dockerIndex, dockerManifest}} {
		t.Run(media[0], func(t *testing.T) {
			index, body := fixture(t, media[0], media[1])
			now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.FixedZone("local", -7*60*60))
			got, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			want := Resolution{Version: 1, Repository: Repository, Tag: "8.3", ResolvedAt: now.UTC(), Image: Repository + "@" + hash(index), IndexDigest: hash(index), ManifestDigest: hash(body), Platform: Platform}
			if !reflect.DeepEqual(got.Resolution, want) || !bytes.Equal(got.Index, index) || !bytes.Equal(got.Manifest, body) {
				t.Fatalf("resolution lost provenance: %+v", got.Resolution)
			}
			dir := filepath.Join(t.TempDir(), "candidate")
			if err := got.Save(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string][]byte{"index.json": index, "manifest.json": body} {
				b, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(b, want) {
					t.Fatalf("exact manifest not preserved: %s %v", name, err)
				}
			}
			original, err := os.ReadFile(filepath.Join(dir, "resolution.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := got.Save(context.Background(), dir); !errors.Is(err, os.ErrExist) {
				t.Fatalf("existing candidate replaced: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "resolution.json"))
			if !bytes.Equal(original, after) || bytes.Contains(original, []byte("private-pull-token")) {
				t.Fatal("receipt changed or leaked credential")
			}
		})
	}
}

func TestResolveRejectsUntrustedRegistryResponses(t *testing.T) {
	for _, tc := range []struct {
		name, at, want string
		alter          func(*http.Response)
	}{
		{"digest mismatch", "8.3", "digest", func(r *http.Response) { r.Header.Set("Docker-Content-Digest", "sha256:"+strings.Repeat("b", 64)) }},
		{"missing digest", "8.3", "digest", func(r *http.Response) { r.Header.Del("Docker-Content-Digest") }},
		{"wrong content type", "8.3", "supported image index", func(r *http.Response) { r.Header.Set("Content-Type", ociManifest) }},
		{"rate limited", "8.3", "HTTP 429", func(r *http.Response) {
			r.StatusCode = 429
			r.Body = io.NopCloser(strings.NewReader("private-server-detail"))
		}},
		{"oversized index", "8.3", "size limit", func(r *http.Response) {
			r.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", maxManifestBytes+1)))
		}},
		{"bad child digest", "sha256:", "does not match", func(r *http.Response) { r.Header.Set("Docker-Content-Digest", "sha256:"+strings.Repeat("c", 64)) }},
		{"truncated child", "sha256:", "does not match", func(r *http.Response) { r.Body = io.NopCloser(strings.NewReader("{}")) }},
		{"child content type", "sha256:", "does not match", func(r *http.Response) { r.Header.Set("Content-Type", ociIndex) }},
		{"missing token", "token?", "invalid token", func(r *http.Response) { r.Body = io.NopCloser(strings.NewReader(`{"token":""}`)) }},
		{"duplicate token", "token?", "invalid token", func(r *http.Response) { r.Body = io.NopCloser(strings.NewReader(`{"token":"a","token":"b"}`)) }},
		{"oversized token response", "token?", "size limit", func(r *http.Response) { r.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", (512<<10)+1))) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, body := fixture(t, ociIndex, ociManifest)
			client := registry(t, index, body, func(req *http.Request, resp *http.Response) (*http.Response, error) {
				if strings.Contains(req.URL.String(), tc.at) {
					tc.alter(resp)
				}
				return resp, nil
			})
			got, err := resolve(context.Background(), "8.3", client, time.Now)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "private-") || got.Resolution.Image != "" {
				t.Fatalf("unsafe response accepted or disclosed: %+v %v", got, err)
			}
		})
	}
}

func TestResolveRejectsAmbiguousPlatformAndForeignDescriptor(t *testing.T) {
	for _, change := range []string{"missing", "duplicate", "variant", "digest", "size", "urls", "nested", "duplicateKey"} {
		t.Run(change, func(t *testing.T) {
			index, body := fixture(t, ociIndex, ociManifest)
			var root manifest
			_ = json.Unmarshal(index, &root)
			switch change {
			case "missing":
				root.Manifests[0].Platform.Architecture = "arm64"
			case "duplicate":
				root.Manifests = append(root.Manifests, root.Manifests[0])
			case "variant":
				root.Manifests[0].Platform.Variant = "v99"
			case "digest":
				root.Manifests[0].Digest = "../../foreign"
			case "size":
				root.Manifests[0].Size = maxManifestBytes + 1
			case "urls":
				root.Manifests[0].URLs = []string{"https://foreign.invalid/manifest"}
			case "nested":
				root.Manifests[0].MediaType = ociIndex
			}
			index, _ = json.Marshal(root)
			if change == "duplicateKey" {
				index = []byte(strings.Replace(string(index), `"schemaVersion":2`, `"schemaVersion":1,"schemaVersion":2`, 1))
			}
			if _, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), time.Now); err == nil {
				t.Fatal("unsafe index accepted")
			}
		})
	}
}

func TestResolveNeverFollowsRedirects(t *testing.T) {
	for _, at := range []string{authURL, manifestURL + "8.3", "child"} {
		t.Run(at, func(t *testing.T) {
			index, body := fixture(t, ociIndex, ociManifest)
			if at == "child" {
				at = manifestURL + hash(body)
			}
			calls := 0
			client := registry(t, index, body, func(req *http.Request, resp *http.Response) (*http.Response, error) {
				calls++
				if req.URL.String() == at {
					resp.StatusCode = 302
					resp.Header.Set("Location", "https://foreign.invalid/collect")
				}
				return resp, nil
			})
			client.CheckRedirect = func(*http.Request, []*http.Request) error { t.Fatal("caller redirect hook invoked"); return nil }
			if _, err := resolve(context.Background(), "8.3", client, time.Now); err == nil || !strings.Contains(err.Error(), "HTTP 302") || calls > 3 {
				t.Fatalf("redirect not refused: %d %v", calls, err)
			}
		})
	}
}

func TestResolveDeadlineAndSanitizedTransportFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		local, _ := http.NewRequestWithContext(req.Context(), http.MethodGet, server.URL, nil)
		return http.DefaultTransport.RoundTrip(local)
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := resolve(ctx, "8.3", client, time.Now); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
	client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private-pull-token") })
	if _, err := resolve(context.Background(), "8.3", client, time.Now); err == nil || strings.Contains(err.Error(), "private-") {
		t.Fatalf("transport details leaked: %v", err)
	}
}

func TestInvalidTagsDoNotReachRegistry(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid tag reached registry"); return nil, nil })}
	for _, tag := range []string{"latest", "8.2", "8.3.01", "8.3.1/foreign", "8.3.1?scope=other", "foreign/8.3", "8.3-nightly", "8.3.100000"} {
		if _, err := resolve(context.Background(), tag, client, time.Now); err == nil {
			t.Fatalf("invalid tag accepted: %q", tag)
		}
	}
}

func TestSaveRefusesCorruptAndCanceledCandidate(t *testing.T) {
	index, body := fixture(t, ociIndex, ociManifest)
	c, err := resolve(context.Background(), "8.3", registry(t, index, body, nil), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "candidate")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Save(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.Index = []byte("corrupt")
	if err := c.Save(context.Background(), dir); err == nil {
		t.Fatal("corrupt bytes published")
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed resolution created output: %v", err)
	}
}
