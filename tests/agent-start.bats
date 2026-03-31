#!/usr/bin/env bats

load test_helper

@test "agent-start increments counter from 0" {
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "agent-start increments existing counter" {
  echo "2" > "$COOLANT_COUNTER"
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ "$(cat "$COOLANT_COUNTER")" = "3" ]
}

@test "agent-start logs event" {
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  grep -q "agent started" "$COOLANT_LOG"
}

@test "agent-start auto-engages at threshold" {
  echo "2" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ -f "$COOLANT_LOCKFILE" ]
}

@test "agent-start does not engage below threshold" {
  echo "0" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "agent-start skips auto-engage if already engaged" {
  echo "2" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  # Should not emit systemMessage if lockfile already existed
  [[ "${output}" != *"auto-engaged"* ]]
}
