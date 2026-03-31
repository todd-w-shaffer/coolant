#!/usr/bin/env bats

load test_helper

@test "parallel-gate suppresses when lockfile present" {
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/parallel-gate.sh"
  [ "$status" -eq 0 ]
  [[ "${output}" == *"typecheck suppressed"* ]]
}

@test "parallel-gate logs suppression" {
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/parallel-gate.sh"
  grep -q "typecheck suppressed" "$COOLANT_LOG"
}

@test "parallel-gate exits cleanly without lockfile" {
  run bash "$PROJECT_ROOT/scripts/parallel-gate.sh"
  [ "$status" -eq 0 ]
  [ -z "${output}" ]
}

@test "parallel-gate does not log without lockfile" {
  run bash "$PROJECT_ROOT/scripts/parallel-gate.sh"
  [ ! -f "$COOLANT_LOG" ]
}
