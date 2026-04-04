#!/usr/bin/env bash
# Shared config and logging for coolant scripts

_COOLANT_DIR="${TMPDIR:-/tmp/}"
COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-${_COOLANT_DIR}coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-${_COOLANT_DIR}coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-${_COOLANT_DIR}coolant-${USER}.log}"
COOLANT_EVENTS="${COOLANT_EVENTS:-${_COOLANT_DIR}coolant-${USER}.events.jsonl}"
COOLANT_THRESHOLD="${COOLANT_THRESHOLD:-3}"

coolant_log() {
  echo "$(date '+%H:%M:%S')  $1" >> "$COOLANT_LOG"
}

coolant_event() {
  local ts
  ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  printf '{"ts":"%s",%s}\n' "$ts" "$1" >> "$COOLANT_EVENTS"
}

# Escape a string for safe embedding in JSON values.
# Handles backslashes and double quotes.
_json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

# Extract a top-level string field from single-line JSON on stdin.
_json_field() {
  sed -n 's/.*"'"$1"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

# Extract "command" from inside tool_input object on stdin.
_nested_command() {
  sed -n 's/.*"tool_input"[[:space:]]*:[[:space:]]*{[^}]*"command"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

# Atomic counter operations using mkdir as a POSIX mutex.
# mkdir is atomic — only one process can create the directory.
_COOLANT_MUTEX="${COOLANT_COUNTER}.lock"

coolant_lock() {
  local tries=0
  while ! mkdir "$_COOLANT_MUTEX" 2>/dev/null; do
    tries=$((tries + 1))
    if [ "$tries" -gt 100 ]; then
      # Stale lock after ~1s — break it
      rmdir "$_COOLANT_MUTEX" 2>/dev/null
      return 1
    fi
    sleep 0.01
  done
}

coolant_unlock() {
  rmdir "$_COOLANT_MUTEX" 2>/dev/null
}
