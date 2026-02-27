package installscripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallShLatestChannelContract(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell installer contract test is only supported on unix hosts")
	}

	arch, ok := installerArch()
	if !ok {
		t.Skipf("unsupported test architecture: %s", runtime.GOARCH)
	}

	root := repoRoot(t)
	expectedArchive := "igw_linux_" + arch + ".tar.gz"
	expectedBase := "https://github.com/example/repo/releases/latest/download"
	expectedExtractDir := "igw_v9.9.9_linux_" + arch

	runInstallShContract(t, root, []string{
		"--repo", "example/repo",
	}, expectedArchive, expectedBase, expectedExtractDir)
}

func TestInstallShPinnedVersionContract(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("shell installer contract test is only supported on unix hosts")
	}

	arch, ok := installerArch()
	if !ok {
		t.Skipf("unsupported test architecture: %s", runtime.GOARCH)
	}

	root := repoRoot(t)
	version := "v1.2.3"
	expectedArchive := "igw_" + version + "_linux_" + arch + ".tar.gz"
	expectedBase := "https://github.com/example/repo/releases/download/" + version
	expectedExtractDir := "igw_" + version + "_linux_" + arch

	runInstallShContract(t, root, []string{
		"--repo", "example/repo",
		"--version", version,
	}, expectedArchive, expectedBase, expectedExtractDir)
}

func TestInstallPowerShellNamingContract(t *testing.T) {
	root := repoRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "scripts", "install.ps1"))
	if err != nil {
		t.Fatalf("read scripts/install.ps1: %v", err)
	}
	script := string(content)

	required := []string{
		"function Get-ArchiveExt",
		"function Get-ExtractDirName",
		"function Get-VersionedArchiveName",
		"function Get-LatestAliasName",
		"https://github.com/$Repo/releases/latest/download",
		"https://github.com/$Repo/releases/download/$Version",
	}
	for _, token := range required {
		if !strings.Contains(script, token) {
			t.Fatalf("expected install.ps1 to contain %q", token)
		}
	}
}

func runInstallShContract(t *testing.T, root string, args []string, expectedArchive string, expectedBase string, expectedExtractDir string) {
	t.Helper()

	mockBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(mockBin, 0o755); err != nil {
		t.Fatalf("mkdir mock bin: %v", err)
	}
	urlLog := filepath.Join(t.TempDir(), "urls.log")
	installDir := filepath.Join(t.TempDir(), "install-dir")

	writeMockScript(t, filepath.Join(mockBin, "curl"), `#!/usr/bin/env bash
set -euo pipefail
url=""
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    -f|-s|-S|-L|-fsSL)
      shift
      ;;
    *)
      if [[ -z "$url" ]]; then
        url="$1"
      fi
      shift
      ;;
  esac
done
if [[ -z "$url" || -z "$out" ]]; then
  echo "unexpected curl args" >&2
  exit 1
fi
printf '%s\n' "$url" >> "$MOCK_URL_LOG"
if [[ "$url" == *"/checksums.txt" ]]; then
  printf '%s  %s\n' "$MOCK_SHA" "$MOCK_ARCHIVE" > "$out"
  exit 0
fi
if [[ "$url" == *"/$MOCK_ARCHIVE" ]]; then
  printf 'archive-bytes' > "$out"
  exit 0
fi
echo "unexpected url: $url" >&2
exit 1
`)

	writeMockScript(t, filepath.Join(mockBin, "sha256sum"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s  %s\n' "$MOCK_SHA" "$1"
`)

	writeMockScript(t, filepath.Join(mockBin, "tar"), `#!/usr/bin/env bash
set -euo pipefail
dest=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -C)
      dest="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$dest" ]]; then
  echo "missing tar destination" >&2
  exit 1
fi
mkdir -p "$dest/$MOCK_EXTRACT_DIR"
printf '#!/usr/bin/env bash\necho igw\n' > "$dest/$MOCK_EXTRACT_DIR/igw"
chmod 0755 "$dest/$MOCK_EXTRACT_DIR/igw"
`)

	writeMockScript(t, filepath.Join(mockBin, "install"), `#!/usr/bin/env bash
set -euo pipefail
src="${@: -2:1}"
dst="${@: -1}"
cp "$src" "$dst"
chmod 0755 "$dst"
`)

	env := append([]string{}, os.Environ()...)
	env = append(env,
		"PATH="+mockBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MOCK_URL_LOG="+urlLog,
		"MOCK_SHA=abc123",
		"MOCK_ARCHIVE="+expectedArchive,
		"MOCK_EXTRACT_DIR="+expectedExtractDir,
	)

	installScript := filepath.Join(root, "scripts", "install.sh")
	cmdArgs := append([]string{installScript}, args...)
	cmdArgs = append(cmdArgs, "--dir", installDir)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run install.sh: %v\n%s", err, out)
	}

	urlsBytes, err := os.ReadFile(urlLog)
	if err != nil {
		t.Fatalf("read url log: %v", err)
	}
	urls := strings.Fields(strings.TrimSpace(string(urlsBytes)))
	if len(urls) != 2 {
		t.Fatalf("expected 2 download URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != expectedBase+"/"+expectedArchive {
		t.Fatalf("unexpected archive URL: got=%q want=%q", urls[0], expectedBase+"/"+expectedArchive)
	}
	if urls[1] != expectedBase+"/checksums.txt" {
		t.Fatalf("unexpected checksums URL: got=%q want=%q", urls[1], expectedBase+"/checksums.txt")
	}

	binPath := filepath.Join(installDir, "igw")
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("expected installed binary at %s: %v", binPath, err)
	}
}

func writeMockScript(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write mock script %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func installerArch() (string, bool) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", true
	case "arm64":
		return "arm64", true
	default:
		return "", false
	}
}
