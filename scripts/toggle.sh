#!/bin/bash
set -euo pipefail
# Toggle coolant parallel mode on/off/status
# Usage: toggle.sh [on|off|status]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

case "${1:-status}" in
  on)
    touch "$COOLANT_LOCKFILE"
    coolant_log "parallel mode ON (manual)"
    echo "Coolant: parallel mode ON"
    echo "  - Build tools and type checkers suppressed"
    echo "  - Test runner concurrency capped per agent count"
    echo "  - Run validation after agents finish, then /coolant off"
    ;;
  off)
    coolant_event '"event":"counter.reset"'
    rm -f "$COOLANT_LOCKFILE" "$COOLANT_COUNTER"
    coolant_log "parallel mode OFF (manual)"
    echo "Coolant: parallel mode OFF"
    echo "  - Per-edit typecheck hooks re-enabled"
    ;;
  status)
    if [ -f "$COOLANT_LOCKFILE" ]; then
      count=$(_reconcile_counter)
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
