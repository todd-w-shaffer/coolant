#!/bin/bash
set -euo pipefail

# coolant thermal dashboard installer
# Downloads the prebuilt binary for your architecture.

REPO="todd-w-shaffer/coolant"
BINARY="thermal"

echo ""
echo "  coolant thermal dashboard installer"
echo ""

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64)        ARCH="amd64" ;;
  *)
    echo "  error: unsupported architecture: $ARCH"
    echo "  coolant only supports macOS on Apple Silicon and Intel."
    exit 1
    ;;
esac

# Detect OS
OS="$(uname -s)"
if [ "$OS" != "Darwin" ]; then
  echo "  error: unsupported OS: $OS"
  echo "  the thermal dashboard is macOS only."
  exit 1
fi

ASSET="${BINARY}-darwin-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "  detected: macOS $ARCH"
echo ""

# Figure out install directory
DEFAULT_DIR="$HOME/.local/bin"
if [ -w "/usr/local/bin" ]; then
  DEFAULT_DIR="/usr/local/bin"
fi

printf "  install to [%s]: " "$DEFAULT_DIR"
read -r INSTALL_DIR
INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_DIR}"

# Expand ~ if someone types it
INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"

# Create directory if needed
if [ ! -d "$INSTALL_DIR" ]; then
  echo "  creating $INSTALL_DIR..."
  mkdir -p "$INSTALL_DIR"
fi

# Download
DEST="${INSTALL_DIR}/${BINARY}"
echo "  downloading ${ASSET}..."
if ! curl -fsSL "$URL" -o "$DEST"; then
  echo ""
  echo "  error: download failed."
  echo "  check https://github.com/${REPO}/releases for available binaries."
  exit 1
fi
chmod +x "$DEST"

echo "  installed to $DEST"

# Check PATH
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo ""
  echo "  note: $INSTALL_DIR is not on your PATH."
  echo "  add this to your shell profile (~/.zshrc or ~/.bashrc):"
  echo ""
  echo "    export PATH=\"$INSTALL_DIR:\$PATH\""
  echo ""
fi

echo ""
echo "  done. try: thermal --demo"
echo ""
