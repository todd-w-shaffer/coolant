#!/bin/bash
set -euo pipefail
# SessionStart hook: one-time preflight checks for common misconfigurations.
# Scans test runner configs for missing .claude/ worktree exclusions.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin for cwd + session_id + source.
input=$(cat)
session_id=$(printf '%s' "$input" | _json_field session_id)
hook_source=$(printf '%s' "$input" | _json_field source)

# ALWAYS re-stamp the session sidecar — and do it BEFORE any event
# emission. The sidecar (read by _reconcile_counter and the Go tailer)
# is the canonical "which session is thermo / the awk pass scoped to."
# A resumed or cleared session keeps its session_id but restarts the
# process; without an unconditional re-stamp, the sidecar stays pinned
# to whatever session last cold-started — often a dead transient — and
# thermo filters out the live session's agents.
if [ -n "$session_id" ]; then
  printf '%s\n' "$session_id" > "$COOLANT_SESSION_FILE"
  export COOLANT_SESSION_ID="$session_id"
fi

# Everything below is cold-start-only. On resume/clear (and compact, were
# it ever matched) the process re-attaches to an EXISTING session whose
# agents and per-session review-gate ledger may still be live in the
# machine-wide thermo and the shared per-user JSONL bus. counter.reset is
# a global signal — the Go live model purges ALL active agent records on
# it — so re-firing it on resume/clear would wipe a concurrent session's
# agents; and truncating the audit logs would clobber an in-flight
# session's review gate. Only a true cold start owns a fresh epoch.
case "$hook_source" in
  resume|clear|compact)
    exit 0
    ;;
esac

# Lifecycle anchor BEFORE counter epoch: aggregator's session.start case
# folds the lifecycle map; consumers that fold events in JSONL order see
# the lifecycle anchor before the counter epoch resets. (Aggregator's
# counter.reset case is a no-op — the two events are not redundant.)
if [ -n "$session_id" ]; then
  coolant_event '"event":"session.start","session_id":"'"$session_id"'"'
fi

# Reset agent counter epoch — new session starts fresh
coolant_event '"event":"counter.reset"'

# Truncate the degraded-write counter — bash fallback line in coolant_event
# accumulates one newline per torn-write incident. Without per-session
# truncation the aggregator's `wc -l` reports cumulative-since-install
# instead of "this session," confusing per-session diagnostics.
: > "$COOLANT_DEGRADED_COUNT"

# Truncate the review-gate audit log so the pre-commit gate is scoped
# per-session. Same pattern as $COOLANT_DEGRADED_COUNT above.
: > "$COOLANT_REVIEW_AUDIT"

cwd=$(echo "$input" | _json_field cwd)
if [ -z "$cwd" ]; then
  exit 0
fi

warnings=""

# Check test configs for missing .claude worktree exclusion
check_worktree_exclude() {
  local config="$1" runner="$2"
  if [ -f "$config" ] && ! grep -q '\.claude' "$config"; then
    warnings="${warnings}[coolant] ${runner} config missing .claude/ exclusion — test discovery may multiply across agent worktrees. Add .claude to your exclude patterns.\n"
    coolant_event '"event":"preflight.warn","check":"worktree_exclude","runner":"'"$runner"'","config":"'"$config"'"'
  fi
}

check_worktree_exclude "$cwd/vitest.config.ts" "vitest"
check_worktree_exclude "$cwd/vitest.config.js" "vitest"
check_worktree_exclude "$cwd/vitest.config.mts" "vitest"
check_worktree_exclude "$cwd/jest.config.js" "jest"
check_worktree_exclude "$cwd/jest.config.ts" "jest"
check_worktree_exclude "$cwd/jest.config.mjs" "jest"

if [ -n "$warnings" ]; then
  safe_warnings=$(_json_escape "$(printf '%b' "$warnings")")
  printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$safe_warnings"
fi
