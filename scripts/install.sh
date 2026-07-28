#!/usr/bin/env bash
# Cue — Install script for Unix (Linux / macOS)
# Usage: curl -fsSL https://github.com/Tijani127/Cue/releases/latest/download/install.sh | bash

set -euo pipefail

REPO="Tijani127/Cue"
BIN_DIR="${CUE_INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)   OS="linux" ;;
  darwin)  OS="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64 | amd64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  i386 | i686)     ARCH="386" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

BINARY="cue-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${BINARY}"

# Use sudo if not running as root and installing to a system path
if [ "$(id -u)" -ne 0 ] && [[ "$BIN_DIR" == /usr/local/* || "$BIN_DIR" == /usr/bin/* ]]; then
  MAYBE_SUDO="sudo"
else
  MAYBE_SUDO=""
fi

echo "Downloading Cue for ${OS}/${ARCH}..."
TMPFILE="$(mktemp)"
cleanup() { rm -f "$TMPFILE"; }
trap cleanup EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$TMPFILE" "$URL"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMPFILE" "$URL"
else
  echo "Error: need curl or wget"
  exit 1
fi

chmod +x "$TMPFILE"
$MAYBE_SUDO mv "$TMPFILE" "${BIN_DIR}/cue"

echo "Installed Cue to ${BIN_DIR}/cue"
echo "Run 'cue --help' to get started."
