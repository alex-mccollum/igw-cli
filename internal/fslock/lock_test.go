package fslock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLockAcrossProcesses(t *testing.T) {
	if path := os.Getenv("IGW_TEST_LOCK_PATH"); path != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if f, err := Acquire(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
			if f != nil {
				f.Close()
			}
			t.Fatalf("lock did not exclude child process: %v", err)
		}
		return
	}
	path := filepath.Join(t.TempDir(), "lock")
	f, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	child := exec.Command(os.Args[0], "-test.run=^TestLockAcrossProcesses$")
	child.Env = append(os.Environ(), "IGW_TEST_LOCK_PATH="+path)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, out)
	}
	f.Close()
	next, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestLockRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, nil, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if f, err := Acquire(context.Background(), link); err == nil {
		f.Close()
		t.Fatal("symlink accepted")
	}
}
