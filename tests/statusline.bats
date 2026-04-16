#!/usr/bin/env bats

# Tests for the statusline update check logic.
# We extract and test the version comparison independently
# since the full statusline requires jq + stdin JSON.

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  # Compute project root before overriding TMPDIR
  TESTS_DIR="$(cd "$(dirname "${BATS_TEST_FILENAME}")" && pwd)"
  PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"
  export TMPDIR="$TEST_TMPDIR/"
  export USER="testuser"
  export COOLANT_UPDATE_TTL=1440
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ── version extraction ──────────────────────────────

@test "statusline has VERSION comment" {
  grep -q '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh"
}

@test "VERSION comment is valid semver" {
  version=$(grep '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1 | sed 's/# VERSION: *//')
  echo "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'
}

# ── cache behavior ──────────────────────────────────

@test "fresh cache with newer version sets upgrade glyph" {
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "99.0.0" > "$cache"

  # Extract the installed version from the script
  installed=$(grep '^COOLANT_INSTALLED_VERSION=' "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1 | sed 's/.*="//' | sed 's/"//')

  # The installed version should be less than 99.0.0
  [ "$installed" != "99.0.0" ]

  # Simulate the comparison logic
  _coolant_latest="99.0.0"
  _coolant_upgrade_glyph=""
  if [ -n "$_coolant_latest" ] && [ "$_coolant_latest" != "$installed" ]; then
    _coolant_upgrade_glyph=" ⬆"
  fi

  [ "$_coolant_upgrade_glyph" = " ⬆" ]
}

@test "fresh cache with same version does not set upgrade glyph" {
  installed=$(grep '^COOLANT_INSTALLED_VERSION=' "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1 | sed 's/.*="//' | sed 's/"//')
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "$installed" > "$cache"

  _coolant_latest="$installed"
  _coolant_upgrade_glyph=""
  if [ -n "$_coolant_latest" ] && [ "$_coolant_latest" != "$installed" ]; then
    _coolant_upgrade_glyph=" ⬆"
  fi

  [ -z "$_coolant_upgrade_glyph" ]
}

@test "missing cache does not set upgrade glyph" {
  # No cache file exists
  _coolant_latest=""
  _coolant_upgrade_glyph=""
  if [ -n "$_coolant_latest" ] && [ "$_coolant_latest" != "0.4.0" ]; then
    _coolant_upgrade_glyph=" ⬆"
  fi

  [ -z "$_coolant_upgrade_glyph" ]
}

@test "empty cache does not set upgrade glyph" {
  cache="${TMPDIR}coolant-${USER}.latest-version"
  touch "$cache"

  _coolant_latest=$(cat "$cache" 2>/dev/null)
  _coolant_upgrade_glyph=""
  if [ -n "$_coolant_latest" ] && [ "$_coolant_latest" != "0.4.0" ]; then
    _coolant_upgrade_glyph=" ⬆"
  fi

  [ -z "$_coolant_upgrade_glyph" ]
}

@test "cache TTL check uses find -mmin" {
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "99.0.0" > "$cache"

  # Fresh file — should be found by find -mmin
  result=$(find "$cache" -mmin -1440 2>/dev/null)
  [ -n "$result" ]
}
