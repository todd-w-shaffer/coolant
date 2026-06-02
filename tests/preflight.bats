#!/usr/bin/env bats

load test_helper

# Helper: run preflight.sh with a SessionStart payload from a given cwd
run_preflight() {
  local cwd="$1"
  printf '{"session_id":"test","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' "$cwd" \
    | bash "$PROJECT_ROOT/scripts/preflight.sh" 2>/dev/null
}

# Helper: run preflight.sh with an explicit SessionStart source
# (startup|resume|clear) and session_id, from a given cwd.
run_preflight_src() {
  local cwd="$1" source="$2" sid="${3:-test}"
  printf '{"session_id":"%s","cwd":"%s","hook_event_name":"SessionStart","source":"%s"}' \
    "$sid" "$cwd" "$source" \
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

@test "preflight emits counter.reset at session start" {
  mkdir -p "$TEST_TMPDIR/project"
  run_preflight "$TEST_TMPDIR/project" > /dev/null
  grep -q '"event":"counter.reset"' "$COOLANT_EVENTS"
}

@test "preflight emits JSONL preflight.warn event" {
  mkdir -p "$TEST_TMPDIR/project"
  cat > "$TEST_TMPDIR/project/vitest.config.ts" <<'EOF'
export default defineConfig({ test: {} })
EOF
  run_preflight "$TEST_TMPDIR/project" > /dev/null
  grep -q '"event":"preflight.warn"' "$COOLANT_EVENTS"
}

@test "preflight writes session_id sidecar before first event" {
  mkdir -p "$TEST_TMPDIR/project"
  printf '{"session_id":"abc-123","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' \
    "$TEST_TMPDIR/project" \
    | bash "$PROJECT_ROOT/scripts/preflight.sh" > /dev/null
  [ -f "$COOLANT_SESSION_FILE" ]
  [ "$(cat "$COOLANT_SESSION_FILE")" = "abc-123" ]
}

@test "SessionStart hook fires on startup, resume, and clear" {
  # preflight writes the session sidecar that thermo's tailer reads to
  # scope agent events. A resumed/cleared session keeps its session_id
  # but, with a startup-only matcher, never re-runs preflight — leaving
  # the sidecar pinned to whatever session last cold-started (often a
  # dead transient). The matcher must fire on all three sources, and the
  # entry must still point at preflight.sh. Asserting each token guards
  # against a future edit silently dropping one (e.g. narrowing back to
  # startup-only, which re-breaks resume/clear agent visibility).
  local entry matcher cmd
  entry=$(jq -c '.hooks.SessionStart[] | select(.hooks[].command | test("preflight"))' \
    "$PROJECT_ROOT/hooks/hooks.json")
  matcher=$(printf '%s' "$entry" | jq -r '.matcher')
  cmd=$(printf '%s' "$entry" | jq -r '.hooks[0].command')
  [[ "$matcher" == *startup* ]]
  [[ "$matcher" == *resume* ]]
  [[ "$matcher" == *clear* ]]
  [[ "$cmd" == *preflight.sh ]]
}

# ── Source-gated side effects (resume/clear must not clobber shared,
#    cross-session state) ───────────────────────────────────────────

@test "preflight does NOT emit counter.reset on resume" {
  # counter.reset is a GLOBAL (non-session-scoped) signal: the Go live
  # model purges ALL active agent records on it, machine-wide. thermo is
  # one-per-machine on a shared JSONL bus, so a resume in one session
  # would wipe another concurrent session's live agents. Only a true
  # cold start should reset the agent epoch.
  mkdir -p "$TEST_TMPDIR/project"
  run_preflight_src "$TEST_TMPDIR/project" resume > /dev/null
  run grep -q '"event":"counter.reset"' "$COOLANT_EVENTS"
  [ "$status" -ne 0 ]
}

@test "preflight does NOT emit counter.reset on clear" {
  mkdir -p "$TEST_TMPDIR/project"
  run_preflight_src "$TEST_TMPDIR/project" clear > /dev/null
  run grep -q '"event":"counter.reset"' "$COOLANT_EVENTS"
  [ "$status" -ne 0 ]
}

@test "preflight preserves the review-audit log on clear" {
  # The review-gate audit log is the per-session ledger the commit gate
  # reads to confirm /simplify + /observations ran. The review→/clear→
  # commit flow is explicitly recommended; truncating on /clear would
  # falsely block the commit. Only cold start truncates.
  mkdir -p "$TEST_TMPDIR/project"
  printf 'prior-review-entry\n' > "$COOLANT_REVIEW_AUDIT"
  run_preflight_src "$TEST_TMPDIR/project" clear > /dev/null
  grep -q 'prior-review-entry' "$COOLANT_REVIEW_AUDIT"
}

@test "preflight still re-stamps the session sidecar on resume" {
  mkdir -p "$TEST_TMPDIR/project"
  run_preflight_src "$TEST_TMPDIR/project" resume "resumed-sid" > /dev/null
  [ "$(cat "$COOLANT_SESSION_FILE")" = "resumed-sid" ]
}

@test "preflight truncates the review-audit log on startup" {
  mkdir -p "$TEST_TMPDIR/project"
  printf 'stale-entry\n' > "$COOLANT_REVIEW_AUDIT"
  run_preflight "$TEST_TMPDIR/project" > /dev/null
  [ "$(wc -l < "$COOLANT_REVIEW_AUDIT")" -eq 0 ]
}

@test "preflight truncates COOLANT_DEGRADED_COUNT" {
  mkdir -p "$TEST_TMPDIR/project"
  # Pre-seed a stale counter from a prior session.
  printf '\n\n\n\n\n' > "$COOLANT_DEGRADED_COUNT"
  [ "$(wc -l < "$COOLANT_DEGRADED_COUNT")" -eq 5 ]

  run_preflight "$TEST_TMPDIR/project" > /dev/null

  # File still exists but is empty — cumulative-since-install bug fixed.
  [ -f "$COOLANT_DEGRADED_COUNT" ]
  [ "$(wc -l < "$COOLANT_DEGRADED_COUNT")" -eq 0 ]
}
