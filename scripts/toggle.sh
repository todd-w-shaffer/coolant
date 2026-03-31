#!/bin/bash
# Toggle coolant parallel mode on/off/status
# Usage: toggle.sh [on|off|status]

LOCKFILE="${COOLANT_LOCKFILE:-/tmp/coolant-${USER}.lock}"
COUNTER="${COOLANT_COUNTER:-/tmp/coolant-agents-${USER}.count}"

case "${1:-status}" in
  on)
    touch "$LOCKFILE"
    echo "Coolant: parallel mode ON"
    echo "  - Per-edit typecheck hooks suppressed"
    echo "  - Cap concurrent agents at 4"
    echo "  - Run npm run check + build after agents finish"
    ;;
  off)
    rm -f "$LOCKFILE" "$COUNTER"
    echo "Coolant: parallel mode OFF"
    echo "  - Per-edit typecheck hooks re-enabled"
    ;;
  status)
    if [ -f "$LOCKFILE" ]; then
      count=$(cat "$COUNTER" 2>/dev/null || echo "0")
      echo "Coolant: parallel mode ON (${count} agents tracked)"
    else
      echo "Coolant: parallel mode OFF"
    fi
    ;;
  *)
    echo "Usage: toggle.sh [on|off|status]"
    exit 1
    ;;
esac
