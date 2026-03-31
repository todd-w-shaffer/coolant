#!/usr/bin/env bats

load test_helper

@test "toggle on creates lockfile" {
  run bash "$PROJECT_ROOT/scripts/toggle.sh" on
  [ "$status" -eq 0 ]
  [ -f "$COOLANT_LOCKFILE" ]
}

@test "toggle on prints confirmation" {
  run bash "$PROJECT_ROOT/scripts/toggle.sh" on
  [[ "${output}" == *"parallel mode ON"* ]]
}

@test "toggle off removes lockfile" {
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/toggle.sh" off
  [ "$status" -eq 0 ]
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "toggle off removes counter file" {
  echo "3" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/toggle.sh" off
  [ ! -f "$COOLANT_COUNTER" ]
}

@test "toggle status reports OFF when no lockfile" {
  run bash "$PROJECT_ROOT/scripts/toggle.sh" status
  [ "$status" -eq 0 ]
  [[ "${output}" == *"parallel mode OFF"* ]]
}

@test "toggle status reports ON with agent count" {
  touch "$COOLANT_LOCKFILE"
  echo "2" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/toggle.sh" status
  [[ "${output}" == *"parallel mode ON (2 agents tracked)"* ]]
}

@test "toggle with no args defaults to status" {
  run bash "$PROJECT_ROOT/scripts/toggle.sh"
  [ "$status" -eq 0 ]
  [[ "${output}" == *"parallel mode OFF"* ]]
}

@test "toggle with invalid arg exits 1" {
  run bash "$PROJECT_ROOT/scripts/toggle.sh" bogus
  [ "$status" -eq 1 ]
  [[ "${output}" == *"Usage:"* ]]
}
