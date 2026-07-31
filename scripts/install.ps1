# Cue — Installer for Windows
# Downloads the official binary from GitHub Releases.
#
# Usage:
#   irm https://raw.githubusercontent.com/Tijani127/Cue/main/scripts/install.ps1 | iex
#   powershell -ExecutionPolicy Bypass -File install.ps1 -AddToPath

param(
    [switch]$AddToPath,
    [string]$InstallDir = "$env:LOCALAPPDATA\Programs\Cue"
)

$ErrorActionPreference = "Stop"
$Repo = "Tijani127/Cue"

# Detect architecture
switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    "X86"   { $Arch = "386" }
    default {
        Write-Host "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

$Url = "https://github.com/$Repo/releases/latest/download/cue-windows-$Arch.exe"
$OutFile = Join-Path $InstallDir "cue.exe"

Write-Host "Downloading Cue for Windows/$Arch ..."
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

# Use the native curl.exe binary (available on Windows 10+).
& curl.exe -L --fail --silent --show-error -o $OutFile $Url
if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "Download failed. If Windows Defender blocked the file, either:"
    Write-Host "  1. Add an exclusion for $OutFile in Windows Security, then re-run, or"
    Write-Host "  2. Download the binary manually from: $Url"
    exit 1
}

Write-Host "Installed Cue to $OutFile"

if ($AddToPath) {
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
        Write-Host "Added $InstallDir to user PATH"
    }
} else {
    Write-Host "Add '$InstallDir' to your PATH to run 'cue' from anywhere."
}
