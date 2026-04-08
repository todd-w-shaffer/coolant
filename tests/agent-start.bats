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

@test "agent-start does NOT create lockfile at threshold" {
  echo "2" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "agent-start does not engage below threshold" {
  echo "0" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "agent-start does not warn when already engaged" {
  echo "2" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [[ "${output}" != *"systemMessage"* ]]
}

@test "agent-start emits JSONL agent.start event" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"event":"agent.start"' "$COOLANT_EVENTS"
}

@test "agent-start JSONL includes agent metadata" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Plan"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"agent_type":"Plan"' "$COOLANT_EVENTS"
  grep -q '"session_id":"s1"' "$COOLANT_EVENTS"
}

@test "agent-start does NOT emit parallel.engaged at threshold" {
  echo "2" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  ! grep -q '"event":"parallel.engaged"' "$COOLANT_EVENTS"
}

@test "agent-start emits warning systemMessage at threshold" {
  echo "2" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [[ "${output}" == *"systemMessage"* ]]
  [[ "${output}" == *"/coolant"* ]]
  [[ "${output}" != *"auto-engaged"* ]]
}

@test "agent-start warning includes agent count" {
  echo "2" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [[ "${output}" == *"3 agents"* ]]
}

@test "agent-start warns again for each spawn above threshold" {
  echo "3" > "$COOLANT_COUNTER"
  export COOLANT_THRESHOLD=3
  run bash "$PROJECT_ROOT/scripts/agent-start.sh"
  [[ "${output}" == *"systemMessage"* ]]
  [[ "${output}" == *"4 agents"* ]]
}
