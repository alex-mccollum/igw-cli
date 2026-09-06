//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package config

import (
	"os"
	"runtime"
	"syscall"
)

func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	runtime.KeepAlive(f)
	return err
}
