#!/bin/bash
set -euo pipefail
# SubagentStart hook: increment agent counter, auto-engage parallel mode at threshold

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
coolant_unlock

coolant_log "agent started ($next active)"
coolant_event '"event":"agent.start","session_id":"'"$_agent_session_id"'","agent_id":"'"$_agent_id"'","agent_type":"'"$_agent_type"'","agent_count":'"$next"

# Auto-engage at threshold
if [ "$next" -ge "$COOLANT_THRESHOLD" ] && [ ! -f "$COOLANT_LOCKFILE" ]; then
  touch "$COOLANT_LOCKFILE"
  coolant_log "parallel mode auto-engaged (threshold: $COOLANT_THRESHOLD)"
  coolant_event '"event":"parallel.engaged","agent_count":'"$next"',"threshold":'"$COOLANT_THRESHOLD"
  echo "{\"systemMessage\":\"[coolant] ${next} agents active (threshold: ${COOLANT_THRESHOLD}) — parallel mode auto-engaged. Per-edit typecheck suppressed.\"}"
fi
