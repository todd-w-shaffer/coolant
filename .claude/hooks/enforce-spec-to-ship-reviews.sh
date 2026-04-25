#!/bin/bash
# PreToolUse hook on Bash. When the command is `git commit` and the
# staged change is "substantive," require the audit log (populated by
# audit-review-agents.sh) to show enough distinct review kinds. Block
# (exit 2) with stderr if missing. Non-substantive commits and
# `[skip-review]`-trailered commits pass through.
#
# Belt-and-suspenders against the failure mode where the agent
# substitutes mental-model "self-review" for actual /simplify and
# /observations skill invocations.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../scripts" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/common.sh"

THRESHOLD_LINES="${COOLANT_GATE_THRESHOLD_LINES:-200}"
SIMPLIFY_MIN="${COOLANT_GATE_SIMPLIFY_MIN:-3}"
OBSERVATIONS_MIN="${COOLANT_GATE_OBSERVATIONS_MIN:-2}"

input=$(cat 2>/dev/null) || exit 0
[ -z "$input" ] && exit 0

case "$input" in
  *'"tool_name":"Bash"'*) ;;
  *) exit 0 ;;
esac

# Substring gating against raw JSON. We deliberately skip
# tool_input.command extraction here — the project's _nested_command
# regex doesn't handle escaped quotes inside `git commit -m "..."`,
# and presence/absence checks on the whole payload work for our gates.

case "$input" in
  *"git commit"*) ;;
  *) exit 0 ;;
esac

# Brackets in `case` patterns are character-class metacharacters; quote
# them so the literal trailer is matched.
case "$input" in
  *'[skip-review]'*) exit 0 ;;
esac

# `git commit -F path` / `--file` is opaque to us. Pass through.
case "$input" in
  *"git commit -F "*|*"git commit --file"*) exit 0 ;;
esac

cwd=$(printf '%s' "$input" | _json_field cwd)
[ -z "$cwd" ] && cwd=$(pwd)

added=$(cd "$cwd" 2>/dev/null && git diff --cached --numstat 2>/dev/null | awk '{sum += $1} END { print sum + 0 }')
[ -z "$added" ] && added=0
if [ "$added" -lt "$THRESHOLD_LINES" ]; then
  exit 0
fi

# Distinct-kind counting via explicit allowlist (typo'd kind names
# can't silently inflate the count).
have_simplify_kinds=0
have_observations_kinds=0
if [ -f "$COOLANT_REVIEW_AUDIT" ]; then
  read -r have_simplify_kinds have_observations_kinds < <(awk -F'"kind":"' '
    NF > 1 {
      k = $2; sub(/".*/, "", k)
      if (k == "simplify-reuse" || k == "simplify-quality" || k == "simplify-efficiency") seen_s[k] = 1
      else if (k == "observations-ci" || k == "observations-static") seen_o[k] = 1
    }
    END {
      ns = 0; for (x in seen_s) ns++
      no = 0; for (x in seen_o) no++
      print ns, no
    }
  ' "$COOLANT_REVIEW_AUDIT")
fi

missing=""
if [ "$have_simplify_kinds" -lt "$SIMPLIFY_MIN" ]; then
  missing="${missing}simplify (have ${have_simplify_kinds}/${SIMPLIFY_MIN} distinct kinds), "
fi
if [ "$have_observations_kinds" -lt "$OBSERVATIONS_MIN" ]; then
  missing="${missing}observations (have ${have_observations_kinds}/${OBSERVATIONS_MIN} distinct kinds), "
fi

if [ -n "$missing" ]; then
  cat >&2 <<EOF
coolant: pre-commit review gate blocked

This commit stages ${added} added lines (>= ${THRESHOLD_LINES} threshold)
but the session audit log shows missing review work: ${missing%, }

Run /simplify and /observations on the staged diff before committing.
Skill outputs populate the audit log automatically. Override only with
intent — append the literal trailer [skip-review] to the commit body
for corrective patches or doc-only changes the heuristic mis-fires on.

Audit log: ${COOLANT_REVIEW_AUDIT}
EOF
  exit 2
fi

exit 0
