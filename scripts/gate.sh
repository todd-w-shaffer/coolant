#!/bin/bash
set -euo pipefail
# PreToolUse hook: route typed heavy commands into the work queue.
#
# A test runner, type checker, linter or build tool typed into the Bash
# tool is rewritten to `<door> run --class <class> -- <command>`, where
# <door> is scripts/coolant beside this hook. The door is a prefix, so
# pipes, chains and redirects stay outside it and apply as typed. The
# gate never caps and never denies: admission is the queue's job.
#
# Reads PreToolUse JSON from stdin; emits an allow-with-updatedInput
# decision, or nothing (leave the command alone).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

COOLANT_DOOR="${COOLANT_DOOR:-${SCRIPT_DIR}/coolant}"

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

# Leading assignments (CI=1 vitest run) are skipped to find the binary;
# the door applies them as env(1) does, so the typed command goes to it
# whole. Only plain values — a quoted value would need word splitting
# the gate does not do, so such a command is simply not recognized.
original_command="$command"
while [[ "$command" =~ ^[A-Za-z_][A-Za-z0-9_]*=[^\ \'\"]*\ (.*)$ ]]; do
  command="${BASH_REMATCH[1]}"
done

# First word is the binary name
binary="${command%% *}"

# Strip path prefix (/usr/local/bin/tsc → tsc)
binary="${binary##*/}"

# Skip transparent command wrappers (npx, env, command, nice, time, etc.)
# and re-extract the actual binary. The wrapper stays in the command: the
# door runs the whole thing, launcher included (npx vitest → npx vitest).
# $command is only for classifying; $original_command is what is routed.
case "$binary" in
  npx|env|command|nice|time|sudo)
    rest="${command#* }"
    # Skip any flags (e.g., env -S, nice -n 5) and env's assignments
    # (env CI=1 vitest). "${rest#* }" is a no-op once rest holds no space,
    # so a bare-flag wrapper ("sudo -v") would spin here forever and hang
    # the hook — stop when there is nothing left to strip.
    while [[ "$rest" == -* ]] || [[ "$binary" == env && "$rest" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; do
      case "$rest" in
        *" "*) rest="${rest#* }" ;;
        *)     break ;;
      esac
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

# ── Parsing ────────────────────────────────────────────────

# Offset of the first shell control operator or redirect sitting outside
# quotes, or the string length when there is none. Quote-aware so a pipe
# inside an argument (vitest -t 'a|b') isn't mistaken for a pipeline. Used
# to scope the class check to the gated invocation: `vitest run | grep
# a.test.ts` names no test file. (A redirect's fd digits land on the left
# of the split, which is harmless: a digit run is never file-shaped.)
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
        '<'|'>'|'|'|'&'|';') printf '%s' "$i"; return ;;
      esac
    fi
    i=$((i + 1))
  done
  printf '%s' "$n"
}

# Whether the gate can be trusted with this command. It understands quoting
# and fd-prefixed redirects; nothing else. Command substitution, subshells
# and process substitution hide structure the gate would misread, and a
# truncated command (trailing backslash, unbalanced quote — _nested_command
# stops at the first escaped quote) isn't the real command at all. Routing
# a command that is not the one typed would run something else, so decline.
route_parseable() {
  local cmd="$1"
  # '(' covers $( and <( as well as a bare subshell.
  case "$cmd" in
    *'`'*|*'('*) return 1 ;;
  esac
  [[ "$cmd" != *$'\n'* ]] || return 1
  # Any backslash at all: _nested_command hands us the raw JSON string
  # without unescaping and emit_route re-escapes it, so "a\.b" would come
  # back as "a\\.b" with the selector it belonged to silently broken.
  case "$cmd" in
    *\\*) return 1 ;;
  esac

  # Unbalanced quote — the command reached us truncated.
  local n=${#cmd} i=0 c q=""
  while [ "$i" -lt "$n" ]; do
    c="${cmd:$i:1}"
    if [ -n "$q" ]; then
      if [ "$c" = "$q" ]; then q=""; fi
    else
      case "$c" in
        \'|\") q="$c" ;;
      esac
    fi
    i=$((i + 1))
  done
  [ -z "$q" ]
}

# Class of a test-runner invocation: `files` when the gated segment names
# a test file, `suite` otherwise. A directory filter (test/) still selects
# a whole suite, so only file-shaped words count; unsure means suite, which
# overweights rather than under-.
test_class() {
  local off head word
  off=$(first_operator_offset "$command")
  head="${command:0:$off}"
  for word in $head; do
    case "$word" in
      -*) ;;
      *.test.*|*.spec.*|*.ts|*.tsx|*.js|*.jsx|*.mjs|*.cjs|*.mts|*.cts|*.py)
        printf 'files'
        return
        ;;
    esac
  done
  printf 'suite'
}

# The door as a shell word: bare when it holds only safe characters,
# single-quoted otherwise. Fails on a path holding a single quote.
door_word() {
  case "$COOLANT_DOOR" in
    *"'"*) return 1 ;;
  esac
  if [[ "$COOLANT_DOOR" =~ ^[A-Za-z0-9_./~+-]+$ ]]; then
    printf '%s' "$COOLANT_DOOR"
  else
    printf "'%s'" "$COOLANT_DOOR"
  fi
}

# ── Route ──────────────────────────────────────────────────

gate_route() {
  local class="$1" door rewritten safe_orig safe_rewritten
  if ! door=$(door_word); then
    coolant_log "unrouted: $original_command (door path holds a quote)"
    exit 0
  fi
  rewritten="${door} run --class ${class} -- ${original_command}"
  safe_orig=$(_json_escape "$original_command")
  safe_rewritten=$(_json_escape "$rewritten")
  coolant_event '"event":"gate.route","tool":"Bash","command":"'"$safe_orig"'","class":"'"$class"'","rewritten":"'"$safe_rewritten"'"'
  coolant_log "routed: $original_command -> $class"
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"command":"%s"}}}\n' "$safe_rewritten"
  exit 0
}

# ── Dispatch ───────────────────────────────────────────────

# Only a heavy binary is worth routing; decide that first, then refuse
# anything the gate cannot parse before classifying it.
case "$binary" in
  vitest|jest|pytest|tsc|mypy|eslint|ruff|pylint|swiftlint|esbuild|webpack|tailwindcss) ;;
  gradle|mvn|javac|cargo|go|swift|xcodebuild|vite) ;;
  *) exit 0 ;;
esac
if ! route_parseable "$original_command"; then
  coolant_log "unrouted: $original_command (unparseable shell)"
  exit 0
fi

case "$binary" in
  vitest|jest|pytest)               gate_route "$(test_class)" ;;
  tsc|mypy)                         gate_route typecheck ;;
  eslint|ruff|pylint|swiftlint)     gate_route lint ;;
  esbuild|webpack|tailwindcss)      gate_route build ;;
  gradle|mvn|javac)                 gate_route build ;;
  cargo)
    case "$subcommand" in
      test)         gate_route suite ;;
      clippy|check) gate_route lint ;;
      build)        gate_route build ;;
    esac
    ;;
  go)
    case "$subcommand" in
      test)  gate_route suite ;;
      vet)   gate_route lint ;;
      build) gate_route build ;;
    esac
    ;;
  swift)
    case "$subcommand" in
      test)  gate_route suite ;;
      build) gate_route build ;;
    esac
    ;;
  xcodebuild)
    case "$subcommand" in
      test)                  gate_route suite ;;
      build|archive|analyze) gate_route build ;;
    esac
    ;;
  vite)
    if [ "$subcommand" = "build" ]; then
      gate_route build
    fi
    ;;
esac

# Unrecognized or ungated command — allow silently
exit 0
