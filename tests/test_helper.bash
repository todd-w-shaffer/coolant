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
  export COOLANT_THRESHOLD=3
  export _COOLANT_NCPU=10
}

# Build a PreToolUse stdin JSON payload for testing gate.sh.
# $1=tool_name, $2=command string
make_pre_tool_use() {
  printf '{"session_id":"test-s","tool_name":"%s","tool_input":{"command":"%s","description":"test"},"hook_event_name":"PreToolUse"}' "$1" "$2"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}
