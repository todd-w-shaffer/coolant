#!/bin/bash
set -euo pipefail
# PreToolUse hook: gate expensive CLI tools.
# - Test runners: always capped with concurrency limit based on agent count
# - Type checkers/linters/build tools: suppressed during parallel mode
#
# Reads PreToolUse JSON from stdin, pattern-matches the command,
# and emits deny/allow/rewrite decisions back to Claude Code.

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

# ── Capping functions ──────────────────────────────────────

# Compute concurrency cap: floor((cores - 2) / agents), min 1
compute_cap() {
  local agents
  agents=$(_read_counter)
  # Zero agents means no contention — default to 1 for the division
  if [ "$agents" -lt 1 ]; then
    agents=1
  fi
  local cap=$(( (_COOLANT_NCPU - 2) / agents ))
  if [ "$cap" -lt 1 ]; then
    cap=1
  fi
  echo "$cap"
}

# Map binary+subcommand to the ecosystem-specific concurrency flag
cap_flag() {
  local bin="$1" sub="$2"
  case "$bin" in
    vitest) printf '%s' "--maxConcurrency" ;;
    jest)   printf '%s' "--maxWorkers" ;;
    pytest) printf '%s' "-n" ;;
    cargo)  if [ "$sub" = "test" ]; then printf '%s' "-j"; fi ;;
    go)     if [ "$sub" = "test" ]; then printf '%s' "-parallel"; fi ;;
  esac
}

# Build the rewritten command with cap flag inserted
apply_cap() {
  local cmd="$1" bin="$2" sub="$3" flag="$4" cap="$5"
  # cargo test: insert -j N after "test" and before any "--"
  if [ "$bin" = "cargo" ] && [ "$sub" = "test" ]; then
    if [[ "$cmd" == *" -- "* ]]; then
      local before="${cmd%% -- *}"
      local after="${cmd#* -- }"
      echo "${before} ${flag} ${cap} -- ${after}"
    else
      echo "${cmd} ${flag} ${cap}"
    fi
  else
    echo "${cmd} ${flag} ${cap}"
  fi
}

# ── Emit functions ─────────────────────────────────────────

# Deny: block the command entirely
emit_deny() {
  local cmd="$1"
  local safe_cmd
  safe_cmd=$(_json_escape "$cmd")
  coolant_event '"event":"gate.suppress","tool":"Bash","command":"'"$safe_cmd"'","reason":"parallel_mode"'
  coolant_log "$cmd suppressed (parallel mode)"
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"[coolant] %s suppressed — parallel mode active"}}\n' "$safe_cmd"
}

# Cap: allow with rewritten command
emit_cap() {
  local orig="$1" rewritten="$2"
  local safe_orig safe_rewritten
  safe_orig=$(_json_escape "$orig")
  safe_rewritten=$(_json_escape "$rewritten")
  coolant_event '"event":"gate.cap","tool":"Bash","command":"'"$safe_orig"'","original":"'"$safe_orig"'","rewritten":"'"$safe_rewritten"'"'
  coolant_log "capped: $orig -> $rewritten"
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"command":"%s"}}}\n' "$safe_rewritten"
}

# ── Gate entry points ──────────────────────────────────────

# Suppress target: deny during parallel mode, allow otherwise
gate_suppress() {
  local cmd="$1"
  if [ -f "$COOLANT_LOCKFILE" ]; then
    emit_deny "$cmd"
    exit 0
  fi
  exit 0
}

# Cap target: compute cap, rewrite command, allow always
gate_cap() {
  local cmd="$1" bin="$2" sub="$3"
  local flag
  flag=$(cap_flag "$bin" "$sub")
  if [ -z "$flag" ]; then
    exit 0
  fi
  # Don't override if flag already present (word-boundary match avoids
  # false positives on paths like tests/test-n-gram.py matching "-n")
  case " $cmd " in
    *" $flag "*|*" $flag="*) exit 0 ;;
  esac
  local cap
  cap=$(compute_cap)
  local rewritten
  rewritten=$(apply_cap "$cmd" "$bin" "$sub" "$flag" "$cap")
  emit_cap "$cmd" "$rewritten"
  exit 0
}

# ── Dispatch ───────────────────────────────────────────────

case "$binary" in
  # Cap targets (test runners)
  vitest|jest|pytest)
    gate_cap "$command" "$binary" ""
    ;;
  # Suppress targets (type checkers, linters, build tools)
  tsc|eslint|prettier|webpack|esbuild)
    gate_suppress "$command"
    ;;
  # Multi-word: route by subcommand
  cargo)
    case "$subcommand" in
      test)              gate_cap "$command" "$binary" "$subcommand" ;;
      build|clippy|check) gate_suppress "$command" ;;
    esac
    ;;
  go)
    case "$subcommand" in
      test)       gate_cap "$command" "$binary" "$subcommand" ;;
      build|vet)  gate_suppress "$command" ;;
    esac
    ;;
  # Suppress-only targets
  mypy|pylint|ruff)
    gate_suppress "$command"
    ;;
  gradle|mvn|javac)
    gate_suppress "$command"
    ;;
  vite)
    if [ "$subcommand" = "build" ]; then
      gate_suppress "$command"
    fi
    ;;
esac

# Unrecognized or ungated command — allow silently
exit 0
