#!/bin/bash
set -euo pipefail
# PreToolUse hook: gate expensive CLI tools during parallel mode.
# Reads PreToolUse JSON from stdin, pattern-matches the command,
# and emits deny/allow decisions back to Claude Code.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin
input=$(cat)

# Only gate Bash tool calls — fast glob match avoids forking sed
if [[ "$input" != *'"tool_name"'*'"Bash"'* ]]; then
  exit 0
fi

# Extract the command being run (one sed fork — unavoidable for nested JSON)
command=$(echo "$input" | _nested_command)
if [ -z "$command" ]; then
  exit 0
fi

# First word is the binary name
binary="${command%% *}"

# Strip path prefix (/usr/local/bin/tsc → tsc)
binary="${binary##*/}"

# Skip transparent command wrappers (npx, env, command, nice, time, etc.)
# and re-extract the actual binary
case "$binary" in
  npx|env|command|nice|time|sudo)
    rest="${command#* }"
    # Skip any flags (e.g., env -S, nice -n 5)
    while [[ "$rest" == -* ]]; do
      rest="${rest#* }"
    done
    binary="${rest%% *}"
    binary="${binary##*/}"
    command="$rest"
    ;;
esac

# For multi-word tools (cargo build, go test, vite build),
# extract the subcommand via parameter expansion (no fork)
subcommand=""
if [[ "$command" == *" "* ]]; then
  subcommand="${command#* }"
  subcommand="${subcommand%% *}"
fi

# Deny the command: emit JSONL event + PreToolUse deny response
emit_deny() {
  local cmd="$1"
  # Escape backslashes then double quotes for safe JSON embedding
  local safe_cmd="${cmd//\\/\\\\}"
  safe_cmd="${safe_cmd//\"/\\\"}"
  coolant_event '"event":"gate.suppress","tool":"Bash","command":"'"$safe_cmd"'","reason":"parallel_mode"'
  coolant_log "$cmd suppressed (parallel mode)"
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"[coolant] %s suppressed — parallel mode active"}}\n' "$safe_cmd"
}

# Check if the command should be gated. Currently: suppress during parallel mode.
# Future: concurrency cap, debounce.
check_gate() {
  local cmd="$1"
  if [ -f "$COOLANT_LOCKFILE" ]; then
    emit_deny "$cmd"
    exit 0
  fi
  # No lockfile — allow
  exit 0
}

# Pattern match on the binary name
case "$binary" in
  tsc|vitest|jest|eslint|prettier|webpack|esbuild)
    check_gate "$command"
    ;;
  cargo)
    case "$subcommand" in
      build|test|clippy|check)
        check_gate "$command"
        ;;
    esac
    ;;
  go)
    case "$subcommand" in
      build|test|vet)
        check_gate "$command"
        ;;
    esac
    ;;
  pytest|mypy|pylint|ruff)
    check_gate "$command"
    ;;
  gradle|mvn|javac)
    check_gate "$command"
    ;;
  vite)
    if [ "$subcommand" = "build" ]; then
      check_gate "$command"
    fi
    ;;
esac

# Unrecognized or ungated command — allow silently
exit 0
