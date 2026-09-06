package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const legacyFixture = `{
  "gatewayURL": "https://default.invalid/proxy", "token": "private-default",
  "activeProfile": "line-a",
  "profiles": {
    "line-a": {"gatewayURL": "https://a.invalid", "token": "private-a"},
    "line-b": {"gatewayURL": "https://b.invalid", "token": "private-b"}
  }
}`

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeLegacy(t *testing.T, dir, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, LegacyFilename), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationPreservesResolutionAndRollbackArchivesEdits(t *testing.T) {
	s := Store{Dir: privateDir(t)}
	writeLegacy(t, s.Dir, legacyFixture)
	before, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.Change(context.Background(), Change{Action: "migrate", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.Before.Source != "legacy" || preview.After.Source != "v1" || preview.After.Revision != "" {
		t.Fatal("incorrect preview")
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 {
		t.Fatal("preview wrote configuration")
	}
	change, err := s.Change(context.Background(), Change{Action: "migrate", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !change.Applied || !hexLength(after.Revision, 16) || !reflect.DeepEqual(before.Config, after.Config) {
		t.Fatal("migration changed configuration")
	}
	for _, profile := range []string{"", "line-a", "line-b"} {
		for _, url := range []string{"", "https://flag.invalid"} {
			for _, env := range []bool{false, true} {
				getenv := func(key string) string {
					if env {
						if key == EnvGatewayURL {
							return "https://env.invalid"
						}
						return "private-env"
					}
					return ""
				}
				left, e1 := ResolveWithProfile(before.Config, getenv, url, "", profile)
				right, e2 := ResolveWithProfile(after.Config, getenv, url, "", profile)
				if e1 != nil || e2 != nil || left != right {
					t.Fatal("resolution changed during migration")
				}
			}
		}
	}
	token := "private-rotated"
	edit, err := s.Change(context.Background(), Change{Action: "set", Name: "line-b", Token: &token, IfRevision: after.Revision, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if edit.After.Revision == after.Revision {
		t.Fatal("edit reused revision")
	}
	if edit.TokenChange != "set" {
		t.Fatal("rotation omitted its explicit token action")
	}
	if _, err = s.Change(context.Background(), Change{Action: "rollback", IfRevision: after.Revision, Yes: true}); err == nil {
		t.Fatal("stale rollback accepted")
	}
	v1, _ := os.ReadFile(filepath.Join(s.Dir, V1Filename))
	back, err := s.Change(context.Background(), Change{Action: "rollback", IfRevision: edit.After.Revision, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if restored.Source != "legacy" || !reflect.DeepEqual(restored.Config, before.Config) {
		t.Fatal("rollback changed legacy settings")
	}
	archive, err := os.ReadFile(filepath.Join(s.Dir, back.Archive))
	if err != nil || !bytes.Equal(archive, v1) {
		t.Fatal("rollback failed to preserve complete v1 bytes")
	}
	legacy, _ := os.ReadFile(filepath.Join(s.Dir, LegacyFilename))
	if string(legacy) != legacyFixture {
		t.Fatal("legacy bytes changed")
	}
	for _, report := range []any{preview, change, edit, back, after} {
		b, _ := json.Marshal(report)
		if bytes.Contains(b, []byte("private-")) || bytes.Contains(b, []byte("legacySHA256")) {
			t.Fatal("report exposed credential material")
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(s.Dir, back.Archive))
		if info.Mode().Perm() != 0600 {
			t.Fatal("archive is not private")
		}
	}
}

func TestConfigChangesRequireConfirmationAndDoNotPersistEnvironment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	s := Store{Dir: dir}
	url := "https://configured.invalid/proxy"
	for _, r := range []Change{{Action: "set", Name: "dev", GatewayURL: &url}, {Action: "set", Name: "dev", GatewayURL: &url, Yes: true, DryRun: true}, {Action: "rollback", DryRun: true}} {
		if _, err := s.Change(context.Background(), r); err == nil {
			t.Fatal("invalid edit accepted")
		}
	}
	preview, err := s.Change(context.Background(), Change{Action: "set", Name: "dev", GatewayURL: &url, Activate: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.After.ActiveProfile != "dev" {
		t.Fatal("activation missing")
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("preview created directory")
	}
	first, err := s.Change(context.Background(), Change{Action: "set", Name: "dev", GatewayURL: &url, Activate: true, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(context.Background(), Change{Action: "remove", Name: "dev", Yes: true}); err == nil {
		t.Fatal("removed active profile")
	}
	if _, err = s.Change(context.Background(), Change{Action: "use", UseDefaults: true, IfRevision: first.After.Revision, Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(context.Background(), Change{Action: "remove", Name: "dev", Yes: true}); err != nil {
		t.Fatal(err)
	}
	state, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Config.ActiveProfile != "" || len(state.Config.Profiles) != 0 {
		t.Fatal("profile removal failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Change(ctx, Change{Action: "set", Name: "canceled", GatewayURL: &url, Yes: true}); err != context.Canceled {
		t.Fatal("cancellation ignored")
	}
}

func TestStrictConfigNeverDropsAmbiguousOrUnknownFields(t *testing.T) {
	for name, raw := range map[string]string{
		"duplicate":  `{"token":"first","token":"second"}`,
		"case":       `{"Token":"private-value"}`,
		"unknown":    `{"private-value":true}`,
		"wrong type": `{"gatewayURL":42}`,
		"null":       `null`, "null token": `{"token":null}`, "null profile": `{"profiles":{"dev":null}}`,
		"null profiles": `{"profiles":null}`, "array": `[]`, "trailing": `{} {}`,
		"unicode": `{"token":"\ud800"}`, "utf8": "{\"token\":\"\xff\"}",
		"profile unknown":   `{"profiles":{"dev":{"private-value":true}}}`,
		"profile duplicate": `{"profiles":{"dev":{},"dev":{}}}`,
		"unreachable name":  `{"profiles":{" dev ":{}}}`,
		"active missing":    `{"activeProfile":"missing"}`,
		"credentialed url":  `{"gatewayURL":"https://user:private-value@gateway.invalid"}`,
		"query url":         `{"profiles":{"dev":{"gatewayURL":"https://gateway.invalid?token=private-value"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			writeLegacy(t, s.Dir, raw)
			_, err := s.Change(context.Background(), Change{Action: "migrate", Yes: true})
			if err == nil || strings.Contains(err.Error(), "private-value") {
				t.Fatal("invalid configuration accepted or leaked")
			}
			entries, _ := os.ReadDir(s.Dir)
			if len(entries) != 1 {
				t.Fatal("failed migration wrote files")
			}
		})
	}
}

func TestConfigRefusesBadV1WithoutLegacyFallback(t *testing.T) {
	s := Store{Dir: privateDir(t)}
	writeLegacy(t, s.Dir, legacyFixture)
	for _, raw := range []string{`{}`, `null`, `{"schemaVersion":"future","revision":"00000000000000000000000000000000","configuration":{}}`, strings.Repeat(" ", maxConfigBytes+1)} {
		if err := os.WriteFile(filepath.Join(s.Dir, V1Filename), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("invalid v1 fell back to legacy")
		}
	}
}

func TestMigrationPreviewRejectsExpandedFileSize(t *testing.T) {
	s := Store{Dir: privateDir(t)}
	// A compact legacy file fits the read bound, but its preserved v1 document
	// and indentation do not. Preview must refuse before claiming it can migrate.
	writeLegacy(t, s.Dir, `{"token":"`+strings.Repeat("x", maxConfigBytes-100)+`"}`)
	for _, apply := range []bool{false, true} {
		if _, err := s.Change(context.Background(), Change{Action: "migrate", DryRun: !apply, Yes: apply}); err == nil {
			t.Fatal("oversized migration accepted")
		}
		entries, _ := os.ReadDir(s.Dir)
		if len(entries) != 1 {
			t.Fatal("oversized migration created files")
		}
	}
}

func TestRollbackArchiveCanResumeButCannotBeReplaced(t *testing.T) {
	for _, identical := range []bool{false, true} {
		s := Store{Dir: privateDir(t)}
		writeLegacy(t, s.Dir, legacyFixture)
		migrated, err := s.Change(context.Background(), Change{Action: "migrate", Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(s.Dir, V1Filename))
		archive := append([]byte(nil), raw...)
		if !identical {
			archive = []byte("different private content")
		}
		path := filepath.Join(s.Dir, "config.v1.rollback-"+migrated.After.Revision+".json")
		if err = os.WriteFile(path, archive, 0600); err != nil {
			t.Fatal(err)
		}
		_, err = s.Change(context.Background(), Change{Action: "rollback", IfRevision: migrated.After.Revision, Yes: true})
		if (err == nil) != identical {
			t.Fatal("rollback archive handling disagrees with byte equality")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(after, archive) {
			t.Fatal("archive was replaced")
		}
		state, err := s.Read()
		if err != nil {
			t.Fatal(err)
		}
		if (state.Source == "legacy") != identical {
			t.Fatal("rollback selected wrong configuration")
		}
	}
}

func TestProfileEditsRejectMalformedNameAndTargetBeforeWriting(t *testing.T) {
	for _, value := range []struct{ name, url string }{
		{"bad\xff", "https://gateway.invalid"},
		{"dev", "https://gateway.invalid/\xff"},
		{"dev", "https://gateway.invalid/../admin"},
		{"dev", "https://gateway.invalid/%2e%2e/admin"},
		{"dev", "https://gateway.invalid/proxy%5cadmin"},
	} {
		s := Store{Dir: filepath.Join(t.TempDir(), "not-created")}
		for _, apply := range []bool{false, true} {
			if _, err := s.Change(context.Background(), Change{Action: "set", Name: value.name, GatewayURL: &value.url, DryRun: !apply, Yes: apply}); err == nil {
				t.Fatal("invalid explicit value accepted")
			}
			if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
				t.Fatal("invalid edit created config directory")
			}
		}
	}
}

func TestMigrationAndRollbackRefuseChangedLegacy(t *testing.T) {
	s := Store{Dir: privateDir(t)}
	writeLegacy(t, s.Dir, legacyFixture)
	url := "https://new.invalid"
	if _, err := s.Change(context.Background(), Change{Action: "set", Name: "new", GatewayURL: &url, Yes: true}); err == nil {
		t.Fatal("legacy modified without migration")
	}
	change, err := s.Change(context.Background(), Change{Action: "migrate", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(context.Background(), Change{Action: "migrate", Yes: true}); err == nil {
		t.Fatal("repeated migration accepted")
	}
	writeLegacy(t, s.Dir, legacyFixture+"\n")
	if _, err = s.Change(context.Background(), Change{Action: "rollback", IfRevision: change.After.Revision, Yes: true}); err == nil {
		t.Fatal("changed legacy accepted for rollback")
	}
	state, _ := s.Read()
	if state.Revision != change.After.Revision {
		t.Fatal("failed rollback changed v1")
	}
}

func TestConfigRefusesSymlinksAndInsecureWriteDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation and POSIX permissions are separately platform qualified")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte(legacyFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, LegacyFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Read(); err == nil {
		t.Fatal("symlinked credentials accepted")
	}
	dir = privateDir(t)
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	url := "https://gateway.invalid"
	if _, err := (Store{Dir: dir}).Change(context.Background(), Change{Action: "set", Name: "dev", GatewayURL: &url, Yes: true}); err == nil {
		t.Fatal("insecure directory accepted")
	}
	dir = privateDir(t)
	if err := os.Symlink(outside, filepath.Join(dir, "config.v1.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Change(context.Background(), Change{Action: "set", Name: "dev", GatewayURL: &url, Yes: true}); err == nil {
		t.Fatal("symlinked lock accepted")
	}
	b, _ := os.ReadFile(outside)
	if string(b) != legacyFixture {
		t.Fatal("external file changed")
	}
}

func TestConcurrentProfileEditsDoNotLoseSuccessfulChanges(t *testing.T) {
	s := Store{Dir: privateDir(t)}
	url := "https://gateway.invalid"
	var wg sync.WaitGroup
	success := make(chan string, 12)
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("profile-%d", n)
			if _, err := s.Change(context.Background(), Change{Action: "set", Name: name, GatewayURL: &url, Yes: true}); err == nil {
				success <- name
			}
		}(n)
	}
	wg.Wait()
	close(success)
	state, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for name := range success {
		count++
		if state.Config.Profiles[name].GatewayURL != url {
			t.Fatal("lost a successful concurrent edit")
		}
	}
	if count == 0 || len(state.Config.Profiles) != count {
		t.Fatal("writer results do not match stored profiles")
	}
}

// Use a separate process to test OS locks rather than just Go mutex behavior.
func TestConfigLockProcessHelper(t *testing.T) {
	path := os.Getenv("IGW_TEST_CONFIG_LOCK")
	if path == "" {
		return
	}
	f, err := acquireLock(path)
	if err != nil {
		os.Exit(20)
	}
	defer f.Close()
	fmt.Fprintln(os.Stdout, "locked")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestConfigKernelLockReleasedAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestConfigLockProcessHelper$")
	cmd.Env = append(os.Environ(), "IGW_TEST_CONFIG_LOCK="+path)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var signal [7]byte
	if n, err := io.ReadFull(output, signal[:]); err != nil || string(signal[:n]) != "locked\n" {
		t.Fatal("child failed to acquire lock")
	}
	if f, err := acquireLock(path); err == nil {
		f.Close()
		t.Fatal("another process acquired held lock")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	f, err := acquireLock(path)
	if err != nil {
		t.Fatal("crashed process left a stale lock")
	}
	f.Close()
}
