#!/usr/bin/env bats
# gate.sh routes typed heavy commands into the work queue's door
# (`coolant run --class <c> -- <command>`). It never caps and never
# denies; a command it cannot parse safely is left alone.

load test_helper

# Helper: run gate.sh with a PreToolUse payload, capture stdout
run_gate() {
  make_pre_tool_use "$1" "$2" | bash "$PROJECT_ROOT/scripts/gate.sh" 2>/dev/null
}

# The rewritten command as Claude Code will run it (JSON-unescaped).
routed() {
  run_gate Bash "$1" | sed -n 's/.*"updatedInput":{"command":"\(.*\)"}}}$/\1/p' | sed 's/\\"/"/g'
}

DOOR_PREFIX() { printf '%s run --class %s -- ' "$PROJECT_ROOT/scripts/coolant" "$1"; }

# ── Command extraction ──────────────────────────────────────

@test "gate ignores non-Bash tool_name" {
  local out
  out=$(run_gate Edit "vitest run") || true
  [ -z "$out" ]
}

@test "gate allows unrecognized command silently" {
  local out
  out=$(run_gate Bash "ls -la")
  [ -z "$out" ]
}

@test "gate allows a rewritten command through" {
  local out
  out=$(run_gate Bash "tsc --noEmit")
  [[ "$out" == *'"permissionDecision":"allow"'* ]]
}

@test "gate never denies" {
  touch "$COOLANT_LOCKFILE"
  local out
  out=$(run_gate Bash "tsc --noEmit")
  [[ "$out" != *"deny"* ]]
}

# ── Classes ─────────────────────────────────────────────────

@test "gate routes bare vitest run as a whole suite" {
  [ "$(routed "vitest run")" = "$(DOOR_PREFIX suite)vitest run" ]
}

@test "gate routes vitest over a directory as a whole suite" {
  [ "$(routed "vitest run test/")" = "$(DOOR_PREFIX suite)vitest run test/" ]
}

@test "gate routes vitest naming test files as files" {
  [ "$(routed "vitest run test/a.test.ts test/b.test.ts")" = "$(DOOR_PREFIX files)vitest run test/a.test.ts test/b.test.ts" ]
}

@test "gate routes jest naming a spec file as files" {
  [ "$(routed "jest src/x.spec.js")" = "$(DOOR_PREFIX files)jest src/x.spec.js" ]
}

@test "gate does not read a -t name as a file" {
  [ "$(routed "vitest run -t parses")" = "$(DOOR_PREFIX suite)vitest run -t parses" ]
}

@test "gate routes pytest naming a .py file as files" {
  [ "$(routed "pytest tests/test_a.py")" = "$(DOOR_PREFIX files)pytest tests/test_a.py" ]
}

@test "gate routes go test as a suite" {
  [ "$(routed "go test ./...")" = "$(DOOR_PREFIX suite)go test ./..." ]
}

@test "gate routes cargo test as a suite" {
  [ "$(routed "cargo test")" = "$(DOOR_PREFIX suite)cargo test" ]
}

@test "gate routes swift test as a suite" {
  [ "$(routed "swift test")" = "$(DOOR_PREFIX suite)swift test" ]
}

@test "gate routes xcodebuild test as a suite" {
  [ "$(routed "xcodebuild test")" = "$(DOOR_PREFIX suite)xcodebuild test" ]
}

@test "gate routes tsc as typecheck" {
  [ "$(routed "tsc --noEmit")" = "$(DOOR_PREFIX typecheck)tsc --noEmit" ]
}

@test "gate routes mypy as typecheck" {
  [ "$(routed "mypy src")" = "$(DOOR_PREFIX typecheck)mypy src" ]
}

@test "gate routes eslint as lint" {
  [ "$(routed "eslint src/")" = "$(DOOR_PREFIX lint)eslint src/" ]
}

@test "gate routes cargo clippy as lint" {
  [ "$(routed "cargo clippy")" = "$(DOOR_PREFIX lint)cargo clippy" ]
}

@test "gate routes go vet as lint" {
  [ "$(routed "go vet ./...")" = "$(DOOR_PREFIX lint)go vet ./..." ]
}

@test "gate routes esbuild as build" {
  [ "$(routed "esbuild src/app.ts --bundle")" = "$(DOOR_PREFIX build)esbuild src/app.ts --bundle" ]
}

@test "gate routes tailwindcss as build" {
  [ "$(routed "tailwindcss -i in.css -o out.css")" = "$(DOOR_PREFIX build)tailwindcss -i in.css -o out.css" ]
}

@test "gate routes vite build as build" {
  [ "$(routed "vite build")" = "$(DOOR_PREFIX build)vite build" ]
}

@test "gate routes go build as build" {
  [ "$(routed "go build ./cmd/x")" = "$(DOOR_PREFIX build)go build ./cmd/x" ]
}

@test "gate routes cargo build as build" {
  [ "$(routed "cargo build")" = "$(DOOR_PREFIX build)cargo build" ]
}

@test "gate routes gradle as build" {
  [ "$(routed "gradle assemble")" = "$(DOOR_PREFIX build)gradle assemble" ]
}

@test "gate leaves vite dev alone" {
  [ -z "$(run_gate Bash "vite dev")" ]
}

@test "gate leaves bare vite alone" {
  [ -z "$(run_gate Bash "vite")" ]
}

@test "gate leaves cargo without a heavy subcommand alone" {
  [ -z "$(run_gate Bash "cargo fmt")" ]
}

@test "gate leaves swift without a heavy subcommand alone" {
  [ -z "$(run_gate Bash "swift --version")" ]
}

@test "gate no longer gates prettier" {
  [ -z "$(run_gate Bash "prettier --write .")" ]
}

# ── Wrappers and shell shape stay outside the door ─────────

@test "gate keeps the npx wrapper inside the door" {
  [ "$(routed "npx vitest run")" = "$(DOOR_PREFIX suite)npx vitest run" ]
}

@test "gate keeps an env wrapper inside the door" {
  [ "$(routed "env CI=1 vitest run")" = "$(DOOR_PREFIX suite)env CI=1 vitest run" ]
}

# The door applies leading NAME=value words the way env(1) does, so the
# gate hands the typed command over whole.
@test "gate hands leading assignments to the door" {
  [ "$(routed "CI=1 NODE_ENV=test vitest run")" = "$(DOOR_PREFIX suite)CI=1 NODE_ENV=test vitest run" ]
}

@test "gate leaves a quoted leading assignment alone" {
  [ -z "$(run_gate Bash "FOO='a b' vitest run" || true)" ]
}

@test "gate classifies through a path prefix" {
  [ "$(routed "./node_modules/.bin/tsc -p .")" = "$(DOOR_PREFIX typecheck)./node_modules/.bin/tsc -p ." ]
}

@test "gate leaves a pipeline outside the door" {
  [ "$(routed "vitest run | tail -5")" = "$(DOOR_PREFIX suite)vitest run | tail -5" ]
}

@test "gate leaves a redirect and chain outside the door" {
  [ "$(routed "tsc --noEmit 2>&1 && echo ok")" = "$(DOOR_PREFIX typecheck)tsc --noEmit 2>&1 && echo ok" ]
}

@test "gate scopes the files check to the gated segment" {
  [ "$(routed "vitest run | grep a.test.ts")" = "$(DOOR_PREFIX suite)vitest run | grep a.test.ts" ]
}

@test "gate keeps a quoted argument intact" {
  [ "$(routed "vitest run -t 'a|b'")" = "$(DOOR_PREFIX suite)vitest run -t 'a|b'" ]
}

# _nested_command stops at the first escaped quote, so a double-quoted
# argument arrives truncated (ending in a backslash) and is declined.
@test "gate declines a double-quoted argument it only sees truncated" {
  [ -z "$(run_gate Bash 'vitest run -t "parses dates"' || true)" ]
}

@test "gate quotes a door path holding a space" {
  local out
  out=$(make_pre_tool_use Bash "tsc" | COOLANT_DOOR="/a b/coolant" bash "$PROJECT_ROOT/scripts/gate.sh")
  [[ "$out" == *"\"command\":\"'/a b/coolant' run --class typecheck -- tsc\""* ]]
}

# ── Parse refusals: decline rather than guess ──────────────

@test "gate declines command substitution" {
  [ -z "$(run_gate Bash "vitest run \$(ls src | head -1)" || true)" ]
}

@test "gate declines backticks" {
  # shellcheck disable=SC2016  # the backticks are the payload
  [ -z "$(run_gate Bash 'tsc `cat files`' || true)" ]
}

@test "gate declines a subshell" {
  [ -z "$(run_gate Bash "(tsc --noEmit)" || true)" ]
}

@test "gate declines an interior backslash" {
  [ -z "$(run_gate Bash "pytest -k 'a\\.b' tests/" || true)" ]
}

@test "gate logs a declined command" {
  run_gate Bash "vitest run \$(ls)" || true
  grep -q "unrouted: vitest run" "$COOLANT_LOG"
}

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

# ── Event bus ───────────────────────────────────────────────

@test "gate emits a gate.route event naming the class" {
  run_gate Bash "npx tsc --noEmit" >/dev/null
  grep -q '"event":"gate.route","tool":"Bash","command":"npx tsc --noEmit","class":"typecheck"' "$COOLANT_EVENTS"
}

@test "gate emits no gate.cap or gate.suppress events" {
  touch "$COOLANT_LOCKFILE"
  run_gate Bash "vitest run" >/dev/null
  run_gate Bash "tsc" >/dev/null
  run grep -c 'gate.cap\|gate.suppress' "$COOLANT_EVENTS"
  [ "$output" = "0" ]
}
