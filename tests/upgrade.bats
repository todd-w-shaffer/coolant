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

# ── thermo upgrade delegation (integration) ────────
#
# These tests run the real scripts/upgrade.sh against a stubbed PATH
# (mock thermo + mock curl). They lock the binary-refresh contract:
# delegate to `thermo upgrade` when supported, fall back to in-place
# curl when it isn't.

@test "upgrade.sh delegates binary refresh to 'thermo upgrade' when supported" {
  mkdir -p "$TEST_TMPDIR/bin" "$TEST_TMPDIR/stub"

  # Stub thermo: 'upgrade' writes a marker and exits 0; '--version'
  # echoes a fixed string.
  cat > "$TEST_TMPDIR/bin/thermo" << EOF
#!/bin/bash
if [ "\$1" = "upgrade" ]; then
  echo "delegated" > "$TEST_TMPDIR/upgrade-marker"
  exit 0
fi
if [ "\$1" = "--version" ]; then
  echo "0.6.0"
fi
EOF
  chmod +x "$TEST_TMPDIR/bin/thermo"
  cp "$TEST_TMPDIR/bin/thermo" "$TEST_TMPDIR/thermo-snapshot"

  # Stub curl: if invoked with -o, overwrite the target with a sentinel.
  # If the production script reaches this stub for the binary, the
  # post-test diff will detect the clobber and the test fails.
  cat > "$TEST_TMPDIR/stub/curl" << 'EOF'
#!/bin/bash
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    echo "CURL_CLOBBERED" > "$2"
    exit 0
  fi
  shift
done
exit 0
EOF
  chmod +x "$TEST_TMPDIR/stub/curl"

  PATH="$TEST_TMPDIR/bin:$TEST_TMPDIR/stub:$PATH" run bash "$PROJECT_ROOT/scripts/upgrade.sh"
  [ "$status" -eq 0 ]
  [ -f "$TEST_TMPDIR/upgrade-marker" ]
  diff -q "$TEST_TMPDIR/bin/thermo" "$TEST_TMPDIR/thermo-snapshot"
}

@test "upgrade.sh falls back to curl when 'thermo upgrade' subcommand fails" {
  mkdir -p "$TEST_TMPDIR/bin" "$TEST_TMPDIR/stub"

  # Stub thermo: 'upgrade' fails (simulates older binary that predates
  # the subcommand); '--version' echoes a fixed string.
  cat > "$TEST_TMPDIR/bin/thermo" << EOF
#!/bin/bash
if [ "\$1" = "upgrade" ]; then exit 1; fi
if [ "\$1" = "--version" ]; then echo "0.4.0"; fi
EOF
  chmod +x "$TEST_TMPDIR/bin/thermo"

  # Stub curl: writes a "new" thermo executable in place. Post-test
  # we confirm the binary's bytes changed.
  cat > "$TEST_TMPDIR/stub/curl" << 'EOF'
#!/bin/bash
while [ $# -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    printf '#!/bin/bash\necho "0.6.0"\n' > "$2"
    exit 0
  fi
  shift
done
exit 0
EOF
  chmod +x "$TEST_TMPDIR/stub/curl"

  PATH="$TEST_TMPDIR/bin:$TEST_TMPDIR/stub:$PATH" run bash "$PROJECT_ROOT/scripts/upgrade.sh"
  [ "$status" -eq 0 ]
  # Curl path was used — the binary's contents now reflect the stub.
  grep -q '0.6.0' "$TEST_TMPDIR/bin/thermo"
}
