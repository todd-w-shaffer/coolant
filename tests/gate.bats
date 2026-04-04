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

@test "gate recognizes vitest" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "vitest run")
  [[ "$out" == *"deny"* ]]
}

@test "gate recognizes jest" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "jest")
  [[ "$out" == *"deny"* ]]
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

@test "gate recognizes cargo test" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "cargo test")
  [[ "$out" == *"deny"* ]]
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

@test "gate recognizes go test" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "go test ./...")
  [[ "$out" == *"deny"* ]]
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

@test "gate recognizes pytest" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "pytest")
  [[ "$out" == *"deny"* ]]
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

@test "gate recognizes env vitest" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "env vitest run")
  [[ "$out" == *"deny"* ]]
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
