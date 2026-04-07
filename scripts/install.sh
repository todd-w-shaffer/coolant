#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="$HOME/.local/bin"
ARCH="$(uname -m)"
URL="https://github.com/todd-w-shaffer/coolant/releases/latest/download/thermal-darwin-${ARCH}"

mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" -o "$INSTALL_DIR/thermal"
chmod +x "$INSTALL_DIR/thermal"

echo "Installed thermal to $INSTALL_DIR/thermal"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: $INSTALL_DIR is not on your PATH. Add this to ~/.zshrc:"
     echo '  export PATH="$HOME/.local/bin:$PATH"' ;;
esac
