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

@test "agent-start JSONL includes cwd" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/myproject"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"cwd":"/Users/dev/myproject"' "$COOLANT_EVENTS"
}

@test "agent-start JSONL omits permission_mode" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","permission_mode":"auto"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  ! grep -q '"permission_mode"' "$COOLANT_EVENTS"
}

@test "agent-start JSONL escapes spaces in cwd" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/my project"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"cwd":"/Users/dev/my project"' "$COOLANT_EVENTS"
  # Verify it's valid JSON by checking the whole line parses
  python3 -c "import json,sys; json.loads(sys.stdin.readline())" < "$COOLANT_EVENTS"
}

@test "agent-start JSONL includes project basename" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/apps/coolant"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"project":"coolant"' "$COOLANT_EVENTS"
}

@test "agent-start project strips .claude-worktrees suffix" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/apps/coolant/.claude-worktrees/segment-readout"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"project":"coolant"' "$COOLANT_EVENTS"
  ! grep -q '"project":"segment-readout"' "$COOLANT_EVENTS"
}

@test "agent-start project strips nested worktree paths" {
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/apps/coolant/.claude-worktrees/feature/deep/nesting"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  grep -q '"project":"coolant"' "$COOLANT_EVENTS"
}

@test "agent-start records start ts in agent-starts state file" {
  echo '{"session_id":"s1","agent_id":"abc123","agent_type":"Explore","cwd":"/Users/dev/apps/coolant"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  [ -f "$COOLANT_AGENT_STARTS" ]
  grep -q "^abc123	" "$COOLANT_AGENT_STARTS"
}

@test "agent-start records distinct state entries for concurrent agents" {
  echo '{"session_id":"s1","agent_id":"first","agent_type":"Explore"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  echo '{"session_id":"s1","agent_id":"second","agent_type":"Plan"}' | \
    bash "$PROJECT_ROOT/scripts/agent-start.sh" > /dev/null
  [ "$(wc -l < "$COOLANT_AGENT_STARTS" | tr -d ' ')" = "2" ]
  grep -q "^first	"  "$COOLANT_AGENT_STARTS"
  grep -q "^second	" "$COOLANT_AGENT_STARTS"
}
