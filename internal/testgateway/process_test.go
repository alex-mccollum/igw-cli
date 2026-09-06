package testgateway

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

const javaProcessFixture = "java\n42 (java) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 246810 20 21\n"

func TestJavaProcessStartTime(t *testing.T) {
	ticks, err := javaStartTicks([]byte(javaProcessFixture), 42)
	if err != nil || ticks != 246810 {
		t.Fatalf("wrong /proc field: %d %v", ticks, err)
	}
	if ticks, err := javaStartTicks([]byte(strings.ReplaceAll(javaProcessFixture, "java", "Main Thread")), 42); err != nil || ticks != 246810 {
		t.Fatal("JVM thread naming changed process start identity")
	}
	for _, raw := range []string{"", strings.Replace(javaProcessFixture, "java\n", "shell\n", 1), strings.Replace(javaProcessFixture, "42 (java)", "43 (java)", 1), strings.Replace(javaProcessFixture, " S ", " Z ", 1), "java\n42 (java) S 1", strings.Replace(javaProcessFixture, "246810", "-1", 1), strings.Replace(javaProcessFixture, "246810", "0", 1)} {
		if _, err := javaStartTicks([]byte(raw), 42); err == nil {
			t.Fatal("unqualified process status accepted")
		}
	}
}

func TestProcessObservationRequiresOwnershipAndContainment(t *testing.T) {
	for _, scenario := range []string{"valid", "unowned", "closed", "foreign", "wrong id", "stopped", "restarted", "config", "kernel", "pid", "executable"} {
		t.Run(scenario, func(t *testing.T) {
			f := &fakeDocker{t: t, owner: "owner"}
			s := &Session{ID: fixtureID, ImageID: fixtureImageID, owner: "owner", created: true}
			pid := int64(42)
			if scenario == "closed" {
				s.closed = true
			}
			if scenario == "unowned" {
				s.created = false
			}
			if scenario == "pid" {
				pid = 0
			}
			procReads := 0
			s.run = func(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
				if args[0] == "inspect" && strings.Contains(args[2], "startedAt") {
					id, owner, running, restarts := fixtureID, "owner", true, 0
					if scenario == "foreign" {
						owner = "foreign"
					}
					if scenario == "wrong id" {
						id = strings.Repeat("d", 64)
					}
					if scenario == "stopped" {
						running = false
					}
					if scenario == "restarted" {
						restarts = 1
					}
					return json.Marshal(map[string]any{"id": id, "owner": owner, "running": running, "restarts": restarts, "startedAt": "2026-09-06T00:00:00Z"})
				}
				if args[0] == "exec" && args[2] == "readlink" {
					if args[1] != fixtureID || len(args) != 4 || args[3] != "/proc/42/exe" {
						t.Fatal("executable lookup exceeded the owned read scope")
					}
					if scenario == "executable" {
						return []byte("/usr/bin/sh\n"), nil
					}
					return []byte("/opt/java/bin/java\n"), nil
				}
				if args[0] == "exec" && args[3] == "/proc/42/comm" {
					if args[1] != fixtureID || len(args) != 5 || args[2] != "cat" || args[4] != "/proc/42/stat" {
						t.Fatal("process observation exceeded the owned read scope")
					}
					procReads++
					return []byte(strings.ReplaceAll(javaProcessFixture, "java", "Main Thread")), nil
				}
				return f.run(ctx, input, args...)
			}
			if scenario == "config" {
				f.unsafeConfig = true
			}
			if scenario == "kernel" {
				f.kernel = "max\nmax\nmax 100000\nmax\n"
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			out, err := s.ObserveJavaProcess(ctx, pid)
			if scenario == "valid" {
				if err != nil || out.ProcessID != 42 || out.StartTicks != 246810 || !out.ContainmentVerified || out.ContainerID != fixtureID || out.ObservedAt.IsZero() || procReads != 2 {
					t.Fatalf("missing independent evidence: %+v %v", out, err)
				}
			} else if err == nil || procReads != 0 {
				t.Fatal("unsafe process observation proceeded")
			}
			if f.starts != 0 || f.removals != 0 {
				t.Fatal("process observation performed lifecycle control")
			}
		})
	}
}
