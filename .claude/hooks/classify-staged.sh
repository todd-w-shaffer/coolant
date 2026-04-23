#!/bin/bash
set -euo pipefail
# Claude Code PreToolUse hook: block `git commit` calls that would stage
# private content per scripts/classify.sh::classify_paths.
#
# Matcher is tool-name-only ("Bash"); we filter to git-commit invocations
# inside this script. Decision JSON on stdout, exit 0 — same idiom as
# scripts/gate.sh.
#
# Convenience layer only — depends on Claude Code's PreToolUse JSON shape
# (`tool_name`, `tool_input.command`, `cwd`). If that shape changes the
# hook silently no-ops; .githooks/pre-push is the authoritative belt and
# still catches anything that slips through.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck disable=SC1091
source "$REPO_ROOT/scripts/common.sh"
# shellcheck disable=SC1091
source "$REPO_ROOT/scripts/classify.sh"

input=$(cat)

if [[ "$input" != *'"tool_name"'*'"Bash"'* ]]; then
  exit 0
fi

command=$(echo "$input" | _nested_command)
# _nested_command's regex stops at the first unescaped quote, so
# `git commit -m "fix \"bug\""` returns truncated. Safe here: we need the
# command prefix and flag tokens, not the message body.

# Strip leading env assignments and binary path; collapse whitespace in
# pure bash (no fork). Avoids reaching `tr -s` on every Bash call Claude
# makes; most never match the git-commit case.
while [[ "$command" == *=* && "${command%% *}" == *=* ]]; do
  command="${command#* }"
done
first="${command%% *}"
first="${first##*/}"
rest="${command#* }"
if [ "$first" = "$rest" ]; then
  command="$first"
else
  command="$first $rest"
fi
while [[ "$command" == *"  "* ]]; do
  command="${command//  / }"
done

case "$command" in
  "git commit"|"git commit "*|"git -c "*"commit"*) : ;;
  *) exit 0 ;;
esac

# cwd field from the payload is authoritative; fall back to REPO_ROOT.
cwd=$(echo "$input" | _json_field cwd)
if [ -n "$cwd" ] && { [ -d "$cwd/.git" ] || [ -f "$cwd/.git" ]; }; then
  cd "$cwd"
else
  cd "$REPO_ROOT" 2>/dev/null || exit 0
fi

# Word-boundary -a / --all detection (a substring match would false-
# positive on `git commit -m "fix -a bug"`).
is_all=0
case " $command " in
  *" -a "*|*" --all "*|*" -am "*|*" -am=*"*|*" --all="*|*" -a="*)
    is_all=1 ;;
esac

tmp="${TMPDIR:-/tmp}/coolant-classify-staged.$$"
trap 'rm -f "$tmp"' EXIT
: > "$tmp"
git diff -z --cached --name-only --diff-filter=ACMR >> "$tmp" 2>/dev/null || true
if [ "$is_all" -eq 1 ]; then
  git diff -z --name-only --diff-filter=ACMR >> "$tmp" 2>/dev/null || true
fi
if [ ! -s "$tmp" ]; then
  exit 0
fi

set --
while IFS= read -r -d '' path; do
  set -- "$@" "$path"
done < "$tmp"

if [ "$#" -eq 0 ]; then
  exit 0
fi

set +e
classify_stderr=$(classify_paths "$@" 2>&1 1>/dev/null)
classify_rc=$?
set -e

if [ "$classify_rc" -eq 0 ]; then
  exit 0
fi

# Compact the multi-line terminal stanzas into one line per blocked path
# for the JSON permissionDecisionReason (agent context saver).
reason=$(
  printf '%s\n' "$classify_stderr" | awk '
    /^\[coolant-classify\] BLOCKED:/ {
      sub(/^\[coolant-classify\] BLOCKED: /, "")
      path = $0
      getline rule_line; sub(/^  rule: /, "", rule_line)
      printf "BLOCKED %s [%s] — move to ../thermal-enterprise/%s or extend .githooks/allowlist.txt (prior commit)\n", path, rule_line, path
    }
    /^\[coolant-classify\] [0-9]+ path\(s\) blocked\./ {
      sub(/^\[coolant-classify\] /, "")
      printf "%s Bypass: git commit --no-verify (deliberate).\n", $0
    }
  '
)

escaped=$(_json_escape "$reason")
printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"%s"}}\n' "$escaped"
exit 0
