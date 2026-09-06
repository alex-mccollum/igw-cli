//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package config

import (
	"errors"
	"os"
)

func lockFile(*os.File) error {
	return errors.New("configuration locking is unsupported on this platform")
}
