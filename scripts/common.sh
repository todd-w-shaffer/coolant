#!/usr/bin/env bash
# Shared config and logging for coolant scripts

_COOLANT_DIR="${TMPDIR:-/tmp/}"
COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-${_COOLANT_DIR}coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-${_COOLANT_DIR}coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-${_COOLANT_DIR}coolant-${USER}.log}"
COOLANT_EVENTS="${COOLANT_EVENTS:-${_COOLANT_DIR}coolant-${USER}.events.jsonl}"
COOLANT_THRESHOLD="${COOLANT_THRESHOLD:-3}"
_COOLANT_NCPU="${_COOLANT_NCPU:-$(sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

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
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\t'/\\t}"
  printf '%s' "$s"
}

# Extract a top-level string field from single-line JSON on stdin.
# Uses bash regex instead of forking sed.
_json_field() {
  local _jf_input _jf_key="$1"
  _jf_input=$(cat)
  if [[ "$_jf_input" =~ \"$_jf_key\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  fi
}

# Extract "command" from inside tool_input object on stdin.
# Uses bash regex instead of forking sed.
_nested_command() {
  local _nc_input
  _nc_input=$(cat)
  if [[ "$_nc_input" =~ \"tool_input\"[[:space:]]*:[[:space:]]*\{[^\}]*\"command\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  fi
}

# Read the agent counter file, validate as positive integer, default to 0.
_read_counter() {
  local val
  val=$(cat "$COOLANT_COUNTER" 2>/dev/null) || val=""
  if [[ "$val" =~ ^[0-9]+$ ]]; then
    printf '%s' "$val"
  else
    printf '%s' "0"
  fi
}

# Extract session_id, agent_id, agent_type from hook JSON.
# Reads from the variable name passed as $1 (or "_eaf_stdin" from stdin).
# Sets: _agent_session_id, _agent_id, _agent_type
_extract_agent_fields() {
  local _eaf_json="$1"
  _agent_session_id=$(_json_escape "$(echo "$_eaf_json" | _json_field session_id)")
  _agent_id=$(_json_escape "$(echo "$_eaf_json" | _json_field agent_id)")
  _agent_type=$(_json_escape "$(echo "$_eaf_json" | _json_field agent_type)")
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
    read -t 0.01 < /dev/null 2>/dev/null || true
  done
}

coolant_unlock() {
  rmdir "$_COOLANT_MUTEX" 2>/dev/null
}
