#!/bin/bash
set -euo pipefail

# coolant installer
# Downloads the thermal dashboard binary and statusline.

REPO="todd-w-shaffer/coolant"
BINARY="thermo"
RAW="https://raw.githubusercontent.com/${REPO}/main"

cat <<'BANNER'

     ___  ___  ___  _    ___  _  _  _____
    / __|/ _ \/ _ \| |  /   \| \| ||_   _|
   | (__| (_)| (_) | |__| - ||    |  | |
    \___|\___/\___/|____|_|_||_|\_|  |_|

    thermal management for claude code

BANNER

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

# Expand ~ and $HOME if someone types them
INSTALL_DIR="${INSTALL_DIR/#\~/$HOME}"
INSTALL_DIR="${INSTALL_DIR//\$HOME/$HOME}"

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

# Statusline
echo ""
printf "  install the braille statusline for Claude Code? [Y/n]: "
read -r INSTALL_SL
INSTALL_SL="${INSTALL_SL:-Y}"

if [[ "$INSTALL_SL" =~ ^[Yy]$ ]]; then
  CLAUDE_DIR="$HOME/.claude"
  SL_DEST="${CLAUDE_DIR}/statusline.sh"
  SETTINGS="${CLAUDE_DIR}/settings.json"

  mkdir -p "$CLAUDE_DIR"
  echo "  downloading statusline.sh..."
  curl -fsSL "${RAW}/claude-statusline/statusline.sh" -o "$SL_DEST"

  if [ -f "$SETTINGS" ]; then
    if grep -q '"statusLine"' "$SETTINGS"; then
      echo "  statusLine already configured in settings.json, skipping."
    else
      printf "  add statusLine to %s? [Y/n]: " "$SETTINGS"
      read -r PATCH_SETTINGS
      PATCH_SETTINGS="${PATCH_SETTINGS:-Y}"
      if [[ "$PATCH_SETTINGS" =~ ^[Yy]$ ]]; then
        if command -v jq &>/dev/null; then
          tmp=$(mktemp)
          jq '.statusLine = {"type": "command", "command": "bash ~/.claude/statusline.sh"}' "$SETTINGS" > "$tmp" && mv "$tmp" "$SETTINGS"
          echo "  statusLine added to settings.json."
        else
          echo "  jq not found. add this to $SETTINGS manually:"
          echo ""
          echo '    "statusLine": {'
          echo '      "type": "command",'
          echo '      "command": "bash ~/.claude/statusline.sh"'
          echo '    }'
          echo ""
        fi
      fi
    fi
  else
    cat > "$SETTINGS" <<'SETTINGS_EOF'
{
  "statusLine": {
    "type": "command",
    "command": "bash ~/.claude/statusline.sh"
  }
}
SETTINGS_EOF
    echo "  created settings.json with statusLine."
  fi

  echo "  statusline installed. restart Claude Code to activate."
fi

cat <<'DONE'

    ✓ coolant installed

    thermo --demo      see the dashboard
    thermo             monitor your system

DONE
