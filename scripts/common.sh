#!/usr/bin/env bash
# Shared config and logging for coolant scripts

_COOLANT_DIR="${TMPDIR:-/tmp/}"
COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-${_COOLANT_DIR}coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-${_COOLANT_DIR}coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-${_COOLANT_DIR}coolant-${USER}.log}"
COOLANT_EVENTS="${COOLANT_EVENTS:-${_COOLANT_DIR}coolant-${USER}.events.jsonl}"
COOLANT_AGENT_STARTS="${COOLANT_AGENT_STARTS:-${_COOLANT_DIR}coolant-${USER}.agent-starts}"
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

# Extract a single JSON string field by key, escape for re-emission.
# Sets _val. Zero forks — inline regex + parameter expansion only.
_extract_escaped() {
  local key="$1" json="$2"
  _val=""
  [[ "$json" =~ \"$key\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]] && _val="${BASH_REMATCH[1]}"
  _val="${_val//\\/\\\\}"; _val="${_val//\"/\\\"}"; _val="${_val//$'\n'/\\n}"; _val="${_val//$'\t'/\\t}"
}

# Extract agent metadata from hook JSON.
# Sets: _agent_session_id, _agent_id, _agent_type, _agent_cwd,
#       _agent_project, _agent_permission_mode, _agent_transcript_path
_extract_agent_fields() {
  local _eaf_json="$1"
  _extract_escaped "session_id"           "$_eaf_json"; _agent_session_id="$_val"
  _extract_escaped "agent_id"             "$_eaf_json"; _agent_id="$_val"
  _extract_escaped "agent_type"           "$_eaf_json"; _agent_type="$_val"
  _extract_escaped "cwd"                  "$_eaf_json"; _agent_cwd="$_val"
  # Worktrees live at <repo>/.claude-worktrees/<branch>; without stripping,
  # project would read as the branch name and split a single repo across
  # multiple "projects" in downstream aggregates.
  local _eaf_trimmed="${_agent_cwd%%/.claude-worktrees/*}"
  _agent_project="${_eaf_trimmed##*/}"
  _extract_escaped "permission_mode"      "$_eaf_json"; _agent_permission_mode="$_val"
  _extract_escaped "agent_transcript_path" "$_eaf_json"; _agent_transcript_path="$_val"
}

# Record agent start time in a tab-separated state file (agent_id<TAB>epoch_s).
# Caller should hold coolant_lock to serialize writes, and passes `now` so
# a single date(1) fork is shared across the hook invocation.
_record_agent_start() {
  local agent_id="$1" now="$2"
  printf '%s\t%s\n' "$agent_id" "$now" >> "$COOLANT_AGENT_STARTS"
}

# Look up recorded start ts for agent_id, compute wall-clock duration in
# seconds, and remove the matched line from the state file. Also prunes
# entries older than 24h to bound state-file growth when agents start
# without a matching stop (CC #44971 orphan-stop bug, agent crashes).
# Sets _agent_duration_s to the duration string, or empty if no start recorded.
# Caller should hold coolant_lock.
_compute_agent_duration() {
  local agent_id="$1" now="$2"
  _agent_duration_s=""
  [ -f "$COOLANT_AGENT_STARTS" ] || return 0

  local start_ts cutoff=$((now - 86400))
  # Single awk pass: emit matched start_ts to stdout, write kept lines to tmp.
  # BEGIN initializes tmp so `mv` below always succeeds even if the filter
  # emits nothing (all entries stale or sole entry was our match).
  start_ts=$(awk -v id="$agent_id" -v cutoff="$cutoff" \
                 -v tmp="${COOLANT_AGENT_STARTS}.tmp" -F'\t' '
    BEGIN             { printf "" > tmp }
    $1 == id          { print $2; next }
    $2 + 0 >= cutoff  { print > tmp }
  ' "$COOLANT_AGENT_STARTS")
  mv "${COOLANT_AGENT_STARTS}.tmp" "$COOLANT_AGENT_STARTS"

  if [ -n "$start_ts" ]; then
    _agent_duration_s=$((now - start_ts))
  fi
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
