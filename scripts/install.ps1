# Cue — Install script for Windows (PowerShell)
# Usage: irm https://github.com/Tijani127/Cue/releases/latest/download/install.ps1 | iex

$Repo = "Tijani127/Cue"
$BinDir = $env:CUE_INSTALL_DIR
if (-not $BinDir) {
    $BinDir = "$env:LOCALAPPDATA\Programs\Cue"
}

# Detect architecture
switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64"  { $Arch = "amd64" }
    "ARM64"  { $Arch = "arm64" }
    "X86"    { $Arch = "386" }
    default  {
        Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

$Binary = "cue-windows-$Arch.exe"
$Url = "https://github.com/$Repo/releases/latest/download/$Binary"
$OutFile = Join-Path $BinDir "cue.exe"

Write-Host "Downloading Cue for Windows/$Arch ..."

# Create bin directory if needed
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
}

# Download
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
try {
    $null = Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing -ErrorAction Stop
} catch {
    Write-Error "Download failed: $_"
    exit 1
}

# Add to PATH if not already there
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$BinDir*") {
    $NewPath = "$UserPath;$BinDir"
    [Environment]::SetEnvironmentVariable("PATH", $NewPath, "User")
    $env:PATH = "$env:PATH;$BinDir"
    Write-Host "Added $BinDir to user PATH"
}

Write-Host "Installed Cue to $OutFile"
Write-Host "Run 'cue' to get started."
