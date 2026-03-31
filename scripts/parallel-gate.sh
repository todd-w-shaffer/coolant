#!/bin/bash
# PostToolUse hook: suppress tsc when coolant parallel mode is active
# If the lockfile exists, skip typecheck and emit a system message.
# If not, exit cleanly and let the project's normal hooks handle it.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

if [ -f "$COOLANT_LOCKFILE" ]; then
  coolant_log "typecheck suppressed (Edit/Write)"
  echo '{"systemMessage":"[coolant] parallel mode active — per-edit typecheck suppressed. Validate after agents finish."}'
  exit 0
fi

# No lockfile = normal mode. Exit cleanly, don't interfere.
exit 0
