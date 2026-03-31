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
  export COOLANT_THRESHOLD=3
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}
