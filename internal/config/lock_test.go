package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/fslock"
)

func TestLockContentionSuggestsRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v1.lock")
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	other, err := acquireLock(path)
	if err == nil {
		other.Close()
		t.Fatal("acquired held lock")
	}
	if !errors.Is(err, fslock.ErrBusy) || !strings.Contains(err.Error(), "retry after the other writer finishes") || strings.Contains(err.Error(), path) {
		t.Fatalf("incorrect contention message: %v", err)
	}
}

func TestLockPermanentErrorsPreserveReasonWithoutPath(t *testing.T) {
	for _, kind := range []string{"missing parent", "directory", "symlink", "permission"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "private-config.v1.lock")
			var setupErr error
			var cause error
			want := "file lock must be a regular file"
			switch kind {
			case "missing parent":
				path = filepath.Join(dir, "missing", "config.v1.lock")
				cause, want = os.ErrNotExist, "cannot lock configuration"
			case "directory":
				setupErr = os.Mkdir(path, 0700)
			case "symlink":
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires privileges")
				}
				setupErr = os.Symlink(filepath.Join(dir, "outside"), path)
			case "permission":
				if runtime.GOOS == "windows" || os.Geteuid() == 0 {
					t.Skip("requires Unix permission enforcement")
				}
				setupErr = os.Chmod(dir, 0000)
				t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
				cause, want = os.ErrPermission, "permission denied"
			}
			if setupErr != nil {
				t.Fatal(setupErr)
			}
			lock, err := acquireLock(path)
			if err == nil {
				lock.Close()
				t.Fatal("unusable lock accepted")
			}
			if !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "retry") || strings.Contains(err.Error(), dir) || errors.Is(err, fslock.ErrBusy) || cause != nil && !errors.Is(err, cause) {
				t.Fatalf("incorrect permanent error: %v", err)
			}
		})
	}
}
