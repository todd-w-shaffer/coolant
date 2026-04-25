#!/bin/bash
# PreToolUse hook on the Agent tool. Detects review-shaped agent prompts
# (the kind /simplify and /observations spawn) and appends a record to
# $COOLANT_REVIEW_AUDIT. Non-blocking observer.
#
# Skill tool itself doesn't fire PreToolUse hooks (per Claude Code docs),
# so we gate on the side effect — Agent invocations whose prompts match
# reviewer-shape markers. Honest skill runs produce these naturally;
# mental-model substitution does not.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../scripts" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common.sh"

input=$(cat 2>/dev/null) || exit 0
[ -z "$input" ] && exit 0

case "$input" in
  *'"tool_name":"Agent"'*) ;;
  *) exit 0 ;;
esac

# Loose patterns — Reviewer (the noun form agents use in prompt text)
# is matched by the same `*[Rr]eview*` glob since "Reviewer" contains
# "Review". Single pass.
classify() {
  case "$1" in
    *[Cc]ode' '[Rr]euse*[Rr]eview*)         printf 'simplify-reuse' ;;
    *[Cc]ode' '[Qq]uality*[Rr]eview*)       printf 'simplify-quality' ;;
    *[Ee]fficiency*[Rr]eview*)              printf 'simplify-efficiency' ;;
    *[Cc][Ii]' '[Ss]afety' '[Nn]et*)        printf 'observations-ci' ;;
    *[Ss]tatic' '[Cc]odebase' '[Hh]ealth*)  printf 'observations-static' ;;
  esac
}

kind=$(classify "$input")
[ -z "$kind" ] && exit 0

ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
session_id=$(printf '%s' "$input" | _json_field session_id)
# JSON-escape session_id before re-emission — an unescaped quote or
# newline would corrupt the audit log and break the gate's awk parser.
session_id=$(_json_escape "$session_id")

mkdir -p "$(dirname "$COOLANT_REVIEW_AUDIT")" 2>/dev/null || true
printf '{"ts":"%s","kind":"%s","session_id":"%s"}\n' "$ts" "$kind" "$session_id" >> "$COOLANT_REVIEW_AUDIT"
exit 0
