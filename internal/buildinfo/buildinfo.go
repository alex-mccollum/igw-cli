package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var (
	// Version is set at build time via -ldflags.
	Version = "dev"
	// Commit is set at build time via -ldflags.
	Commit = ""
	// Date is set at build time via -ldflags.
	Date = ""
)

var readBuildInfo = debug.ReadBuildInfo

func Short() string {
	version := strings.TrimSpace(Version)
	if version != "" && version != "dev" {
		return version
	}

	if bi, ok := readBuildInfo(); ok && bi != nil {
		moduleVersion := strings.TrimSpace(bi.Main.Version)
		if moduleVersion != "" && moduleVersion != "(devel)" {
			return moduleVersion
		}
	}

	if version == "" {
		return "dev"
	}
	return version
}

func Long() string {
	version := Short()
	commit := strings.TrimSpace(Commit)
	date := strings.TrimSpace(Date)
	if commit == "" && date == "" {
		return version
	}
	if date == "" {
		return fmt.Sprintf("%s (%s)", version, commit)
	}
	if commit == "" {
		return fmt.Sprintf("%s (%s)", version, date)
	}
	return fmt.Sprintf("%s (%s, %s)", version, commit, date)
}
