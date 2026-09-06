package testgateway

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWrapperJavaChildObservation(t *testing.T) {
	for _, scenario := range []string{"one child", "duplicate child", "no java", "multiple java", "wrong parent", "invalid child"} {
		t.Run(scenario, func(t *testing.T) {
			f := &fakeDocker{t: t, owner: "owner"}
			s := &Session{ID: fixtureID, ImageID: fixtureImageID, owner: "owner", created: true}
			s.run = func(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
				if args[0] == "inspect" && strings.Contains(args[2], "startedAt") {
					return json.Marshal(map[string]any{"id": fixtureID, "owner": "owner", "running": true, "restarts": 0, "startedAt": "2026-09-06T00:00:00Z"})
				}
				if args[0] == "exec" && args[2] == "sh" {
					if args[1] != fixtureID || len(args) != 7 || args[6] != "42" || !strings.Contains(args[4], `/proc/"$1"/task/*/children`) {
						t.Fatal("child lookup exceeded the owned wrapper scope")
					}
					switch scenario {
					case "duplicate child":
						return []byte("43\n43\n"), nil
					case "multiple java":
						return []byte("43 44\n"), nil
					case "invalid child":
						return []byte("../../other\n"), nil
					default:
						return []byte("43\n"), nil
					}
				}
				if args[0] == "exec" && args[2] == "readlink" {
					switch args[3] {
					case "/proc/42/exe":
						return []byte("/opt/ignition/ignition-gateway\n"), nil
					case "/proc/43/exe", "/proc/44/exe":
						if scenario == "no java" {
							return []byte("/bin/sh\n"), nil
						}
						return []byte("/opt/java/bin/java\n"), nil
					default:
						t.Fatal("unexpected executable observation")
					}
				}
				if args[0] == "exec" && args[2] == "cat" {
					switch args[3] {
					case "/proc/42/comm":
						return []byte(strings.ReplaceAll(javaProcessFixture, "java", "wrapper")), nil
					case "/proc/43/comm":
						stat := strings.Replace(javaProcessFixture, "42 (java) S 1 ", "43 (java) S 42 ", 1)
						if scenario == "wrong parent" {
							stat = strings.Replace(stat, " S 42 ", " S 41 ", 1)
						}
						return []byte(strings.Replace(stat, "246810", "357911", 1)), nil
					}
				}
				return f.run(ctx, input, args...)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			out, err := s.ObserveJavaProcess(ctx, 42)
			if scenario == "one child" || scenario == "duplicate child" {
				if err != nil || out.ProcessID != 43 || out.ReportedProcessID != 42 || out.ReportedExecutable != "ignition-gateway" || out.ReportedStartTicks != 246810 || out.StartTicks != 357911 || !out.ContainmentVerified {
					t.Fatalf("wrapper/JVM identities conflated: %+v %v", out, err)
				}
			} else if err == nil {
				t.Fatal("unqualified Java child accepted")
			}
			if f.starts != 0 || f.removals != 0 {
				t.Fatal("observation changed container lifecycle")
			}
		})
	}
}
