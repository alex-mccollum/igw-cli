package fslock

import (
	"context"
	"errors"
	"os"
	"time"
)

func TryAcquire(path string) (*os.File, error) {
	prior, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil && !prior.Mode().IsRegular() {
		return nil, errors.New("file lock must be a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	opened, statErr := f.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || prior != nil && !os.SameFile(prior, opened) {
		f.Close()
		return nil, errors.New("file lock changed while opening it")
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	// Do not unlink the file: another process may already hold its descriptor.
	// Closing the descriptor releases the kernel lock, including after a crash.
	return f, nil
}

// ErrBusy means another process owns the lock. Unsupported locking fails closed.
var ErrBusy = errors.New("file is locked by another writer")

// Acquire waits for a short critical section, honoring cancellation. The lock
// file is persistent; callers release the kernel lock by closing the descriptor.
func Acquire(ctx context.Context, path string) (*os.File, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := TryAcquire(path)
		if !errors.Is(err, ErrBusy) {
			return f, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
