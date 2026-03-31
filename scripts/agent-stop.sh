#!/bin/bash
# SubagentStop hook: decrement agent counter, auto-disengage when all agents done

LOCKFILE="${COOLANT_LOCKFILE:-/tmp/coolant-${USER}.lock}"
COUNTER="${COOLANT_COUNTER:-/tmp/coolant-agents-${USER}.count}"

current=$(cat "$COUNTER" 2>/dev/null || echo "1")
next=$((current - 1))

# Floor at zero
if [ "$next" -lt 0 ]; then
  next=0
fi

echo "$next" > "$COUNTER"

if [ "$next" -eq 0 ] && [ -f "$LOCKFILE" ]; then
  rm -f "$LOCKFILE"
  echo "{\"systemMessage\":\"[coolant] all agents finished — parallel mode auto-disengaged. Per-edit typecheck re-enabled. Run your build gate now.\"}"
fi
