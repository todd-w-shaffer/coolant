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

@test "agent-stop emits JSONL agent.stop event" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"event":"agent.stop"' "$COOLANT_EVENTS"
}

@test "agent-stop JSONL includes agent count" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Plan"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"agent_count":1' "$COOLANT_EVENTS"
}

@test "agent-stop emits parallel.disengaged JSONL on auto-disengage" {
  echo "1" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"event":"parallel.disengaged"' "$COOLANT_EVENTS"
}

@test "agent-stop JSONL includes cwd" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/myproject"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"cwd":"/Users/dev/myproject"' "$COOLANT_EVENTS"
}

@test "agent-stop JSONL includes permission_mode" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Plan","permission_mode":"plan"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"permission_mode":"plan"' "$COOLANT_EVENTS"
}

@test "agent-stop JSONL includes project basename" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/apps/shootsfilm"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"project":"shootsfilm"' "$COOLANT_EVENTS"
}

@test "agent-stop JSONL includes agent_transcript_path" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","agent_transcript_path":"/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"transcript_path":"/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl"' "$COOLANT_EVENTS"
}
