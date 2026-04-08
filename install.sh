#!/bin/bash
set -euo pipefail

# coolant installer
# Downloads the thermo dashboard binary and statusline.

REPO="todd-w-shaffer/coolant"
BINARY="thermo"
RAW="https://raw.githubusercontent.com/${REPO}/main"

# Colors
DIM='\033[2m'
CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
BOLD='\033[1m'
RESET='\033[0m'

# Braille progress: sparse → mid → dense
BAR1="⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂"
BAR2="⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒⠒"
BAR3="⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿"

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
  echo "  the thermo dashboard is macOS only."
  exit 1
fi

ASSET="${BINARY}-darwin-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "  detected: macOS $ARCH"

# ── step 1 · dashboard ──────────────────────────────────
echo ""
printf "  ${CYAN}${BAR1}${RESET}\n"
printf "  ${BOLD}1/3${RESET} ${DIM}dashboard${RESET}\n"
echo ""

DEFAULT_DIR="$HOME/.local/bin"
if [ -w "/usr/local/bin" ]; then
  DEFAULT_DIR="/usr/local/bin"
fi

echo "  where should thermo live?"
printf "  press Enter for %s (recommended), or type a path: " "$DEFAULT_DIR"
read -r INSTALL_DIR < /dev/tty
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

printf "  ${GREEN}✓${RESET} installed to $DEST\n"

# ── step 2 · path ───────────────────────────────────────
ON_PATH=true
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  ON_PATH=false

  echo ""
  printf "  ${YELLOW}${BAR2}${RESET}\n"
  printf "  ${BOLD}2/3${RESET} ${DIM}path${RESET}\n"
  echo ""

  # Detect shell profile
  SHELL_NAME="$(basename "$SHELL")"
  case "$SHELL_NAME" in
    zsh)  PROFILE="$HOME/.zshrc" ;;
    bash) PROFILE="$HOME/.bashrc" ;;
    *)    PROFILE="$HOME/.profile" ;;
  esac

  EXPORT_LINE="export PATH=\"$INSTALL_DIR:\$PATH\""

  echo "  want to run thermo from anywhere in your terminal?"
  printf "  (adds %s to PATH in %s) [Y/n]: " "$INSTALL_DIR" "$PROFILE"
  read -r ADD_PATH < /dev/tty
  ADD_PATH="${ADD_PATH:-Y}"

  if [[ "$ADD_PATH" =~ ^[Yy]$ ]]; then
    echo "" >> "$PROFILE"
    echo "# coolant" >> "$PROFILE"
    echo "$EXPORT_LINE" >> "$PROFILE"
    export PATH="$INSTALL_DIR:$PATH"
    ON_PATH=true
    printf "  ${GREEN}✓${RESET} added to %s and activated for this session.\n" "$PROFILE"
  else
    echo ""
    echo "  no worries. to add it later, run:"
    echo ""
    echo "    echo '$EXPORT_LINE' >> $PROFILE && source $PROFILE"
  fi
fi

# ── step 3 · statusline ─────────────────────────────────
echo ""
printf "  ${GREEN}${BAR3}${RESET}\n"
printf "  ${BOLD}3/3${RESET} ${DIM}statusline${RESET}\n"
echo ""

echo "  coolant includes a statusline for Claude Code — context usage,"
echo "  session usage, weekly quota, plan refresh timer, and git branch."
printf "  install it? [Y/n]: "
read -r INSTALL_SL < /dev/tty
INSTALL_SL="${INSTALL_SL:-Y}"

if [[ "$INSTALL_SL" =~ ^[Yy]$ ]]; then
  CLAUDE_DIR="$HOME/.claude"
  SL_DEST="${CLAUDE_DIR}/statusline.sh"
  SETTINGS="${CLAUDE_DIR}/settings.json"

  mkdir -p "$CLAUDE_DIR"
  echo "  downloading statusline..."
  curl -fsSL "${RAW}/claude-statusline/statusline.sh" -o "$SL_DEST"

  if [ -f "$SETTINGS" ]; then
    if grep -q '"statusLine"' "$SETTINGS"; then
      printf "  ${GREEN}✓${RESET} statusline already configured, skipping.\n"
    else
      if command -v jq &>/dev/null; then
        tmp=$(mktemp)
        jq '.statusLine = {"type": "command", "command": "bash ~/.claude/statusline.sh"}' "$SETTINGS" > "$tmp" && mv "$tmp" "$SETTINGS"
        printf "  ${GREEN}✓${RESET} statusline added to Claude Code settings.\n"
      else
        echo ""
        echo "  almost there — jq isn't installed, so you'll need to add this"
        echo "  to $SETTINGS by hand. open the file and add:"
        echo ""
        echo '    "statusLine": {'
        echo '      "type": "command",'
        echo '      "command": "bash ~/.claude/statusline.sh"'
        echo '    }'
        echo ""
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
    printf "  ${GREEN}✓${RESET} statusline configured.\n"
  fi

  echo "  restart Claude Code to see it."
fi

# Final output — use full path if not on PATH
if [ "$ON_PATH" = true ]; then
  RUN="thermo"
else
  RUN="$DEST"
fi

echo ""
printf "  ${GREEN}${BAR3}${RESET}\n"
echo ""
printf "    ${GREEN}${BOLD}✓ coolant installed${RESET}\n"
echo ""
echo "    $RUN --demo      see the dashboard"
echo "    $RUN             monitor your system"
echo ""
