#!/usr/bin/env bats

load test_helper

# Default fixture stdin — populated agent_type so the empty-agent_type
# defense (CC #44971 bug) doesn't intercept tests that aren't probing it.
_AS_STDIN='{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}'

@test "agent-stop decrements counter" {
  echo "3" > "$COOLANT_COUNTER"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ "$status" -eq 0 ]
  [ "$(cat "$COOLANT_COUNTER")" = "2" ]
}

@test "agent-stop floors counter at zero" {
  echo "0" > "$COOLANT_COUNTER"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ "$(cat "$COOLANT_COUNTER")" = "0" ]
}

@test "agent-stop logs event" {
  echo "1" > "$COOLANT_COUNTER"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
  grep -q "agent stopped" "$COOLANT_LOG"
}

@test "agent-stop auto-disengages when counter reaches zero" {
  echo "1" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
  [ ! -f "$COOLANT_LOCKFILE" ]
}

@test "agent-stop emits systemMessage on auto-disengage" {
  echo "1" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
  [[ "${output}" == *"auto-disengaged"* ]]
}

@test "agent-stop does not disengage when agents remain" {
  echo "3" > "$COOLANT_COUNTER"
  touch "$COOLANT_LOCKFILE"
  run bash -c 'echo "$1" | bash "$2"' _ "$_AS_STDIN" "$PROJECT_ROOT/scripts/agent-stop.sh"
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

@test "agent-stop emits duration_s when start was recorded" {
  echo "1" > "$COOLANT_COUNTER"
  printf 'abc123\t%s\n' "$(($(date +%s) - 2))" > "$COOLANT_AGENT_STARTS"
  echo '{"session_id":"s1","agent_id":"abc123","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -qE '"duration_s":[0-9]+' "$COOLANT_EVENTS"
}

@test "agent-stop duration_s is a non-negative integer" {
  echo "1" > "$COOLANT_COUNTER"
  printf 'abc123\t%s\n' "$(($(date +%s) - 5))" > "$COOLANT_AGENT_STARTS"
  echo '{"session_id":"s1","agent_id":"abc123","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  local duration
  duration=$(grep -oE '"duration_s":[0-9]+' "$COOLANT_EVENTS" | head -1 | cut -d: -f2)
  [ -n "$duration" ]
  [ "$duration" -ge 5 ]
  [ "$duration" -lt 60 ]
}

@test "agent-stop omits duration_s for orphan stops (no recorded start)" {
  echo "1" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"orphan","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  ! grep -q '"duration_s"' "$COOLANT_EVENTS"
}

@test "agent-stop removes matched line from state file" {
  echo "1" > "$COOLANT_COUNTER"
  printf 'abc123\t%s\n' "$(($(date +%s) - 2))" > "$COOLANT_AGENT_STARTS"
  echo '{"session_id":"s1","agent_id":"abc123","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  ! grep -q "^abc123	" "$COOLANT_AGENT_STARTS" 2>/dev/null
}

@test "agent-stop preserves other agents' state entries" {
  echo "2" > "$COOLANT_COUNTER"
  {
    printf 'alpha\t%s\n' "$(($(date +%s) - 10))"
    printf 'beta\t%s\n'  "$(($(date +%s) - 5))"
  } > "$COOLANT_AGENT_STARTS"
  echo '{"session_id":"s1","agent_id":"alpha","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  ! grep -q "^alpha	" "$COOLANT_AGENT_STARTS"
  grep   -q "^beta	"  "$COOLANT_AGENT_STARTS"
}

@test "agent-stop prunes state entries older than 24h" {
  echo "1" > "$COOLANT_COUNTER"
  local now=$(date +%s)
  {
    printf 'stale\t%s\n' "$((now - 86500))"  # >24h old
    printf 'fresh\t%s\n' "$((now - 60))"     # recent, unrelated
  } > "$COOLANT_AGENT_STARTS"
  echo '{"session_id":"s1","agent_id":"trigger","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  ! grep -q "^stale	" "$COOLANT_AGENT_STARTS"
  grep   -q "^fresh	" "$COOLANT_AGENT_STARTS"
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

@test "agent-stop drops empty agent_type (CC orphan-stop bug defense)" {
  echo "3" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"orphan","agent_type":""}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  # No JSONL line emitted for the dropped event.
  ! grep -q '"agent_id":"orphan"' "$COOLANT_EVENTS"
  # Counter unchanged — neither decrement nor underflow.
  [ "$(cat "$COOLANT_COUNTER")" = "3" ]
  # Drop is logged for diagnostic visibility.
  grep -q "dropped empty agent_type" "$COOLANT_LOG"
}

@test "agent-stop JSONL includes agent_transcript_path" {
  echo "2" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","agent_transcript_path":"/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl"}' | \
    bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null
  grep -q '"transcript_path":"/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl"' "$COOLANT_EVENTS"
}
