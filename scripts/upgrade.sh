#!/bin/bash
set -euo pipefail

# coolant upgrade — re-fetches thermo binary and statusline.
# No prompts. Prints a summary of what changed.

REPO="todd-w-shaffer/coolant"
RAW="https://raw.githubusercontent.com/${REPO}/main"

GREEN='\033[32m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

# ── locate thermo ──────────────────────────────────
THERMO_BIN=""
if command -v thermo >/dev/null 2>&1; then
  THERMO_BIN="$(command -v thermo)"
else
  for candidate in "$HOME/.local/bin/thermo" "/usr/local/bin/thermo"; do
    if [ -x "$candidate" ]; then
      THERMO_BIN="$candidate"
      break
    fi
  done
fi

# ── capture old versions ───────────────────────────
OLD_THERMO_VERSION=""
if [ -n "$THERMO_BIN" ]; then
  OLD_THERMO_VERSION=$("$THERMO_BIN" --version 2>/dev/null || echo "unknown")
fi

SL_PATH="$HOME/.claude/statusline.sh"
OLD_SL_VERSION="unknown"
if [ -f "$SL_PATH" ]; then
  OLD_SL_VERSION=$(grep '^# VERSION:' "$SL_PATH" 2>/dev/null | head -1 | sed 's/# VERSION: *//' || echo "unknown")
  if [ -z "$OLD_SL_VERSION" ]; then
    OLD_SL_VERSION="unknown"
  fi
fi

# ── detect architecture ────────────────────────────
ARCH="$(uname -m)"
case "$ARCH" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64)        ARCH="amd64" ;;
  *)
    printf "  unsupported architecture: %s\n" "$ARCH"
    exit 1
    ;;
esac

OS="$(uname -s)"
if [ "$OS" != "Darwin" ]; then
  printf "  unsupported OS: %s\n" "$OS"
  exit 1
fi

# ── upgrade thermo ─────────────────────────────────
NEW_THERMO_VERSION=""
if [ -n "$THERMO_BIN" ]; then
  ASSET="thermo-darwin-${ARCH}"
  URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
  if curl -fsSL "$URL" -o "$THERMO_BIN" 2>/dev/null; then
    chmod +x "$THERMO_BIN"
    NEW_THERMO_VERSION=$("$THERMO_BIN" --version 2>/dev/null || echo "unknown")
  else
    NEW_THERMO_VERSION="$OLD_THERMO_VERSION"
  fi
fi

# ── upgrade statusline ─────────────────────────────
NEW_SL_VERSION=""
if [ -f "$SL_PATH" ]; then
  if curl -fsSL "${RAW}/claude-statusline/statusline.sh" -o "$SL_PATH" 2>/dev/null; then
    NEW_SL_VERSION=$(grep '^# VERSION:' "$SL_PATH" 2>/dev/null | head -1 | sed 's/# VERSION: *//' || echo "unknown")
    if [ -z "$NEW_SL_VERSION" ]; then
      NEW_SL_VERSION="unknown"
    fi
  else
    NEW_SL_VERSION="$OLD_SL_VERSION"
  fi
fi

# ── invalidate version cache ───────────────────────
rm -f "${TMPDIR:-/tmp/}coolant-${USER}.latest-version"

# ── summary ────────────────────────────────────────
echo ""
printf "  ${BOLD}coolant upgraded${RESET}\n"
echo ""

if [ -n "$THERMO_BIN" ]; then
  if [ "$OLD_THERMO_VERSION" = "$NEW_THERMO_VERSION" ]; then
    printf "  ${GREEN}✓${RESET} thermo: %s (already current)\n" "$NEW_THERMO_VERSION"
  else
    printf "  ${GREEN}✓${RESET} thermo: %s → %s\n" "$OLD_THERMO_VERSION" "$NEW_THERMO_VERSION"
  fi
else
  printf "  ${DIM}–${RESET} thermo not installed, skipping\n"
fi

if [ -f "$SL_PATH" ]; then
  if [ "$OLD_SL_VERSION" = "$NEW_SL_VERSION" ]; then
    printf "  ${GREEN}✓${RESET} statusline: %s (already current)\n" "$NEW_SL_VERSION"
  else
    printf "  ${GREEN}✓${RESET} statusline: %s → %s\n" "$OLD_SL_VERSION" "$NEW_SL_VERSION"
  fi
else
  printf "  ${DIM}–${RESET} statusline not installed, skipping\n"
fi

echo ""
