package config

import (
	"errors"
	"os"
)

func acquireLock(path string) (*os.File, error) {
	prior, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && !prior.Mode().IsRegular() {
		return nil, errors.New("configuration lock must be a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("could not open configuration lock")
	}
	opened, statErr := f.Stat()
	current, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || prior != nil && !os.SameFile(prior, opened) {
		f.Close()
		return nil, errors.New("configuration lock changed while opening it")
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, errors.New("configuration is busy or file locking is unavailable; retry after the other writer finishes")
	}
	// Do not unlink the file: another process may already hold its descriptor.
	// Closing the descriptor releases the kernel lock, including after a crash.
	return f, nil
}
