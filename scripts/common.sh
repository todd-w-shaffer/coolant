#!/usr/bin/env bash
# Shared config and logging for coolant scripts

_COOLANT_DIR="${TMPDIR:-/tmp/}"
COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-${_COOLANT_DIR}coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-${_COOLANT_DIR}coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-${_COOLANT_DIR}coolant-${USER}.log}"
COOLANT_EVENTS="${COOLANT_EVENTS:-${_COOLANT_DIR}coolant-${USER}.events.jsonl}"
COOLANT_AGENT_STARTS="${COOLANT_AGENT_STARTS:-${_COOLANT_DIR}coolant-${USER}.agent-starts}"
# Degraded-write counter: one newline per lock-failure fallback. Out of
# band from JSONL so a torn line in the degraded path can't itself
# corrupt the event log. Aggregator reads `wc -l` for total count.
COOLANT_DEGRADED_COUNT="${COOLANT_DEGRADED_COUNT:-${_COOLANT_DIR}coolant-${USER}.degraded.count}"
# Per-session JSONL audit log of review-shaped Agent invocations.
# Populated by .claude/hooks/audit-review-agents.sh, gated against by
# .claude/hooks/enforce-spec-to-ship-reviews.sh. Truncated by preflight.
COOLANT_REVIEW_AUDIT="${COOLANT_REVIEW_AUDIT:-${_COOLANT_DIR}coolant-${USER}.review-audit.jsonl}"
# Sidecar holding the current session_id, written by preflight.sh on
# session start. Read by _reconcile_counter and the Go tailer to scope
# per-session counters and event consumption.
COOLANT_SESSION_FILE="${COOLANT_SESSION_FILE:-${_COOLANT_DIR}coolant-${USER}.session}"
COOLANT_THRESHOLD="${COOLANT_THRESHOLD:-3}"
# Maximum bytes of a per-agent transcript that `_parse_agent_telemetry`
# will read. 5 MiB sits well above any realistic agent run (tens of
# thousands of messages); the cap exists to keep the SubagentStop 5s
# hook budget bounded against pathological transcripts. Override for
# very-high-message-count workloads.
COOLANT_TRANSCRIPT_CAP_BYTES="${COOLANT_TRANSCRIPT_CAP_BYTES:-5242880}"
_COOLANT_NCPU="${_COOLANT_NCPU:-$(sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

coolant_log() {
  echo "$(date '+%H:%M:%S')  $1" >> "$COOLANT_LOG"
}

coolant_event() {
  local ts line
  ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  # Format outside the lock — pure-string work, no syscalls. The
  # critical section is just the append.
  printf -v line '{"ts":"%s","schema":1,%s}\n' "$ts" "$1"
  # mkdir-mutex is NOT reentrant: callers already holding coolant_lock
  # (e.g. agent-start.sh) MUST release it before invoking coolant_event.
  if coolant_lock; then
    printf '%s' "$line" >> "$COOLANT_EVENTS"
    coolant_unlock
  else
    # Lock acquisition timed out (~1s). Emit unsynchronized so signal
    # isn't lost — the Go aggregator skips spliced lines on JSON parse
    # error, so worst case is a single dropped event, not corruption.
    printf '%s' "$line" >> "$COOLANT_EVENTS"
    # Out-of-band degradation marker — one byte well under PIPE_BUF,
    # `>>` atomic at this size, no JSON / no torn-line risk inside the
    # path that already lost serialization. Aggregator reads via
    # `wc -l` to surface DegradedWritesTotal.
    printf '\n' >> "$COOLANT_DEGRADED_COUNT"
    coolant_log "coolant_event: lock failed, wrote unsynchronized"
  fi
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
#
# Single awk pass: state-machine reset on counter.reset, then accumulate
# starts/stops. Patterns are anchored on the event-field value-closer
# (`,` or `}`), so substring leaks in other fields don't false-match.
# Constant memory regardless of file size; one I/O instead of the prior
# grep -n + tail + grep -c × 2.
_reconcile_counter() {
  if [ ! -f "$COOLANT_EVENTS" ]; then
    _read_counter
    return
  fi
  local sid="${COOLANT_SESSION_ID:-}"
  if [ -z "$sid" ] && [ -f "$COOLANT_SESSION_FILE" ]; then
    sid=$(cat "$COOLANT_SESSION_FILE" 2>/dev/null) || sid=""
  fi
  local jsonl_count file_count
  # When sid is empty (degraded fallback), the per-line filter is a
  # no-op and all agent.start/agent.stop lines count — same as before
  # the session-id filter landed. When sid is set, only events whose
  # line carries a matching session_id contribute; lines with empty
  # or mismatched session_id are dropped per the per-session
  # correctness contract.
  jsonl_count=$(awk -v sid="$sid" '
    {
      # Extract session_id value from the line. The +14/-15 offsets
      # strip the literal `"session_id":"` prefix (14 chars) and the
      # trailing `"` from a match of length RLENGTH (so the value is
      # RLENGTH - 15 chars). Renaming or padding the key requires
      # updating both numbers.
      if (match($0, /"session_id":"[^"]*"/)) {
        line_sid = substr($0, RSTART+14, RLENGTH-15)
      } else {
        line_sid = ""
      }
    }
    /"event":"counter\.reset"[,}]/ { starts=0; stops=0; next }
    /"event":"agent\.start"[,}]/   { if (sid == "" || line_sid == sid) starts++; next }
    /"event":"agent\.stop"[,}]/    { if (sid == "" || line_sid == sid) stops++; next }
    END {
      d = starts - stops
      if (d < 0) d = 0
      print d
    }
  ' "$COOLANT_EVENTS")

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

# Parse per-agent telemetry from a Claude Code subagent transcript.
# Sets _agent_tokens_in, _agent_tokens_out, _agent_tool_call_count.
#
# Sums per-message usage.input_tokens / usage.output_tokens (taking
# the FIRST match per line so iterations[] copies don't double-count)
# and counts "type":"tool_use" content items via gsub.
#
# Failure modes (sets all three out-vars empty):
#   - empty path
#   - file unreadable
# A readable file with no assistant messages succeeds with all
# three at "0" — distinguishes "parsed, value zero" from "parse
# failed", which the emission discipline in agent-stop.sh relies on.
#
# No jq dependency (per CLAUDE.md). awk is bash 3.2 / macOS BSD-awk
# compatible; the 2-arg match() + substr() pattern avoids gawk's
# 3-arg capture-group form which BSD awk doesn't support.
_parse_agent_telemetry() {
  local _pat_path="$1"
  _agent_tokens_in=""
  _agent_tokens_out=""
  _agent_tool_call_count=""

  [ -n "$_pat_path" ] || return 0
  [ -r "$_pat_path" ] || return 0

  # Cap input bytes — see COOLANT_TRANSCRIPT_CAP_BYTES at the top of
  # this file. Capped parses undercount gracefully on huge transcripts;
  # the hook never blocks past its 5s budget.
  local _pat_out
  _pat_out=$(head -c "$COOLANT_TRANSCRIPT_CAP_BYTES" "$_pat_path" 2>/dev/null | awk '
    function extract_int(line, key,    rx, span, digits) {
      rx = "\"" key "\"[[:space:]]*:[[:space:]]*[0-9]+"
      if (match(line, rx)) {
        span = substr(line, RSTART, RLENGTH)
        if (match(span, /[0-9]+/)) {
          digits = substr(span, RSTART, RLENGTH)
          return digits + 0
        }
      }
      return -1
    }
    {
      v = extract_int($0, "input_tokens")
      if (v >= 0) tin += v
      v = extract_int($0, "output_tokens")
      if (v >= 0) tout += v
      tool += gsub(/"type"[[:space:]]*:[[:space:]]*"tool_use"/, "&")
    }
    END { printf "%d %d %d\n", tin, tout, tool }
  ' 2>/dev/null) || return 0
  [ -n "$_pat_out" ] || return 0

  read -r _agent_tokens_in _agent_tokens_out _agent_tool_call_count <<< "$_pat_out"
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
