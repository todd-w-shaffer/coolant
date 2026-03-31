#!/bin/bash
# Toggle coolant parallel mode on/off/status
# Usage: toggle.sh [on|off|status]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

case "${1:-status}" in
  on)
    touch "$COOLANT_LOCKFILE"
    coolant_log "parallel mode ON (manual)"
    echo "Coolant: parallel mode ON"
    echo "  - Per-edit typecheck hooks suppressed"
    echo "  - Cap concurrent agents at 4"
    echo "  - Run npm run check + build after agents finish"
    ;;
  off)
    rm -f "$COOLANT_LOCKFILE" "$COOLANT_COUNTER"
    coolant_log "parallel mode OFF (manual)"
    echo "Coolant: parallel mode OFF"
    echo "  - Per-edit typecheck hooks re-enabled"
    ;;
  status)
    if [ -f "$COOLANT_LOCKFILE" ]; then
      count=$(cat "$COOLANT_COUNTER" 2>/dev/null || echo "0")
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
