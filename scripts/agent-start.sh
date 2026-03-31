#!/bin/bash
# SubagentStart hook: increment agent counter, auto-engage parallel mode at threshold

LOCKFILE="${COOLANT_LOCKFILE:-/tmp/coolant-${USER}.lock}"
COUNTER="${COOLANT_COUNTER:-/tmp/coolant-agents-${USER}.count}"
THRESHOLD="${COOLANT_THRESHOLD:-3}"

# Atomic-ish increment (good enough for our use case)
current=$(cat "$COUNTER" 2>/dev/null || echo "0")
next=$((current + 1))
echo "$next" > "$COUNTER"

# Auto-engage at threshold
if [ "$next" -ge "$THRESHOLD" ] && [ ! -f "$LOCKFILE" ]; then
  touch "$LOCKFILE"
  echo "{\"systemMessage\":\"[coolant] ${next} agents active (threshold: ${THRESHOLD}) — parallel mode auto-engaged. Per-edit typecheck suppressed.\"}"
fi
