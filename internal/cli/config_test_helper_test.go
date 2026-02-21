package cli

import (
	"path/filepath"
	"testing"

	"github.com/alex-mccollum/igw-cli/internal/config"
)

// setIsolatedConfigDir forces os.UserConfigDir() to resolve inside a test temp dir
// across Linux, macOS, and Windows.
func setIsolatedConfigDir(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("resolve config dir: %v", err)
	}
	return dir
}
