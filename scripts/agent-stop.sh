#!/bin/bash
set -euo pipefail
# SubagentStop hook: decrement agent counter, auto-disengage when all agents done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin for agent metadata
input=$(cat)
_extract_agent_fields "$input"

# Atomic decrement via mkdir mutex (see common.sh)
if ! coolant_lock; then
  coolant_log "agent-stop: lock failed, proceeding unprotected"
fi
current=$(_read_counter)
next=$((current - 1))

# Floor at zero
if [ "$next" -lt 0 ]; then
  next=0
fi

echo "$next" > "$COOLANT_COUNTER"
coolant_unlock

coolant_log "agent stopped ($next remaining)"
coolant_event '"event":"agent.stop","session_id":"'"$_agent_session_id"'","agent_id":"'"$_agent_id"'","agent_type":"'"$_agent_type"'","cwd":"'"$_agent_cwd"'","project":"'"$_agent_project"'","permission_mode":"'"$_agent_permission_mode"'","transcript_path":"'"$_agent_transcript_path"'","agent_count":'"$next"

if [ "$next" -eq 0 ] && [ -f "$COOLANT_LOCKFILE" ]; then
  rm -f "$COOLANT_LOCKFILE"
  coolant_log "parallel mode auto-disengaged (all agents done)"
  coolant_event '"event":"parallel.disengaged","agent_count":0'
  echo "{\"systemMessage\":\"[coolant] all agents finished — parallel mode auto-disengaged. Per-edit typecheck re-enabled. Run your build gate now.\"}"
fi
