#!/bin/bash
set -euo pipefail

# coolant installer
# Downloads the thermo dashboard binary and statusline.

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  echo "Usage: bash install.sh [--uninstall] [--upgrade]"
  echo ""
  echo "Installs the thermo dashboard binary and optional Claude Code statusline."
  echo "Run via: curl -fsSL https://raw.githubusercontent.com/todd-w-shaffer/coolant/main/install.sh | bash"
  echo ""
  echo "  --uninstall    remove thermo binary, statusline, and PATH entry"
  echo "  --upgrade      re-fetch thermo binary and statusline to latest"
  exit 0
fi

if [ "${1:-}" = "--upgrade" ]; then
  exec bash <(curl -fsSL "https://raw.githubusercontent.com/todd-w-shaffer/coolant/main/scripts/upgrade.sh")
fi

if [ "${1:-}" = "--uninstall" ]; then
  BINARY="thermo"

  # Colors
  RED='\033[31m'
  GREEN='\033[32m'
  DIM='\033[2m'
  BOLD='\033[1m'
  RESET='\033[0m'

  echo ""
  printf "  ${BOLD}uninstalling coolant${RESET}\n"
  echo ""

  # Find and remove binary
  FOUND_BIN="$(command -v "$BINARY" 2>/dev/null || true)"
  if [ -n "$FOUND_BIN" ]; then
    rm "$FOUND_BIN"
    printf "  ${GREEN}✓${RESET} removed %s\n" "$FOUND_BIN"
  else
    printf "  ${DIM}–${RESET} thermo binary not found on PATH, skipping\n"
  fi

  # Remove statusline
  SL="$HOME/.claude/statusline.sh"
  if [ -f "$SL" ]; then
    rm "$SL"
    printf "  ${GREEN}✓${RESET} removed %s\n" "$SL"
  else
    printf "  ${DIM}–${RESET} statusline not found, skipping\n"
  fi

  # Remove statusLine from settings.json
  SETTINGS="$HOME/.claude/settings.json"
  if [ -f "$SETTINGS" ] && grep -q '"statusLine"' "$SETTINGS"; then
    if command -v jq &>/dev/null; then
      tmp=$(mktemp)
      jq 'del(.statusLine)' "$SETTINGS" > "$tmp" && mv "$tmp" "$SETTINGS"
      printf "  ${GREEN}✓${RESET} removed statusLine from settings.json\n"
    else
      printf "  ${RED}!${RESET} statusLine entry in settings.json needs manual removal (jq not installed)\n"
    fi
  fi

  # Remove PATH entry from shell profile
  SHELL_NAME="$(basename "$SHELL")"
  case "$SHELL_NAME" in
    zsh)  PROFILE="$HOME/.zshrc" ;;
    bash) PROFILE="$HOME/.bashrc" ;;
    *)    PROFILE="$HOME/.profile" ;;
  esac

  if [ -f "$PROFILE" ] && grep -q '# coolant' "$PROFILE"; then
    # Remove the comment and the export line that follows it
    sed -i '' '/^# coolant$/,+1d' "$PROFILE"
    # Remove any blank line left behind
    sed -i '' '/^$/N;/^\n$/d' "$PROFILE"
    printf "  ${GREEN}✓${RESET} removed PATH entry from %s\n" "$PROFILE"
  fi

  echo ""
  printf "  ${BOLD}✓ coolant uninstalled${RESET}\n"
  echo ""
  exit 0
fi

REPO="todd-w-shaffer/coolant"
BINARY="thermo"
RAW="https://raw.githubusercontent.com/${REPO}/main"

# Read from terminal even when piped via curl | bash
prompt() { read -r "$1" < /dev/tty; }

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

echo ""
printf "${GREEN}"
echo '   ___   ___   ___   _      _    _  _  _____'
echo '  / __| / _ \ / _ \ | |    / \  | \| ||_   _|'
echo ' | (__  | (_) | (_) | |__ / _ \ |    |  | |'
echo '  \___| \___/ \___/ |____/_/ \_\|_|\_|  |_|'
printf "${RESET}"
echo ""
echo "    thermal management for claude code"
echo ""
printf "    ${DIM}built with Go + Bubbletea · looks great full-width in Ghostty${RESET}\n"
printf "    ${DIM}pairs well with Catppuccin Frappé and FiraCode Nerd Font Mono${RESET}\n"
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
prompt INSTALL_DIR
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
  prompt ADD_PATH
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
prompt INSTALL_SL
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
