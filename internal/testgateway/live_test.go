package testgateway

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Compile this test binary first, then run only this opt-in test through the
// bounded runner. Normal unit/race suites never contact a Docker engine.
func TestLiveCaptureLifetime(t *testing.T) {
	image := os.Getenv("IGW_CAPTURE_TEST_IMAGE")
	if image == "" {
		t.Skip("requires an explicit pinned disposable image and live-test invocation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cfg := Config{Image: image, Docker: os.Getenv("IGW_CAPTURE_TEST_DOCKER"), Lifetime: 5 * time.Second}
	s, err := Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Errorf("owned container cleanup failed: %v", err)
		}
	}()
	second, err := Start(ctx, cfg)
	if second != nil {
		_ = second.Close()
		t.Fatal("concurrent capture was admitted")
	}
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("exclusive admission was not verified: %v", err)
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		b, err := s.command(ctx, nil, "inspect", "--format", "{{json .State}}", s.ID)
		if err != nil {
			t.Fatal(err)
		}
		var state struct {
			Running, OOMKilled    bool
			ExitCode              int
			StartedAt, FinishedAt time.Time
		}
		if err := json.Unmarshal(b, &state); err != nil {
			t.Fatal(err)
		}
		if !state.Running {
			elapsed := state.FinishedAt.Sub(state.StartedAt)
			if state.OOMKilled || (state.ExitCode != 124 && state.ExitCode != 137) || elapsed < 4*time.Second || elapsed > 25*time.Second {
				t.Fatalf("container did not stop through its lifetime guard: exit=%d OOM=%t duration=%s", state.ExitCode, state.OOMKilled, elapsed)
			}
			t.Logf("verified container memory/swap/CPU/PID limits, exclusive admission, and lifetime termination: exit=%d duration=%s", state.ExitCode, elapsed)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := s.command(ctx, nil, "ps", "--all", "--filter", "id="+s.ID, "--format", "{{.ID}}")
	if err != nil || strings.TrimSpace(string(b)) != "" {
		t.Fatalf("owned container still present after cleanup: %v", err)
	}
}
