package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/alex-mccollum/igw-cli/internal/fslock"
)

func acquireLock(path string) (*os.File, error) {
	f, err := fslock.TryAcquire(path)
	if err != nil {
		if errors.Is(err, fslock.ErrBusy) {
			return nil, fmt.Errorf("configuration is busy; retry after the other writer finishes: %w", err)
		}
		// Keep the OS reason without exposing the user's configuration path.
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			err = pathError.Err
		}
		return nil, fmt.Errorf("cannot lock configuration: %w", err)
	}
	return f, nil
}
