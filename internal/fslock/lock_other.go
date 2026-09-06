//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package fslock

import (
	"errors"
	"os"
)

func lockFile(*os.File) error {
	return errors.New("file locking is unsupported on this platform")
}
