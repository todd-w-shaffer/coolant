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

@test "toggle status uses reconciled count" {
  touch "$COOLANT_LOCKFILE"
  echo "5" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  run bash "$PROJECT_ROOT/scripts/toggle.sh" status
  [[ "${output}" == *"2 agents tracked"* ]]
}

@test "toggle off emits counter.reset event" {
  touch "$COOLANT_LOCKFILE"
  echo "3" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/toggle.sh" off
  grep -q '"event":"counter.reset"' "$COOLANT_EVENTS"
}

@test "toggle off then on then status shows zero agents" {
  # Simulate prior activity
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s2"}\n' >> "$COOLANT_EVENTS"
  echo "2" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  # Off resets, on re-engages, status should show 0 (not stale 2)
  bash "$PROJECT_ROOT/scripts/toggle.sh" off > /dev/null
  bash "$PROJECT_ROOT/scripts/toggle.sh" on > /dev/null
  run bash "$PROJECT_ROOT/scripts/toggle.sh" status
  [[ "${output}" == *"0 agents tracked"* ]]
}
