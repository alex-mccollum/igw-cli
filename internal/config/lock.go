package config

import (
	"errors"
	"github.com/alex-mccollum/igw-cli/internal/fslock"
	"os"
)

func acquireLock(path string) (*os.File, error) {
	f, err := fslock.TryAcquire(path)
	if err != nil {
		return nil, errors.New("configuration is busy or file locking is unavailable; retry after the other writer finishes")
	}
	return f, nil
}
