#!/usr/bin/env bats

load test_helper

HOOK="$PROJECT_ROOT/.claude/hooks/enforce-spec-to-ship-reviews.sh"

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_REVIEW_AUDIT="${TEST_TMPDIR}/coolant-review-audit.jsonl"
  export COOLANT_GATE_THRESHOLD_LINES=200

  REPO="$TEST_TMPDIR/repo"
  git init -q -b main "$REPO"
  cd "$REPO"
  git config user.email "test@coolant.local"
  git config user.name "test"
  echo "baseline" > README.md
  git add README.md
  git commit -q -m "baseline"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# Append N audit entries of the given kind.
seed_audit() {
  local kind="$1" n="$2"
  mkdir -p "$(dirname "$COOLANT_REVIEW_AUDIT")"
  for _ in $(seq 1 "$n"); do
    printf '{"ts":"2026-04-25T00:00:00Z","kind":"%s","session_id":"s1"}\n' "$kind" >> "$COOLANT_REVIEW_AUDIT"
  done
}

stage_substantive() {
  for i in $(seq 1 250); do echo "line $i" >> bigfile.go; done
  git add bigfile.go
}

stage_trivial() {
  echo "tiny" >> README.md
  git add README.md
}

@test "gate allows non-commit Bash calls" {
  echo "$(make_pre_tool_use Bash 'ls -la' "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "gate allows trivial commits without review evidence" {
  stage_trivial
  echo "$(make_pre_tool_use Bash 'git commit -m "fix typo"' "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "gate blocks substantive commit with no review evidence" {
  stage_substantive
  local payload status=0
  payload=$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")
  out=$(echo "$payload" | "$HOOK" 2>&1) || status=$?
  [ "$status" -eq 2 ]
  [[ "$out" == *"simplify"* ]] || [[ "$out" == *"observations"* ]]
}

@test "gate blocks substantive commit with partial review evidence" {
  stage_substantive
  seed_audit "simplify-reuse" 1
  seed_audit "simplify-quality" 1
  seed_audit "simplify-efficiency" 1
  local payload status=0
  payload=$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")
  out=$(echo "$payload" | "$HOOK" 2>&1) || status=$?
  [ "$status" -eq 2 ]
  [[ "$out" == *"observations"* ]]
}

@test "gate allows substantive commit with full review evidence" {
  stage_substantive
  seed_audit "simplify-reuse" 1
  seed_audit "simplify-quality" 1
  seed_audit "simplify-efficiency" 1
  seed_audit "observations-ci" 1
  seed_audit "observations-static" 1
  echo "$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "gate honors [skip-review] trailer in commit body" {
  stage_substantive
  local cmd
  cmd=$(printf 'git commit -m "Land big feature\n\nThis is a corrective patch.\n\n[skip-review]"')
  echo "$(make_pre_tool_use Bash "$cmd" "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}

# `git commit -F path` is opaque (message lives in a file we can't safely
# interpret). v1 scope: pass through, don't block.
@test "gate passes through -F file commits" {
  stage_substantive
  echo "$(make_pre_tool_use Bash 'git commit -F /tmp/msg.txt' "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}

@test "gate counts unique simplify kinds, not duplicate marker spam" {
  stage_substantive
  seed_audit "simplify-reuse" 5
  seed_audit "observations-ci" 1
  seed_audit "observations-static" 1
  local payload status=0
  payload=$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")
  out=$(echo "$payload" | "$HOOK" 2>&1) || status=$?
  [ "$status" -eq 2 ]
  [[ "$out" == *"simplify"* ]]
}

@test "gate ignores typo'd kind names (allowlist not regex)" {
  stage_substantive
  # 5 typo'd kinds wouldn't pass a regex /^simplify-/ + /^observations-/
  # gate but pass the allowlist gate cleanly: still 0 valid kinds.
  seed_audit "simplify-typo" 5
  seed_audit "observations-typo" 5
  local payload status=0
  payload=$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")
  out=$(echo "$payload" | "$HOOK" 2>&1) || status=$?
  [ "$status" -eq 2 ]
}

@test "audit hook + gate hook end-to-end integration" {
  # Bridge the seam: drive the AUDIT hook with five real review-shaped
  # prompts, then run the gate hook. A rename of any kind string in
  # audit-review-agents.sh that's not mirrored in enforce-spec-to-ship-
  # reviews.sh's allowlist would break this test — both files' isolated
  # tests would still pass.
  local audit_hook="$PROJECT_ROOT/.claude/hooks/audit-review-agents.sh"
  echo "$(make_pre_tool_use_agent "You're a Code Reuse Review agent")"  | "$audit_hook"
  echo "$(make_pre_tool_use_agent "You're a Code Quality reviewer")"     | "$audit_hook"
  echo "$(make_pre_tool_use_agent "You're an Efficiency Review agent")"  | "$audit_hook"
  echo "$(make_pre_tool_use_agent "You're a CI Safety Net reviewer")"    | "$audit_hook"
  echo "$(make_pre_tool_use_agent "Static Codebase Health reviewer")"    | "$audit_hook"

  stage_substantive
  echo "$(make_pre_tool_use Bash 'git commit -m "Land big feature"' "$REPO")" | "$HOOK"
  [ "$?" -eq 0 ]
}
