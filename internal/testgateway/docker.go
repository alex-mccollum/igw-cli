// Package testgateway manages disposable official Ignition containers for
// contract capture and integration tests. It never reuses an existing Gateway.
package testgateway

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ownerLabel = "io.igw-cli.qualification"
const captureName = "igw-qualification"
const maxLifetime = 10 * time.Minute

type commandRunner func(context.Context, io.Reader, ...string) ([]byte, error)

var pinnedImage = regexp.MustCompile(`^inductiveautomation/ignition@sha256:[a-f0-9]{64}$`)
var containerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Config struct {
	Docker  string
	Image   string
	Modules []string
	// Lifetime can lower the container's ten-minute maximum (e.g. for a
	// bounded lifecycle probe). It cannot raise it.
	Lifetime time.Duration
}

type Session struct {
	ID             string
	Name           string
	URL            string
	Image          string
	ImageID        string
	Platform       string
	Modules        []string
	GatewayVersion string
	run            commandRunner
	password       string
	owner          string
	created        bool
	closed         bool
	closeMu        sync.Mutex
	lifetime       time.Duration
}

// Start requires an already-pulled digest. Credentials enter the stopped
// container through a tar stream, never command arguments or Docker env values.
func Start(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Docker == "" {
		cfg.Docker = "docker"
	}
	return start(ctx, cfg, dockerRunner(cfg.Docker), func() error { return requireValidationBudget(os.ReadFile) })
}

func start(ctx context.Context, cfg Config, run commandRunner, admit func() error) (*Session, error) {
	if !pinnedImage.MatchString(cfg.Image) {
		return nil, errors.New("capture requires an official Ignition image pinned by SHA-256 digest")
	}
	for _, module := range cfg.Modules {
		if !strings.HasPrefix(module, "com.inductiveautomation.") || strings.ContainsAny(module, ", \t\r\n") {
			return nil, errors.New("capture module whitelist must contain explicit first-party identifiers")
		}
	}
	if err := admit(); err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 8*time.Minute {
		return nil, errors.New("capture requires an invocation deadline of at most eight minutes")
	}
	if cfg.Lifetime == 0 {
		cfg.Lifetime = maxLifetime
	}
	if cfg.Lifetime < time.Second || cfg.Lifetime > maxLifetime || cfg.Lifetime%time.Second != 0 {
		return nil, errors.New("capture container lifetime must be whole seconds between one second and ten minutes")
	}
	var entropy [24]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	s := &Session{Name: captureName, Image: cfg.Image, Modules: append([]string(nil), cfg.Modules...), run: run, owner: hex.EncodeToString(entropy[:8]), password: hex.EncodeToString(entropy[8:]) + "Aa1!", lifetime: cfg.Lifetime}
	image, err := s.preflight(ctx)
	if err != nil {
		return nil, err
	}
	s.ImageID, s.Platform = image.ID, image.OS+"/"+image.Architecture
	uid, gid, err := numericOwner(image.User)
	if err != nil {
		return nil, err
	}
	// The fixed name is an engine-wide exclusive slot. Its unique ownership
	// label prevents a losing/concurrent invocation from removing the winner.
	args := []string{"create", "--pull=never", "--platform", s.Platform, "--name", s.Name, "--label", ownerLabel + "=" + s.owner, "--publish", "127.0.0.1::8088", "--memory", "2g", "--memory-swap", "2g", "--cpus", "2", "--pids-limit", "256", "--restart", "no", "--stop-timeout", "15", "--entrypoint", "/usr/bin/timeout", "--log-opt", "max-size=10m", "--log-opt", "max-file=2",
		"--env", "ACCEPT_IGNITION_EULA=Y", "--env", "IGNITION_EDITION=standard", "--env", "GATEWAY_ADMIN_USERNAME=admin", "--env", "GATEWAY_ADMIN_PASSWORD_FILE=/run/igw-admin-password", "--env", "DISABLE_QUICKSTART=true", "--env", "GATEWAY_NETWORK_ENABLED=false"}
	if len(cfg.Modules) > 0 {
		args = append(args, "--env", "GATEWAY_MODULES_ENABLED="+strings.Join(cfg.Modules, ","))
	}
	// The container enforces its own lifetime even if this client or WSL dies.
	// An image without timeout fails startup; there is no unlimited fallback.
	args = append(args, cfg.Image, "--signal=TERM", "--kill-after=15s", strconv.Itoa(int(s.lifetime.Seconds()))+"s")
	args = append(args, image.Entrypoint...)
	args = append(args, "-n", "igw-reference", "-m", "1024")
	s.created = true
	b, err := s.command(ctx, nil, args...)
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	id := strings.TrimSpace(string(b))
	if !containerID.MatchString(id) {
		return nil, errors.Join(errors.New("Docker returned an invalid container ID"), s.Close())
	}
	s.ID = id
	if err := s.verifyConfiguration(ctx); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	secret := []byte(s.password)
	if err := w.WriteHeader(&tar.Header{Name: "igw-admin-password", Mode: 0400, Uid: uid, Gid: gid, Size: int64(len(secret)), Typeflag: tar.TypeReg}); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if _, err := w.Write(secret); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if err := w.Close(); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if _, err := s.command(ctx, &archive, "cp", "--archive", "-", s.ID+":/run/"); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if _, err := s.command(ctx, nil, "start", s.ID); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	if err := s.verifyKernelLimits(ctx); err != nil {
		return nil, errors.Join(err, s.Close())
	}
	port, err := s.command(ctx, nil, "port", s.ID, "8088/tcp")
	if err != nil {
		return nil, errors.Join(err, s.Close())
	}
	address := strings.TrimSpace(string(port))
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return nil, errors.Join(errors.New("capture requires exactly one loopback-only HTTP binding"), s.Close())
	}
	s.URL = "http://" + address
	return s, nil
}

func numericOwner(user string) (int, int, error) {
	if user == "" {
		return 0, 0, nil
	}
	parts := strings.Split(user, ":")
	uid, err := strconv.Atoi(parts[0])
	if err != nil || uid < 0 || len(parts) > 2 {
		return 0, 0, errors.New("capture image must declare a numeric container user")
	}
	gid := 0
	if len(parts) == 2 {
		gid, err = strconv.Atoi(parts[1])
		if err != nil || gid < 0 {
			return 0, 0, errors.New("capture image must declare a numeric container group")
		}
	}
	return uid, gid, nil
}

// Close removes only this uniquely labeled container and its anonymous data
// volumes. Cleanup has its own deadline so cancellation does not skip it.
func (s *Session) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if !s.created || s.closed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := s.Name
	if s.ID != "" {
		name = s.ID
	}
	data, err := s.command(ctx, nil, "inspect", "--format", `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "`+ownerLabel+`")}}}`, name)
	if err != nil {
		return err
	}
	var identity struct{ ID, Owner string }
	if json.Unmarshal(data, &identity) != nil || !containerID.MatchString(identity.ID) || identity.Owner != s.owner || (s.ID != "" && identity.ID != s.ID) {
		return errors.New("refusing to remove a container without this session's ownership label")
	}
	_, err = s.command(ctx, nil, "rm", "--force", "--volumes", identity.ID)
	if err == nil {
		s.closed = true
	}
	return err
}

func (s *Session) command(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return s.run(ctx, input, args...)
}

func dockerRunner(executable string) commandRunner {
	return func(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.WaitDelay = 2 * time.Second
		cmd.Stdin = input
		var stdout limitedOutput
		var stderr limitedBuffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("docker %s failed: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return stdout.Bytes(), nil
	}
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("Docker response exceeds size limit")
	}
	return b.Buffer.Write(p)
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 8192 - b.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}

func (s *Session) HTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
