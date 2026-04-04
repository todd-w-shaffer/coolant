#!/usr/bin/env bats

load test_helper

# Helper: run preflight.sh with a SessionStart payload from a given cwd
run_preflight() {
  local cwd="$1"
  printf '{"session_id":"test","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' "$cwd" \
    | bash "$PROJECT_ROOT/scripts/preflight.sh" 2>/dev/null
}

# ── Worktree exclusion detection ─────────────────────────

@test "preflight warns when vitest.config.ts missing .claude exclude" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.ts" <<'EOF'
export default defineConfig({
  test: {
    poolOptions: {}
  }
})
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project")
  [[ "$out" == *".claude"* ]]
  [[ "$out" == *"vitest"* ]]
}

@test "preflight silent when vitest.config.ts has .claude exclude" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.ts" <<'EOF'
export default defineConfig({
  test: {
    exclude: ['**/.claude/**']
  }
})
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project") || true
  [ -z "$out" ]
}

@test "preflight warns for jest.config.js missing .claude exclude" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/jest.config.js" <<'EOF'
module.exports = {
  testMatch: ['**/*.test.js']
}
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project")
  [[ "$out" == *".claude"* ]]
  [[ "$out" == *"jest"* ]]
}

@test "preflight silent when no test config exists" {
  mkdir -p "$TEST_TMPDIR/project"
  local out
  out=$(run_preflight "$TEST_TMPDIR/project") || true
  [ -z "$out" ]
}

@test "preflight warns for vitest.config.js too" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.js" <<'EOF'
export default defineConfig({ test: {} })
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project")
  [[ "$out" == *"vitest"* ]]
}

@test "preflight silent when vitest.config.ts has .claude in testPathIgnorePatterns" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.ts" <<'EOF'
export default defineConfig({
  test: {
    exclude: ['node_modules', '.claude']
  }
})
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project") || true
  [ -z "$out" ]
}

@test "preflight warns for jest.config.ts too" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/jest.config.ts" <<'EOF'
export default { testMatch: ['**/*.test.ts'] }
EOF
  local out
  out=$(run_preflight "$TEST_TMPDIR/project")
  [[ "$out" == *"jest"* ]]
}

@test "preflight emits JSONL preflight.warn event" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.ts" <<'EOF'
export default defineConfig({ test: {} })
EOF
  run_preflight "$TEST_TMPDIR/project" > /dev/null
  grep -q '"event":"preflight.warn"' "$COOLANT_EVENTS"
}
