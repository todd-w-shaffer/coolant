#!/usr/bin/env bats

# Statusline tests. The per-session statusline surface deliberately
# does NOT signal coolant install staleness — that's a system-wide
# concern owned by thermo's notification banner.

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  TESTS_DIR="$(cd "$(dirname "${BATS_TEST_FILENAME}")" && pwd)"
  PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"
  export TMPDIR="$TEST_TMPDIR/"
  export USER="testuser"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ── version comment (read by scripts/upgrade.sh) ────

@test "statusline has VERSION comment" {
  grep -q '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh"
}

@test "VERSION comment is valid semver" {
  version=$(grep '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1 | sed 's/# VERSION: *//')
  echo "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'
}

# ── coolant-staleness signal belongs in thermo, not statusline ─────

@test "statusline output never shows the upgrade glyph" {
  # System-wide staleness (coolant install version drift) belongs in
  # thermo's notification banner, not the per-session statusline.
  # Even with a cache asserting a much newer remote version exists,
  # the statusline must not emit ⬆.
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "99.0.0" > "$cache"

  input='{"context_window":{"used_percentage":0,"total_input_tokens":0,"total_output_tokens":0},"rate_limits":{"five_hour":{"used_percentage":0,"resets_at":0},"seven_day":{"used_percentage":0}},"cwd":"."}'

  output=$(printf '%s' "$input" | bash "$PROJECT_ROOT/claude-statusline/statusline.sh" 2>&1)
  ! printf '%s' "$output" | grep -q '⬆'
}
