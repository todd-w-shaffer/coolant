#!/usr/bin/env bats

load test_helper

@test "common.sh sets default config paths under TMPDIR" {
  # Unset overrides so defaults kick in
  unset COOLANT_LOCKFILE COOLANT_COUNTER COOLANT_LOG COOLANT_THRESHOLD COOLANT_EVENTS
  source "$PROJECT_ROOT/scripts/common.sh"

  local dir="${TMPDIR:-/tmp/}"
  [ "$COOLANT_LOCKFILE" = "${dir}coolant-${USER}.lock" ]
  [ "$COOLANT_COUNTER" = "${dir}coolant-agents-${USER}.count" ]
  [ "$COOLANT_LOG" = "${dir}coolant-${USER}.log" ]
  [ "$COOLANT_EVENTS" = "${dir}coolant-${USER}.events.jsonl" ]
  [ "$COOLANT_THRESHOLD" = "3" ]
}

@test "common.sh respects environment overrides" {
  export COOLANT_LOCKFILE="/custom/lock"
  export COOLANT_COUNTER="/custom/count"
  export COOLANT_LOG="/custom/log"
  export COOLANT_THRESHOLD=5
  source "$PROJECT_ROOT/scripts/common.sh"

  [ "$COOLANT_LOCKFILE" = "/custom/lock" ]
  [ "$COOLANT_COUNTER" = "/custom/count" ]
  [ "$COOLANT_LOG" = "/custom/log" ]
  [ "$COOLANT_THRESHOLD" = "5" ]
}

@test "coolant_log appends timestamped entry to log file" {
  source "$PROJECT_ROOT/scripts/common.sh"
  coolant_log "test message"

  [ -f "$COOLANT_LOG" ]
  grep -q "test message" "$COOLANT_LOG"
}

@test "common.sh sets default COOLANT_EVENTS path under TMPDIR" {
  unset COOLANT_EVENTS
  source "$PROJECT_ROOT/scripts/common.sh"

  local dir="${TMPDIR:-/tmp/}"
  [ "$COOLANT_EVENTS" = "${dir}coolant-${USER}.events.jsonl" ]
}

@test "COOLANT_EVENTS respects environment override" {
  export COOLANT_EVENTS="/custom/events.jsonl"
  source "$PROJECT_ROOT/scripts/common.sh"

  [ "$COOLANT_EVENTS" = "/custom/events.jsonl" ]
}

@test "coolant_event writes JSONL line with ts field" {
  source "$PROJECT_ROOT/scripts/common.sh"
  coolant_event '"event":"test.ping","msg":"hello"'

  [ -f "$COOLANT_EVENTS" ]
  grep -q '"ts":' "$COOLANT_EVENTS"
  grep -q '"event":"test.ping"' "$COOLANT_EVENTS"
}

@test "coolant_event appends without clobbering" {
  source "$PROJECT_ROOT/scripts/common.sh"
  coolant_event '"event":"first"'
  coolant_event '"event":"second"'

  local count
  count=$(wc -l < "$COOLANT_EVENTS")
  [ "$count" -eq 2 ]
}

@test "_json_field extracts top-level string" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(echo '{"session_id":"abc-123","tool_name":"Bash"}' | _json_field session_id)

  [ "$result" = "abc-123" ]
}

@test "_json_field extracts second field" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(echo '{"session_id":"abc","tool_name":"Bash"}' | _json_field tool_name)

  [ "$result" = "Bash" ]
}

@test "_json_escape escapes backslashes and quotes" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(_json_escape 'hello "world" foo\bar')

  [ "$result" = 'hello \"world\" foo\\bar' ]
}

@test "_nested_command extracts tool_input.command" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(echo '{"tool_name":"Bash","tool_input":{"command":"tsc --noEmit","description":"typecheck"},"hook_event_name":"PreToolUse"}' | _nested_command)

  [ "$result" = "tsc --noEmit" ]
}
