#!/bin/bash
# SubagentStop hook: decrement agent counter, auto-disengage when all agents done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

current=$(cat "$COOLANT_COUNTER" 2>/dev/null || echo "1")
next=$((current - 1))

# Floor at zero
if [ "$next" -lt 0 ]; then
  next=0
fi

echo "$next" > "$COOLANT_COUNTER"

coolant_log "agent stopped ($next remaining)"

if [ "$next" -eq 0 ] && [ -f "$COOLANT_LOCKFILE" ]; then
  rm -f "$COOLANT_LOCKFILE"
  coolant_log "parallel mode auto-disengaged (all agents done)"
  echo "{\"systemMessage\":\"[coolant] all agents finished — parallel mode auto-disengaged. Per-edit typecheck re-enabled. Run your build gate now.\"}"
fi
