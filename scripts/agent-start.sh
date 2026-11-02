#!/bin/bash
set -euo pipefail
# SubagentStart hook: increment agent counter, record start time

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin for agent metadata
input=$(cat)
_extract_agent_fields "$input"

# Atomic increment via mkdir mutex (see common.sh)
if ! coolant_lock; then
  coolant_log "agent-start: lock failed, proceeding unprotected"
fi
current=$(_read_counter)
next=$((current + 1))
echo "$next" > "$COOLANT_COUNTER"
_record_agent_start "$_agent_id" "$(date +%s)"
coolant_unlock

coolant_log "agent started ($next active)"
coolant_event '"event":"agent.start","session_id":"'"$_agent_session_id"'","agent_id":"'"$_agent_id"'","agent_type":"'"$_agent_type"'","cwd":"'"$_agent_cwd"'","project":"'"$_agent_project"'","agent_count":'"$next"

