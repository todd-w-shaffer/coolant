#!/usr/bin/env bash
# Shared test setup — isolates all coolant state to a temp directory
# so tests never touch the real /tmp/coolant-* files.

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_LOCKFILE="${TEST_TMPDIR}/coolant.lock"
  export COOLANT_COUNTER="${TEST_TMPDIR}/coolant.count"
  export COOLANT_LOG="${TEST_TMPDIR}/coolant.log"
  export COOLANT_EVENTS="${TEST_TMPDIR}/coolant.events.jsonl"
  export COOLANT_AGENT_STARTS="${TEST_TMPDIR}/coolant.agent-starts"
  export COOLANT_DEGRADED_COUNT="${TEST_TMPDIR}/coolant.degraded.count"
  export COOLANT_THRESHOLD=3
  export _COOLANT_NCPU=10
}

# Build a PreToolUse stdin JSON payload for testing gate.sh and
# classify-staged.sh.  $1=tool_name, $2=command string, $3=cwd (optional).
# Backslashes and quotes in command/cwd are escaped for valid JSON.
make_pre_tool_use() {
  local tool="$1" cmd="$2" cwd="${3:-}"
  cmd="${cmd//\\/\\\\}"; cmd="${cmd//\"/\\\"}"
  if [ -n "$cwd" ]; then
    cwd="${cwd//\\/\\\\}"; cwd="${cwd//\"/\\\"}"
    printf '{"session_id":"test-s","tool_name":"%s","tool_input":{"command":"%s","description":"test"},"hook_event_name":"PreToolUse","cwd":"%s"}' "$tool" "$cmd" "$cwd"
  else
    printf '{"session_id":"test-s","tool_name":"%s","tool_input":{"command":"%s","description":"test"},"hook_event_name":"PreToolUse"}' "$tool" "$cmd"
  fi
}

# Create a throwaway git repo at $TEST_TMPDIR/repo with the classification
# hook infrastructure wired up.  Data files (blocklist/allowlist) are
# copied so tests can mutate them; scripts and hooks are symlinked so tests
# run live code while CLASSIFY_LIB_DIR / REPO_ROOT resolve to the test
# repo (not $PROJECT_ROOT).
setup_git_tmprepo() {
  local repo="$TEST_TMPDIR/repo"
  git init -q -b main "$repo"
  cd "$repo"
  git config user.email "test@coolant.local"
  git config user.name "test"

  # Data files — real copies (tests may mutate; git-show-HEAD needs them
  # committed, not symlinked — git show on a symlink returns the target
  # string, not file content).
  mkdir -p .githooks
  cp "$PROJECT_ROOT/.githooks/blocklist.txt" .githooks/blocklist.txt
  cp "$PROJECT_ROOT/.githooks/allowlist.txt" .githooks/allowlist.txt

  # Scripts — symlinks so tests use live code.  CLASSIFY_LIB_DIR resolves
  # via dirname(BASH_SOURCE[0]) → $TEST_TMPDIR/repo/scripts, so data-file
  # lookups hit the test repo's .githooks/, not $PROJECT_ROOT's.
  mkdir -p scripts
  ln -s "$PROJECT_ROOT/scripts/classify.sh" scripts/classify.sh
  ln -s "$PROJECT_ROOT/scripts/common.sh"   scripts/common.sh

  # Hooks — symlinks for the same reason.
  mkdir -p .claude/hooks
  ln -s "$PROJECT_ROOT/.claude/hooks/classify-staged.sh" .claude/hooks/classify-staged.sh
  ln -s "$PROJECT_ROOT/.githooks/pre-push" .githooks/pre-push

  # Ensure exec bit (idempotent; lets bats work before install-hooks.sh).
  chmod +x "$PROJECT_ROOT/.githooks/pre-push" \
           "$PROJECT_ROOT/.claude/hooks/classify-staged.sh"

  # Initial commit with data files so `git show HEAD:.githooks/allowlist.txt`
  # works (the security-critical property).
  git add .githooks/blocklist.txt .githooks/allowlist.txt
  git commit -q -m "bootstrap: hook data files"
}

# Create a bare remote at $TEST_TMPDIR/remote.git and add it as origin.
make_bare_remote() {
  git init -q --bare "$TEST_TMPDIR/remote.git"
  git remote add origin "$TEST_TMPDIR/remote.git"
}

# Emit one line of git's pre-push stdin protocol.
# $1=local_ref, $2=local_sha, $3=remote_ref, $4=remote_sha
make_push_stdin() {
  printf '%s %s %s %s\n' "$1" "$2" "$3" "$4"
}

# Emit one schema:1 event via coolant_event. Sources common.sh on first
# call so callers (including bats subshells) don't have to remember.
# Body should be a JSON fragment with NO leading or trailing brace, e.g.:
#   emit_event '"event":"smoke","msg":"hello"'
emit_event() {
  if ! declare -F coolant_event > /dev/null; then
    # shellcheck disable=SC1091
    source "$PROJECT_ROOT/scripts/common.sh"
  fi
  coolant_event "$1"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}
