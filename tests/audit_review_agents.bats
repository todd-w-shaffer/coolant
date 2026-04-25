#!/usr/bin/env bats

load test_helper

HOOK="$PROJECT_ROOT/.claude/hooks/audit-review-agents.sh"

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_REVIEW_AUDIT="${TEST_TMPDIR}/coolant-review-audit.jsonl"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

@test "audit logs simplify-reuse for Code Reuse Review prompt" {
  echo "$(make_pre_tool_use_agent "You're a Code Reuse Review agent. Look at the diff.")" | "$HOOK"
  grep -q '"kind":"simplify-reuse"' "$COOLANT_REVIEW_AUDIT"
}

@test "audit logs simplify-quality for Code Quality Review prompt" {
  echo "$(make_pre_tool_use_agent "You're a Code Quality reviewer.")" | "$HOOK"
  grep -q '"kind":"simplify-quality"' "$COOLANT_REVIEW_AUDIT"
}

@test "audit logs simplify-efficiency for Efficiency Review prompt" {
  echo "$(make_pre_tool_use_agent "You're an Efficiency Review agent.")" | "$HOOK"
  grep -q '"kind":"simplify-efficiency"' "$COOLANT_REVIEW_AUDIT"
}

@test "audit logs observations-ci for CI Safety Net prompt" {
  echo "$(make_pre_tool_use_agent "You're a CI Safety Net reviewer for the diff.")" | "$HOOK"
  grep -q '"kind":"observations-ci"' "$COOLANT_REVIEW_AUDIT"
}

@test "audit logs observations-static for Static Codebase Health prompt" {
  echo "$(make_pre_tool_use_agent "Static Codebase Health reviewer.")" | "$HOOK"
  grep -q '"kind":"observations-static"' "$COOLANT_REVIEW_AUDIT"
}

@test "audit ignores non-review prompts" {
  echo "$(make_pre_tool_use_agent "Find files matching pattern X.")" | "$HOOK"
  [ ! -f "$COOLANT_REVIEW_AUDIT" ] || ! grep -q '"kind":' "$COOLANT_REVIEW_AUDIT"
}

@test "audit ignores non-Agent tools" {
  echo '{"tool_name":"Bash","tool_input":{"command":"ls"}}' | "$HOOK"
  [ ! -f "$COOLANT_REVIEW_AUDIT" ] || ! grep -q '"kind":' "$COOLANT_REVIEW_AUDIT"
}

@test "audit appends multiple entries on repeated calls" {
  echo "$(make_pre_tool_use_agent "Code Reuse Review")" | "$HOOK"
  echo "$(make_pre_tool_use_agent "Code Quality reviewer")" | "$HOOK"
  echo "$(make_pre_tool_use_agent "Efficiency Review")" | "$HOOK"
  [ "$(wc -l < "$COOLANT_REVIEW_AUDIT")" -eq 3 ]
}

@test "audit exits zero (non-blocking)" {
  echo "$(make_pre_tool_use_agent "Code Reuse Review")" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "audit handles malformed JSON gracefully" {
  echo "not json {{{" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "audit JSON-escapes session_id (injection-safe)" {
  # Session id with a literal quote and newline shouldn't be able to
  # break out of the JSON string and corrupt the audit log.
  local payload='{"session_id":"ab\"c\nd","tool_name":"Agent","tool_input":{"prompt":"Code Reuse Review"}}'
  echo "$payload" | "$HOOK"
  # Audit line still parses as JSON via python3.
  command -v python3 >/dev/null || skip "python3 not available"
  local line
  line=$(tail -1 "$COOLANT_REVIEW_AUDIT")
  printf '%s' "$line" | python3 -c 'import sys,json; json.loads(sys.stdin.read())'
}

@test "COOLANT_REVIEW_AUDIT default resolves via common.sh" {
  # Unset the test override and confirm the default path constant
  # in common.sh kicks in — guards against a regression that broke
  # the path-defaulting expansion.
  unset COOLANT_REVIEW_AUDIT
  local fake_tmp="${TEST_TMPDIR}/fake-tmpdir/"
  mkdir -p "$fake_tmp"
  TMPDIR="$fake_tmp" echo "$(make_pre_tool_use_agent "Code Reuse Review")" | TMPDIR="$fake_tmp" "$HOOK"
  # Default path: ${TMPDIR}coolant-${USER}.review-audit.jsonl
  [ -f "${fake_tmp}coolant-${USER}.review-audit.jsonl" ]
}
