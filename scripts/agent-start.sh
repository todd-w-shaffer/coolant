#!/bin/bash
# SubagentStart hook: increment agent counter, auto-engage parallel mode at threshold

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Atomic-ish increment (good enough for our use case)
current=$(cat "$COOLANT_COUNTER" 2>/dev/null || echo "0")
next=$((current + 1))
echo "$next" > "$COOLANT_COUNTER"

coolant_log "agent started ($next active)"

# Auto-engage at threshold
if [ "$next" -ge "$COOLANT_THRESHOLD" ] && [ ! -f "$COOLANT_LOCKFILE" ]; then
  touch "$COOLANT_LOCKFILE"
  coolant_log "parallel mode auto-engaged (threshold: $COOLANT_THRESHOLD)"
  echo "{\"systemMessage\":\"[coolant] ${next} agents active (threshold: ${COOLANT_THRESHOLD}) — parallel mode auto-engaged. Per-edit typecheck suppressed.\"}"
fi
