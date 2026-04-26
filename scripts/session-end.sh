#!/bin/bash
set -euo pipefail
# SessionEnd hook: emit a session.end lifecycle event so the aggregator
# can compute longest_session_s from explicit start/end timestamps
# instead of inferred-from-first-agent. Kill -9 / SIGKILL / OS reboot
# do NOT fire SessionEnd; that gap is closed by the staleness fallback
# in stats/aggregator.go::Snapshot.
#
# No counter mutation, no coolant_lock acquisition (coolant_event
# handles its own lock).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

input=$(cat)
session_id=$(printf '%s' "$input" | _json_field session_id)
if [ -z "$session_id" ]; then
  exit 0
fi

coolant_event '"event":"session.end","session_id":"'"$session_id"'"'
