#!/usr/bin/env bats

load test_helper

# Helper: run gate.sh with a PreToolUse payload, capture stdout
run_gate() {
  make_pre_tool_use "$1" "$2" | bash "$PROJECT_ROOT/scripts/gate.sh" 2>/dev/null
}

# ── Command extraction ──────────────────────────────────────

@test "gate ignores non-Bash tool_name" {
  local out
  out=$(run_gate Edit "something") || true
  [ -z "$out" ]
}

@test "gate extracts command from PreToolUse stdin" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc --noEmit")
  echo "$out" | grep -q "tsc"
}

# ── Pattern matching: TypeScript/Node ───────────────────────

@test "gate recognizes tsc" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc")
  [[ "$out" == *"deny"* ]]
}

@test "gate caps vitest (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "3" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate caps jest (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "jest")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes eslint" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "eslint src/")
  [[ "$out" == *"deny"* ]]
}

# ── Pattern matching: Rust ──────────────────────────────────

@test "gate recognizes cargo build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "cargo build")
  [[ "$out" == *"deny"* ]]
}

@test "gate caps cargo test (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "cargo test")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes cargo clippy" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "cargo clippy")
  [[ "$out" == *"deny"* ]]
}

@test "gate allows cargo without gated subcommand" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "cargo fmt") || true
  [[ "$out" != *"deny"* ]]
}

# ── Pattern matching: Go ────────────────────────────────────

@test "gate caps go test (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "go test ./...")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes go build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "go build ./cmd/thermal/")
  [[ "$out" == *"deny"* ]]
}

@test "gate recognizes go vet" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "go vet ./...")
  [[ "$out" == *"deny"* ]]
}

# ── Pattern matching: Python ────────────────────────────────

@test "gate caps pytest (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "pytest")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes mypy" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "mypy src/")
  [[ "$out" == *"deny"* ]]
}

# ── Pattern matching: Java ──────────────────────────────────

@test "gate recognizes gradle" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "gradle build")
  [[ "$out" == *"deny"* ]]
}

@test "gate recognizes mvn" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "mvn compile")
  [[ "$out" == *"deny"* ]]
}

# ── Pattern matching: vite edge case ────────────────────────

@test "gate suppresses vite build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "vite build")
  [[ "$out" == *"deny"* ]]
}

@test "gate allows vite dev" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "vite dev") || true
  [[ "$out" != *"deny"* ]]
}

@test "gate allows bare vite" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "vite") || true
  [[ "$out" != *"deny"* ]]
}

# ── Allow/deny behavior ────────────────────────────────────

@test "gate allows unrecognized command" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "ls -la") || true
  [ -z "$out" ]
}

@test "gate allows gated command when no lockfile" {
  local out
  out=$(run_gate Bash "tsc") || true
  [ -z "$out" ]
}

@test "gate returns deny JSON when suppressing" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc")
  echo "$out" | grep -q '"permissionDecision":"deny"'
}

@test "gate emits JSONL gate.suppress event when suppressing" {
  touch "$COOLANT_LOCKFILE"
  run_gate Bash "tsc" > /dev/null
  grep -q '"event":"gate.suppress"' "$COOLANT_EVENTS"
}

@test "gate handles command with flags" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc --noEmit --strict")
  [[ "$out" == *"deny"* ]]
}

# ── Wrapper prefix handling ─────────────────────────────────

@test "gate recognizes npx tsc" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "npx tsc --noEmit")
  [[ "$out" == *"deny"* ]]
}

@test "gate caps env vitest (wrapper + cap)" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "env vitest run")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"maxConcurrency"* ]]
}

@test "gate recognizes command cargo build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "command cargo build")
  [[ "$out" == *"deny"* ]]
}

@test "gate strips path prefix from binary" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "/usr/local/bin/tsc")
  [[ "$out" == *"deny"* ]]
}

# ── Concurrency capping: cap value via vitest output ──────

@test "gate computes cap 8 for 1 agent on 10 cores" {
  echo "1" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency 8"* ]]
}

@test "gate computes cap 2 for 3 agents on 10 cores" {
  echo "3" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency 2"* ]]
}

@test "gate computes cap 1 minimum for 100 agents" {
  echo "100" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency 1"* ]]
}

@test "gate defaults to cap 8 when counter missing" {
  rm -f "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency 8"* ]]
}

@test "gate defaults to cap 8 when counter is 0" {
  echo "0" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency 8"* ]]
}

# ── Concurrency capping: per-ecosystem flags ──────────────

@test "gate caps vitest even in parallel mode" {
  touch "$COOLANT_LOCKFILE"
  echo "3" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"--maxConcurrency 2"* ]]
}

@test "gate caps jest with --maxWorkers" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "jest --verbose")
  [[ "$out" == *"--maxWorkers 4"* ]]
}

@test "gate caps cargo test with -j" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "cargo test")
  [[ "$out" == *"-j 4"* ]]
}

@test "gate caps go test with -parallel" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "go test ./...")
  [[ "$out" == *"-parallel 4"* ]]
}

@test "gate caps pytest with -n" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "pytest tests/")
  [[ "$out" == *"-n 4"* ]]
}

# ── Concurrency capping: output format ────────────────────

@test "gate returns allow with updatedInput JSON when capping" {
  echo "1" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  echo "$out" | grep -q '"permissionDecision":"allow"'
  echo "$out" | grep -q '"updatedInput"'
}

@test "gate emits gate.cap JSONL event when capping" {
  echo "1" > "$COOLANT_COUNTER"
  run_gate Bash "vitest run" > /dev/null
  grep -q '"event":"gate.cap"' "$COOLANT_EVENTS"
}

# ── Concurrency capping: edge cases ──────────────────────

@test "gate doesn't double-add --maxConcurrency if already present" {
  echo "3" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run --maxConcurrency 1") || true
  [[ "$out" != *"--maxConcurrency 2"* ]]
}

@test "gate caps npx vitest (wrapper + cap)" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "npx vitest run")
  [[ "$out" == *"--maxConcurrency 4"* ]]
}

@test "gate inserts cap flag before a pipe, not on the downstream command" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run | tail -5")
  [[ "$out" == *"vitest run --maxConcurrency 4 | tail -5"* ]]
}

@test "gate inserts cap flag before an && chain" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run && echo done")
  [[ "$out" == *"vitest run --maxConcurrency 4 && echo done"* ]]
}

@test "gate inserts cap flag before an output redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run > out.txt")
  [[ "$out" == *"vitest run --maxConcurrency 4 > out.txt"* ]]
}

@test "gate inserts cap flag before a semicolon" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run; echo done")
  [[ "$out" == *"vitest run --maxConcurrency 4; echo done"* ]]
}

@test "gate ignores a pipe character inside a quoted argument" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run -t 'a|b'")
  [[ "$out" == *"vitest run -t 'a|b' --maxConcurrency 4"* ]]
}

# ── Shell shapes the scanner must get right ────────────────

@test "gate inserts cap flag before an fd-prefixed redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run test/a.test.ts 2>&1 | tail -4")
  [[ "$out" == *"vitest run test/a.test.ts --maxConcurrency 4 2>&1 | tail -4"* ]]
}

@test "gate inserts go test -parallel before an fd-prefixed redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "go test ./... 2>&1 | tail -20")
  [[ "$out" == *"go test ./... -parallel 4 2>&1 | tail -20"* ]]
}

@test "gate keeps a trailing digit attached to its word before a redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run f1>out.txt")
  [[ "$out" == *"vitest run f1 --maxConcurrency 4 >out.txt"* ]]
}

@test "gate separates the cap value from an abutting redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run>out.txt")
  [[ "$out" == *"vitest run --maxConcurrency 4 >out.txt"* ]]
}

@test "gate keeps a space-separated digit argument before a redirect" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run 2 > out.txt")
  [[ "$out" == *"vitest run 2 --maxConcurrency 4 > out.txt"* ]]
}

@test "gate refuses to cap a command containing command substitution" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run \$(ls src | head -1)" || true)
  [ -z "$out" ]
}

@test "gate caps when the flag appears only on a downstream command" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "pytest tests/ | grep -n foo")
  [[ "$out" == *"pytest tests/ -n 4 | grep -n foo"* ]]
}

@test "gate preserves the npx wrapper in the rewritten command" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "npx vitest run f.test.ts | tail -15")
  [[ "$out" == *"npx vitest run f.test.ts --maxConcurrency 4 | tail -15"* ]]
}

# ── Wrapper and fidelity guards ────────────────────────────

# "${rest#* }" is a no-op once rest has no space, so the flag-skip loop
# spun forever and hung the PreToolUse hook on any bare-flag wrapper call.
@test "gate terminates on a wrapper invoked with only a flag" {
  local rc=0
  make_pre_tool_use Bash "sudo -v" \
    | perl -e 'alarm 5; exec @ARGV' bash "$PROJECT_ROOT/scripts/gate.sh" >/dev/null 2>&1 || rc=$?
  [ "$rc" -ne 142 ]
}

@test "gate terminates on npx invoked with only a flag" {
  local rc=0
  make_pre_tool_use Bash "npx -y" \
    | perl -e 'alarm 5; exec @ARGV' bash "$PROJECT_ROOT/scripts/gate.sh" >/dev/null 2>&1 || rc=$?
  [ "$rc" -ne 142 ]
}

# _nested_command extracts the raw JSON string without unescaping and
# emit_cap re-escapes it, so an interior backslash comes back doubled and
# the selector it belonged to no longer matches anything.
@test "gate refuses to cap a command containing an interior backslash" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "pytest -k 'a\\.b' tests/" || true)
  [ -z "$out" ]
}

@test "gate reports the wrapper-prefixed command when suppressing" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "npx tsc --noEmit")
  [[ "$out" == *"blocked: npx tsc --noEmit"* ]]
}

@test "gate inserts cargo test -j before -- separator" {
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "cargo test -- specific_test")
  [[ "$out" == *"-j 4"* ]]
  [[ "$out" == *"-- specific_test"* ]]
}

# ── Suppress targets unchanged ────────────────────────────

@test "gate handles non-numeric counter gracefully" {
  echo "abc" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"--maxConcurrency"* ]]
}

@test "gate still suppresses tsc in parallel mode" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc --noEmit")
  [[ "$out" == *"deny"* ]]
}

@test "gate still allows tsc outside parallel mode" {
  local out
  out=$(run_gate Bash "tsc --noEmit") || true
  [ -z "$out" ]
}

# ── Pattern matching: Swift ────────────────────────────────

@test "gate caps swift test (not deny)" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "swift test")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes swift build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "swift build")
  [[ "$out" == *"deny"* ]]
}

@test "gate recognizes xcodebuild build" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "xcodebuild build")
  [[ "$out" == *"deny"* ]]
}

@test "gate recognizes xcodebuild test with cap" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  local out
  out=$(run_gate Bash "xcodebuild test")
  [[ "$out" == *"allow"* ]]
  [[ "$out" == *"updatedInput"* ]]
}

@test "gate recognizes swiftlint" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "swiftlint")
  [[ "$out" == *"deny"* ]]
}

@test "gate allows swift without gated subcommand" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "swift package resolve") || true
  [[ "$out" != *"deny"* ]]
}

# ── JSONL reconciliation ─────────────────────────────────────

@test "gate reconciles stale counter for cap calculation" {
  echo "0" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "vitest run")
  # JSONL-derived count is 2: cap = (10-2)/2 = 4
  [[ "$out" == *"--maxConcurrency 4"* ]]
}

@test "gate reconciles inflated counter from orphaned agents" {
  echo "10" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  local out
  out=$(run_gate Bash "vitest run")
  # JSONL-derived count is 1: cap = (10-2)/1 = 8
  [[ "$out" == *"--maxConcurrency 8"* ]]
}
