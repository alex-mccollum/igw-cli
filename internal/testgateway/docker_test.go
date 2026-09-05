package testgateway

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

const fixtureImage = "inductiveautomation/ignition@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const fixtureID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const fixtureImageID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

type fakeDocker struct {
	t                                                       *testing.T
	calls                                                   [][]string
	owner, password, existing, kernel                       string
	imageOS, imageArch, imageIdentity, containerImage       string
	unsafeConfig, collide, wrongID, foreignOwner, cpFailure bool
	starts, removals                                        int
}

func (f *fakeDocker) run(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	f.t.Helper()
	if ctx.Err() != nil {
		f.t.Fatal("Docker operation inherited expired context")
	}
	f.calls = append(f.calls, append([]string(nil), args...))
	encode := func(v any) ([]byte, error) { return json.Marshal(v) }
	switch args[0] {
	case "info":
		return []byte(`{"OSType":"linux","CgroupVersion":"2","MemoryLimit":true,"SwapLimit":true,"CpuCfsQuota":true,"PidsLimit":true}`), nil
	case "ps":
		return []byte(f.existing), nil
	case "image":
		osName, arch, identity := f.imageOS, f.imageArch, f.imageIdentity
		if osName == "" {
			osName = "linux"
		}
		if arch == "" {
			arch = "amd64"
		}
		if identity == "" {
			identity = fixtureImageID
		}
		return encode(map[string]any{"entrypoint": []string{"docker-entrypoint.sh"}, "user": "2003:2003", "os": osName, "architecture": arch, "id": identity})
	case "create":
		for n, arg := range args {
			if arg == "--label" {
				f.owner = strings.TrimPrefix(args[n+1], ownerLabel+"=")
			}
		}
		if f.collide {
			return nil, errors.New("name already in use")
		}
		return []byte(fixtureID), nil
	case "inspect":
		if args[2] == "{{.Image}}" {
			if f.containerImage != "" {
				return []byte(f.containerImage), nil
			}
			return []byte(fixtureImageID), nil
		}
		if args[2] == "{{json .HostConfig}}" {
			swap := int64(2 << 30)
			if f.unsafeConfig {
				swap = -1
			}
			return encode(map[string]any{"Memory": 2 << 30, "MemorySwap": swap, "NanoCpus": 2e9, "PidsLimit": 256, "RestartPolicy": map[string]any{"Name": "no"}, "PortBindings": map[string]any{"8088/tcp": []any{map[string]any{"HostIp": "127.0.0.1", "HostPort": ""}}}})
		}
		owner, id := f.owner, fixtureID
		if f.collide || f.foreignOwner {
			owner = "another-session"
		}
		if f.wrongID {
			id = strings.Repeat("c", 64)
		}
		return encode(map[string]string{"id": id, "owner": owner})
	case "cp":
		reader := tar.NewReader(input)
		header, err := reader.Next()
		if err != nil || header.Uid != 2003 || header.Gid != 2003 || header.Mode != 0400 {
			f.t.Fatal("credential file is not private to image user")
		}
		secret, err := io.ReadAll(reader)
		if err != nil || len(secret) < 32 {
			f.t.Fatal("temporary credential missing")
		}
		f.password = string(secret)
		if f.cpFailure {
			return nil, errors.New("copy failed")
		}
		return nil, nil
	case "start":
		f.starts++
		return nil, nil
	case "exec":
		if f.kernel != "" {
			return []byte(f.kernel), nil
		}
		return []byte("2147483648\n0\n200000 100000\n256\n"), nil
	case "port":
		return []byte("127.0.0.1:32888\n"), nil
	case "rm":
		if args[len(args)-1] != fixtureID {
			f.t.Fatal("cleanup did not use exact owned ID")
		}
		f.removals++
		return nil, nil
	default:
		f.t.Fatalf("unexpected Docker command %q", args[0])
		return nil, nil
	}
}

func fakeStart(t *testing.T, f *fakeDocker) (*Session, context.CancelFunc, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	s, err := start(ctx, Config{Image: fixtureImage}, f.run, func() error { return nil })
	return s, cancel, err
}

func TestCaptureLimitsCredentialsAndCanceledCleanup(t *testing.T) {
	f := &fakeDocker{t: t}
	s, cancel, err := fakeStart(t, f)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel() // Cleanup must own a fresh deadline, independent of capture.
	if s.URL != "http://127.0.0.1:32888" {
		t.Fatal("wrong capture target")
	}
	if s.Platform != "linux/amd64" || s.ImageID != fixtureImageID {
		t.Fatal("observed image provenance missing")
	}
	var create []string
	for _, call := range f.calls {
		if call[0] == "create" {
			create = call
		}
		if strings.Contains(strings.Join(call, " "), f.password) {
			t.Fatal("temporary password exposed in process arguments")
		}
	}
	joined := strings.Join(create, " ")
	for _, required := range []string{"--platform linux/amd64", "--name " + captureName, "--memory 2g --memory-swap 2g", "--cpus 2 --pids-limit 256", "--restart no", "--entrypoint /usr/bin/timeout", "--signal=TERM --kill-after=15s 600s docker-entrypoint.sh"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing capture containment: %s", required)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil || f.removals != 1 {
		t.Fatal("cleanup is not idempotent")
	}
}

func TestCaptureAdmissionAndCleanupRefuseForeignContainers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*fakeDocker)
	}{
		{"existing", func(f *fakeDocker) { f.existing = "existing-id" }},
		{"collision", func(f *fakeDocker) { f.collide = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDocker{t: t}
			tc.configure(f)
			_, cancel, err := fakeStart(t, f)
			defer cancel()
			if err == nil || f.starts != 0 || f.removals != 0 {
				t.Fatal("foreign container was reused or removed")
			}
		})
	}
	for _, wrongID := range []bool{false, true} {
		f := &fakeDocker{t: t}
		s, cancel, err := fakeStart(t, f)
		defer cancel()
		if err != nil {
			t.Fatal(err)
		}
		f.foreignOwner = !wrongID
		f.wrongID = wrongID
		if err := s.Close(); err == nil || f.removals != 0 {
			t.Fatal("ownership mismatch did not stop cleanup")
		}
	}
}

func TestCaptureRejectsUnverifiedPlatformBeforeStartup(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*fakeDocker)
		removals  int
	}{
		{"architecture", func(f *fakeDocker) { f.imageArch = "arm64" }, 0},
		{"operating system", func(f *fakeDocker) { f.imageOS = "windows" }, 0},
		{"image digest", func(f *fakeDocker) { f.imageIdentity = "sha256:invalid" }, 0},
		{"container image", func(f *fakeDocker) { f.containerImage = "sha256:" + strings.Repeat("d", 64) }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDocker{t: t}
			tc.configure(f)
			s, cancel, err := fakeStart(t, f)
			defer cancel()
			if err == nil || s != nil || f.starts != 0 || f.removals != tc.removals {
				t.Fatalf("unverified image started or cleanup lost: starts=%d removals=%d err=%v", f.starts, f.removals, err)
			}
		})
	}
}

func TestCaptureFailureCleansOnlyItsContainer(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*fakeDocker)
		starts    int
	}{
		{"unbounded-swap", func(f *fakeDocker) { f.unsafeConfig = true }, 0},
		{"copy", func(f *fakeDocker) { f.cpFailure = true }, 0},
		{"kernel-cpu", func(f *fakeDocker) { f.kernel = "2147483648 0 max 100000 256" }, 1},
		{"kernel-memory", func(f *fakeDocker) { f.kernel = "max 0 200000 100000 256" }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeDocker{t: t}
			tc.configure(f)
			_, cancel, err := fakeStart(t, f)
			defer cancel()
			if err == nil || f.starts != tc.starts || f.removals != 1 {
				t.Fatalf("failure not contained: starts=%d removals=%d err=%v", f.starts, f.removals, err)
			}
		})
	}
}

func TestCaptureRequiresDeadlineAndAdmission(t *testing.T) {
	f := &fakeDocker{t: t}
	if _, err := start(context.Background(), Config{Image: fixtureImage}, f.run, func() error { return nil }); err == nil {
		t.Fatal("unbounded capture accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := start(ctx, Config{Image: fixtureImage}, f.run, func() error { return errors.New("limits unavailable") }); err == nil {
		t.Fatal("admission failure ignored")
	}
	if len(f.calls) != 0 {
		t.Fatal("Docker invoked before admission")
	}
}

func TestCaptureVerifiesItsProcessBudget(t *testing.T) {
	group := "/user.slice/user@1000.service/igw-validation-test.scope"
	files := map[string]string{"/proc/self/cgroup": "0::" + group + "\n"}
	for key, value := range map[string]string{"memory.max": "8589934592", "memory.swap.max": "0", "pids.max": "256"} {
		files["/sys/fs/cgroup"+group+"/"+key] = value
	}
	read := func(path string) ([]byte, error) {
		value, ok := files[path]
		if !ok {
			return nil, errors.New("missing")
		}
		return []byte(value), nil
	}
	if err := requireValidationBudget(read); err != nil {
		t.Fatal(err)
	}
	files["/sys/fs/cgroup"+group+"/memory.max"] = "max"
	if err := requireValidationBudget(read); err == nil {
		t.Fatal("unbounded client admitted")
	}
	files["/proc/self/cgroup"] = "0::/init.scope"
	if err := requireValidationBudget(read); err == nil {
		t.Fatal("unguarded client admitted")
	}
}
