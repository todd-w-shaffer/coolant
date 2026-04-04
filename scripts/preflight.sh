#!/bin/bash
set -euo pipefail
# SessionStart hook: one-time preflight checks for common misconfigurations.
# Scans test runner configs for missing .claude/ worktree exclusions.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# Read hook stdin for cwd
input=$(cat)
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
