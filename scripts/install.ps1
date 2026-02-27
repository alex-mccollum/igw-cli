[CmdletBinding()]
param(
  [string]$Version = "",
  [string]$Repo = "alex-mccollum/igw-cli",
  [string]$InstallDir = "$env:LOCALAPPDATA\Programs\igw\bin"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-LatestVersion {
  param([Parameter(Mandatory = $true)][string]$Repository)
  $url = "https://api.github.com/repos/$Repository/releases/latest"
  $release = Invoke-RestMethod -Uri $url
  if (-not $release.tag_name) {
    throw "Failed to resolve latest release tag for $Repository."
  }
  return [string]$release.tag_name
}

function Get-Arch {
  $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
  switch ($arch) {
    "X64" { return "amd64" }
    "Arm64" { return "arm64" }
    default { throw "Unsupported CPU architecture: $arch" }
  }
}

if ($Version -eq "") {
  $Version = Get-LatestVersion -Repository $Repo
}

if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') {
  throw "Version must use semantic tag format vMAJOR.MINOR.PATCH."
}

$arch = Get-Arch
$archiveName = "igw_${Version}_windows_${arch}.zip"
$baseUrl = "https://github.com/$Repo/releases/download/$Version"
$archiveUrl = "$baseUrl/$archiveName"
$checksumsUrl = "$baseUrl/checksums.txt"

$tmpRoot = Join-Path -Path ([System.IO.Path]::GetTempPath()) -ChildPath ("igw-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmpRoot | Out-Null

try {
  $archivePath = Join-Path -Path $tmpRoot -ChildPath $archiveName
  $checksumsPath = Join-Path -Path $tmpRoot -ChildPath "checksums.txt"
  $extractDir = Join-Path -Path $tmpRoot -ChildPath "extract"

  Write-Host "==> downloading $archiveUrl"
  Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath

  Write-Host "==> downloading $checksumsUrl"
  Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath

  $checksumLine = Select-String -Path $checksumsPath -Pattern ("  " + [Regex]::Escape($archiveName) + '$') | Select-Object -First 1
  if (-not $checksumLine) {
    throw "Checksum for $archiveName not found in checksums.txt."
  }
  $expected = ($checksumLine.Line -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
  if ($expected -ne $actual) {
    throw "Checksum mismatch for $archiveName. expected=$expected actual=$actual"
  }

  Write-Host "==> extracting $archiveName"
  Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force

  $binaryPath = Join-Path -Path $extractDir -ChildPath ("igw_${Version}_windows_${arch}\igw.exe")
  if (-not (Test-Path -Path $binaryPath -PathType Leaf)) {
    throw "Extracted binary not found: $binaryPath"
  }

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Copy-Item -Path $binaryPath -Destination (Join-Path $InstallDir "igw.exe") -Force

  Write-Host "ok: installed igw $Version to $InstallDir\igw.exe"
  Write-Host "verify: `"$InstallDir\igw.exe`" version"
} finally {
  if (Test-Path -Path $tmpRoot) {
    Remove-Item -Path $tmpRoot -Recurse -Force
  }
}
