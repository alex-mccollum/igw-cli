package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestShortPrefersExplicitVersion(t *testing.T) {
	restore := snapshotBuildInfoState()
	defer restore()

	Version = "v9.9.9"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{
				Path:    "github.com/alex-mccollum/igw-cli",
				Version: "v0.3.0",
			},
		}, true
	}

	if got := Short(); got != "v9.9.9" {
		t.Fatalf("Short() = %q, want %q", got, "v9.9.9")
	}
}

func TestShortFallsBackToModuleVersion(t *testing.T) {
	restore := snapshotBuildInfoState()
	defer restore()

	Version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{
				Path:    "github.com/alex-mccollum/igw-cli",
				Version: "v0.3.0",
			},
		}, true
	}

	if got := Short(); got != "v0.3.0" {
		t.Fatalf("Short() = %q, want %q", got, "v0.3.0")
	}
}

func TestShortKeepsDevWhenModuleVersionUnavailable(t *testing.T) {
	restore := snapshotBuildInfoState()
	defer restore()

	Version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{
				Path:    "github.com/alex-mccollum/igw-cli",
				Version: "(devel)",
			},
		}, true
	}

	if got := Short(); got != "dev" {
		t.Fatalf("Short() = %q, want %q", got, "dev")
	}
}

func TestLongUsesResolvedShortVersion(t *testing.T) {
	restore := snapshotBuildInfoState()
	defer restore()

	Version = "dev"
	Commit = ""
	Date = ""
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{
				Path:    "github.com/alex-mccollum/igw-cli",
				Version: "v0.3.0",
			},
		}, true
	}

	if got := Long(); got != "v0.3.0" {
		t.Fatalf("Long() = %q, want %q", got, "v0.3.0")
	}
}

func snapshotBuildInfoState() func() {
	prevVersion := Version
	prevCommit := Commit
	prevDate := Date
	prevReadBuildInfo := readBuildInfo

	return func() {
		Version = prevVersion
		Commit = prevCommit
		Date = prevDate
		readBuildInfo = prevReadBuildInfo
	}
}
