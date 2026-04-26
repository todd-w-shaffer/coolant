#!/bin/bash
set -euo pipefail
# SubagentStop hook: decrement agent counter, auto-disengage when all agents done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin for agent metadata
input=$(cat)
_extract_agent_fields "$input"

# Defense against CC SubagentStop bug (#44971/#49671): some shutdown
# paths fire SubagentStop with an empty agent_type even when no real
# subagent ran. Treating those as agent stops corrupts the counter
# and the orphan column. Drop them before any state mutation —
# nothing logged into JSONL, no counter change, no parallel-disengage
# check. Keeps a coolant_log line for diagnostic visibility.
if [ -z "$_agent_type" ]; then
  coolant_log "agent-stop: dropped empty agent_type (CC bug defense)"
  exit 0
fi

# Telemetry tail. Empty out-vars (parse failure) → no tail emitted;
# Go consumers default missing fields to zero.
_parse_agent_telemetry "$_agent_transcript_path"
_telemetry_tail=""
if [ -n "$_agent_tokens_in" ] && [ -n "$_agent_tokens_out" ] && [ -n "$_agent_tool_call_count" ]; then
  _telemetry_tail=',"tokens_in":'"$_agent_tokens_in"',"tokens_out":'"$_agent_tokens_out"',"tool_call_count":'"$_agent_tool_call_count"
fi

# Atomic decrement via mkdir mutex (see common.sh)
if ! coolant_lock; then
  coolant_log "agent-stop: lock failed, proceeding unprotected"
fi
current=$(_read_counter)
next=$((current - 1))
underflow_raw=""

# Floor at zero. A trigger here means we under-counted upstream;
# the counter.underflow event below carries the pre-floor value for
# diagnostic visibility. Emission happens AFTER coolant_unlock since
# coolant_event takes its own (non-reentrant) lock.
if [ "$next" -lt 0 ]; then
  underflow_raw="$next"
  next=0
fi

echo "$next" > "$COOLANT_COUNTER"
_compute_agent_duration "$_agent_id" "$(date +%s)"
coolant_unlock

if [ -n "$underflow_raw" ]; then
  coolant_log "WARN: counter underflow (raw=$underflow_raw)"
  coolant_event '"event":"counter.underflow","session_id":"'"$_agent_session_id"'","raw":'"$underflow_raw"
fi

coolant_log "agent stopped ($next remaining)"
_stop_tail=""
[ -n "${_agent_duration_s}" ] && _stop_tail=',"duration_s":'"$_agent_duration_s"
coolant_event '"event":"agent.stop","session_id":"'"$_agent_session_id"'","agent_id":"'"$_agent_id"'","agent_type":"'"$_agent_type"'","cwd":"'"$_agent_cwd"'","project":"'"$_agent_project"'","permission_mode":"'"$_agent_permission_mode"'","transcript_path":"'"$_agent_transcript_path"'","agent_count":'"$next""$_telemetry_tail""$_stop_tail"

if [ "$next" -eq 0 ] && [ -f "$COOLANT_LOCKFILE" ]; then
  rm -f "$COOLANT_LOCKFILE"
  coolant_log "parallel mode auto-disengaged (all agents done)"
  coolant_event '"event":"parallel.disengaged","agent_count":0'
  echo "{\"systemMessage\":\"[coolant] all agents finished — parallel mode auto-disengaged. Per-edit typecheck re-enabled. Run your build gate now.\"}"
fi
