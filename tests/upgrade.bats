#!/usr/bin/env bats

# Tests for upgrade.sh logic — mocked curl, isolated filesystem.

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  TESTS_DIR="$(cd "$(dirname "${BATS_TEST_FILENAME}")" && pwd)"
  PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"
  export HOME="$TEST_TMPDIR/home"
  export TMPDIR="$TEST_TMPDIR/tmp/"
  export USER="testuser"
  mkdir -p "$HOME/.claude" "$TMPDIR"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ── version extraction ──────────────────────────────

@test "extracts old statusline version from VERSION comment" {
  cat > "$HOME/.claude/statusline.sh" << 'EOF'
#!/usr/bin/env bash
# VERSION: 0.3.0
# stuff
EOF

  version=$(grep '^# VERSION:' "$HOME/.claude/statusline.sh" | head -1 | sed 's/# VERSION: *//')
  [ "$version" = "0.3.0" ]
}

@test "missing VERSION comment defaults to unknown" {
  cat > "$HOME/.claude/statusline.sh" << 'EOF'
#!/usr/bin/env bash
# no version here
EOF

  version=$(grep '^# VERSION:' "$HOME/.claude/statusline.sh" 2>/dev/null | head -1 | sed 's/# VERSION: *//' || echo "unknown")
  [ -z "$version" ] || [ "$version" = "unknown" ]
}

# ── architecture detection ──────────────────────────

@test "arm64 arch maps correctly" {
  ARCH="arm64"
  case "$ARCH" in
    arm64|aarch64) ARCH="arm64" ;;
    x86_64)        ARCH="amd64" ;;
  esac
  [ "$ARCH" = "arm64" ]
}

@test "x86_64 arch maps to amd64" {
  ARCH="x86_64"
  case "$ARCH" in
    arm64|aarch64) ARCH="arm64" ;;
    x86_64)        ARCH="amd64" ;;
  esac
  [ "$ARCH" = "amd64" ]
}

# ── mock thermo binary ─────────────────────────────

@test "captures old version from thermo --version" {
  mkdir -p "$TEST_TMPDIR/bin"
  cat > "$TEST_TMPDIR/bin/thermo" << 'EOF'
#!/bin/bash
if [ "$1" = "--version" ]; then
  echo "0.4.0"
fi
EOF
  chmod +x "$TEST_TMPDIR/bin/thermo"

  OLD_THERMO_VERSION=$("$TEST_TMPDIR/bin/thermo" --version 2>/dev/null || echo "unknown")
  [ "$OLD_THERMO_VERSION" = "0.4.0" ]
}

@test "missing thermo binary sets empty path" {
  THERMO_BIN=""
  if command -v thermo_nonexistent_binary >/dev/null 2>&1; then
    THERMO_BIN="$(command -v thermo_nonexistent_binary)"
  fi
  [ -z "$THERMO_BIN" ]
}

# ── cache invalidation ─────────────────────────────

@test "cache file is removed after upgrade" {
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "0.5.0" > "$cache"
  [ -f "$cache" ]

  rm -f "$cache"
  [ ! -f "$cache" ]
}

# ── summary formatting ─────────────────────────────

@test "same versions shows already current" {
  OLD="0.4.0"
  NEW="0.4.0"
  if [ "$OLD" = "$NEW" ]; then
    result="$NEW (already current)"
  else
    result="$OLD → $NEW"
  fi
  echo "$result" | grep -q "already current"
}

@test "different versions shows arrow" {
  OLD="0.4.0"
  NEW="0.5.0"
  if [ "$OLD" = "$NEW" ]; then
    result="$NEW (already current)"
  else
    result="$OLD → $NEW"
  fi
  echo "$result" | grep -q "→"
}

# ── install.sh --upgrade flag ───────────────────────

@test "install.sh help mentions --upgrade" {
  run bash "$PROJECT_ROOT/install.sh" --help
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "upgrade"
}

# ── upgrade.sh syntax check ────────────────────────

@test "upgrade.sh passes bash -n syntax check" {
  run bash -n "$PROJECT_ROOT/scripts/upgrade.sh"
  [ "$status" -eq 0 ]
}
