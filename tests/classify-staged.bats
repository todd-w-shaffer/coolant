#!/usr/bin/env bats
#
# Integration tests for .claude/hooks/classify-staged.sh, the Claude Code
# PreToolUse hook that blocks `git commit` calls staging private content.

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  setup_git_tmprepo
}

# Helper: pipe a synthetic PreToolUse JSON into the hook and capture stdout.
run_hook() {
  make_pre_tool_use Bash "$1" "$PWD" | .claude/hooks/classify-staged.sh
}

# Helper: stage a single file with given content and return its path.
stage_file() {
  local path="$1" content="${2:-x}"
  mkdir -p "$(dirname "$path")"
  echo "$content" > "$path"
  git add "$path"
}

# ── non-git / non-Bash passthrough ───────────────────────────────────────

@test "classify-staged ignores non-Bash tool_name" {
  local out
  out=$(printf '{"tool_name":"Edit","tool_input":{"file_path":"x"}}' \
        | .claude/hooks/classify-staged.sh) || true
  [ -z "$out" ]
}

@test "classify-staged ignores non-git-commit Bash commands" {
  local out
  out=$(run_hook "ls -la") || true
  [ -z "$out" ]
}

# ── allow path ───────────────────────────────────────────────────────────

@test "classify-staged allows a commit with only allowlisted paths" {
  stage_file docs/go-design.md
  local out
  out=$(run_hook "git commit -m msg") || true
  [ -z "$out" ]
}

# ── deny path ────────────────────────────────────────────────────────────

@test "classify-staged denies a commit with a blocked path" {
  stage_file docs/backlog/new-spec.md
  local out
  out=$(run_hook "git commit -m msg")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
  [[ "$out" == *"docs/backlog/new-spec.md"* ]]
}

@test "classify-staged reason field is valid JSON" {
  stage_file docs/backlog/new-spec.md
  local out
  out=$(run_hook 'git commit -m "the message"')
  echo "$out" | python3 -c 'import sys, json; json.load(sys.stdin)'
}

# ── -a / --all flag handling ─────────────────────────────────────────────

@test "classify-staged catches -a (commit all) modifications" {
  mkdir -p docs/backlog
  echo "initial" > docs/backlog/tracked-blocked.md
  git add docs/backlog/tracked-blocked.md
  git commit -q -m "initial blocked file"
  # Modify without staging.
  echo "change" >> docs/backlog/tracked-blocked.md
  local out
  out=$(run_hook "git commit -am msg")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
  [[ "$out" == *"tracked-blocked.md"* ]]
}

@test "classify-staged ignores -a inside a quoted commit message" {
  # _nested_command truncates at the first unescaped quote, so the -m "..."
  # body never reaches the -a detector; the test asserts the conservative
  # outcome (no false-positive deny when nothing is actually staged-all).
  local out
  out=$(run_hook 'git commit -m "fix -a bug"') || true
  [ -z "$out" ]
}

# ── retroactive-policy: existing main-tracked blocked files don't trip ──

@test "classify-staged does not flag pre-existing tracked blocklisted files" {
  mkdir -p docs/enterprise-otel
  echo "old content" > docs/enterprise-otel/spec.md
  git add docs/enterprise-otel/spec.md
  git commit -q -m "already-tracked blocked file"
  # Stage an unrelated allowed file.
  mkdir -p docs
  echo "unrelated" > docs/go-design.md
  git add docs/go-design.md
  local out
  out=$(run_hook "git commit -m msg") || true
  [ -z "$out" ]
}

# ── edge: empty staged set ───────────────────────────────────────────────

@test "classify-staged allows empty staged set" {
  local out
  out=$(run_hook "git commit --allow-empty -m empty") || true
  [ -z "$out" ]
}

# ── command-form matching ────────────────────────────────────────────────

@test "classify-staged matches git -c foo=bar commit variant" {
  stage_file docs/backlog/new.md
  local out
  out=$(run_hook "git -c user.email=x@y commit -m msg")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
}

@test "classify-staged matches absolute-path git" {
  stage_file docs/backlog/new.md
  local out
  out=$(run_hook "/usr/local/bin/git commit -m msg")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
}

@test "classify-staged matches leading env assignment" {
  stage_file docs/backlog/new.md
  local out
  out=$(run_hook "GIT_EDITOR=vi git commit")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
}

# ── rename that introduces a blocked path ────────────────────────────────

@test "classify-staged matches rename that introduces a blocked path" {
  mkdir -p docs
  echo "content" > docs/go-design.md
  git add docs/go-design.md
  git commit -q -m "seed allowed file"
  # Rename to a blocked path.
  mkdir -p docs/backlog
  git mv docs/go-design.md docs/backlog/now-blocked.md
  local out
  out=$(run_hook "git commit -m rename")
  [[ "$out" == *'"permissionDecision":"deny"'* ]]
  [[ "$out" == *"docs/backlog/now-blocked.md"* ]]
}
