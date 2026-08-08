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
  agents=$(_reconcile_counter)
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
    cargo)      if [ "$sub" = "test" ]; then printf '%s' "-j"; fi ;;
    go)         if [ "$sub" = "test" ]; then printf '%s' "-parallel"; fi ;;
    swift)      if [ "$sub" = "test" ]; then printf '%s' "-j"; fi ;;
    xcodebuild) if [ "$sub" = "test" ]; then printf '%s' "-parallel-testing-worker-count"; fi ;;
  esac
}

# Byte offset of the first shell control operator sitting outside quotes, or the
# string length when there is none. Quote-aware so a pipe inside an argument
# (vitest -t 'a|b') isn't mistaken for a pipeline. Char loop, but commands are
# short and this saves a fork on every gated Bash call.
first_operator_offset() {
  local cmd="$1"
  local n=${#cmd} i=0 c q=""
  while [ "$i" -lt "$n" ]; do
    c="${cmd:$i:1}"
    if [ -n "$q" ]; then
      # Inside quotes: only the matching close quote is significant
      if [ "$c" = "$q" ]; then q=""; fi
    else
      case "$c" in
        \'|\") q="$c" ;;
        \\)    i=$((i + 1)) ;;
        '|'|'&'|';'|'<'|'>') printf '%s' "$i"; return ;;
      esac
    fi
    i=$((i + 1))
  done
  printf '%s' "$n"
}

# Build the rewritten command with cap flag inserted.
# The flag belongs to the gated invocation, so it goes before any pipeline or
# chain operator — appending to the whole line lands it on the downstream
# command instead (vitest run | tail -5 → tail -5 --maxConcurrency N).
apply_cap() {
  local cmd="$1" bin="$2" sub="$3" flag="$4" cap="$5"
  local off head tail gap capped
  off=$(first_operator_offset "$cmd")
  head="${cmd:0:$off}"
  tail="${cmd:$off}"
  # Move the whitespace that preceded the operator to after the flag, so the
  # original spacing is preserved verbatim ("run | tail" keeps its space,
  # "run; echo" stays tight) rather than a space being invented.
  gap=""
  while [ "${head% }" != "$head" ]; do
    head="${head% }"
    gap=" ${gap}"
  done

  # cargo test: insert -j N after "test" and before any "--"
  if [ "$bin" = "cargo" ] && [ "$sub" = "test" ] && [[ "$head" == *" -- "* ]]; then
    local before="${head%% -- *}"
    local after="${head#* -- }"
    capped="${before} ${flag} ${cap} -- ${after}"
  else
    capped="${head} ${flag} ${cap}"
  fi

  echo "${capped}${gap}${tail}"
}

# ── Emit functions ─────────────────────────────────────────

# Deny: block the command entirely
emit_deny() {
  local cmd="$1"
  local safe_cmd
  safe_cmd=$(_json_escape "$cmd")
  coolant_event '"event":"gate.suppress","tool":"Bash","command":"'"$safe_cmd"'","reason":"parallel_mode"'
  coolant_log "blocked: $cmd (parallel mode)"
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"[coolant] blocked: %s (parallel mode — /coolant to release)"}}\n' "$safe_cmd"
}

# Cap: allow with rewritten command
emit_cap() {
  local orig="$1" rewritten="$2"
  local safe_orig safe_rewritten
  safe_orig=$(_json_escape "$orig")
  safe_rewritten=$(_json_escape "$rewritten")
  coolant_event '"event":"gate.cap","tool":"Bash","command":"'"$safe_orig"'","rewritten":"'"$safe_rewritten"'"'
  coolant_log "throttled: $orig -> $rewritten"
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
  # Multi-word: Swift
  swift)
    case "$subcommand" in
      test)       gate_cap "$command" "$binary" "$subcommand" ;;
      build)      gate_suppress "$command" ;;
    esac
    ;;
  xcodebuild)
    case "$subcommand" in
      test)                   gate_cap "$command" "$binary" "$subcommand" ;;
      build|archive|analyze)  gate_suppress "$command" ;;
    esac
    ;;
  # Suppress-only targets
  swiftlint|mypy|pylint|ruff)
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
