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

# Reconcile counter file against JSONL event log ground truth.
# If JSONL exists and derived count differs, fix the counter.
# Returns the reconciled count on stdout.
_reconcile_counter() {
  if [ ! -f "$COOLANT_EVENTS" ]; then
    _read_counter
    return
  fi
  local starts stops jsonl_count file_count events
  # Scope to events after last counter.reset (if any)
  local reset_line
  reset_line=$(grep -n '"event":"counter.reset"' "$COOLANT_EVENTS" 2>/dev/null | tail -1 | cut -d: -f1) || true
  if [ -n "$reset_line" ]; then
    events=$(tail -n +"$((reset_line + 1))" "$COOLANT_EVENTS")
  else
    events=$(cat "$COOLANT_EVENTS")
  fi
  starts=$(printf '%s\n' "$events" | grep -c '"event":"agent.start"' 2>/dev/null) || starts=0
  stops=$(printf '%s\n' "$events" | grep -c '"event":"agent.stop"' 2>/dev/null) || stops=0
  jsonl_count=$((starts - stops))
  if [ "$jsonl_count" -lt 0 ]; then
    jsonl_count=0
  fi
  file_count=$(_read_counter)
  if [ "$jsonl_count" -ne "$file_count" ]; then
    echo "$jsonl_count" > "$COOLANT_COUNTER"
    coolant_log "reconciled counter: file=$file_count jsonl=$jsonl_count"
  fi
  printf '%s' "$jsonl_count"
}

# Extract session_id, agent_id, agent_type from hook JSON.
# Inline regex + parameter expansion — zero forks.
# Sets: _agent_session_id, _agent_id, _agent_type
_extract_agent_fields() {
  local _eaf_json="$1" _val

  _val=""
  [[ "$_eaf_json" =~ \"session_id\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]] && _val="${BASH_REMATCH[1]}"
  _val="${_val//\\/\\\\}"; _val="${_val//\"/\\\"}"; _val="${_val//$'\n'/\\n}"; _val="${_val//$'\t'/\\t}"
  _agent_session_id="$_val"

  _val=""
  [[ "$_eaf_json" =~ \"agent_id\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]] && _val="${BASH_REMATCH[1]}"
  _val="${_val//\\/\\\\}"; _val="${_val//\"/\\\"}"; _val="${_val//$'\n'/\\n}"; _val="${_val//$'\t'/\\t}"
  _agent_id="$_val"

  _val=""
  [[ "$_eaf_json" =~ \"agent_type\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]] && _val="${BASH_REMATCH[1]}"
  _val="${_val//\\/\\\\}"; _val="${_val//\"/\\\"}"; _val="${_val//$'\n'/\\n}"; _val="${_val//$'\t'/\\t}"
  _agent_type="$_val"
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
