#!/usr/bin/env bats

load test_helper

@test "agent-stop decrements counter" {
  echo "3" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "$COOLANT_COUNTER")" = "2" ]
}

@test "agent-stop floors counter at zero" {
  echo "0" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ "$(cat "$COOLANT_COUNTER")" = "0" ]
}

@test "agent-stop logs event" {
  echo "1" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  grep -q "agent stopped" "$COOLANT_LOG"
}

@test "agent-stop auto-disengages when counter reaches zero" {
  echo "1" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "agent-stop emits systemMessage on auto-disengage" {
  echo "1" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  [[ "${output}" == *"auto-disengaged"* ]]
}

@test "agent-stop does not disengage when agents remain" {
  echo "3" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ -f "$COOLANT_LOCKFILE" ]
}
