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

@test "_json_escape escapes newlines and tabs" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local input=$'line1\nline2\ttab'
  local result
  result=$(_json_escape "$input")

  [[ "$result" == *'\n'* ]]
  [[ "$result" == *'\t'* ]]
}

@test "_nested_command extracts tool_input.command" {
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(echo '{"tool_name":"Bash","tool_input":{"command":"tsc --noEmit","description":"typecheck"},"hook_event_name":"PreToolUse"}' | _nested_command)

  [ "$result" = "tsc --noEmit" ]
}

# ── _reconcile_counter ─────────────────────────────────────

@test "reconcile corrects counter when JSONL shows fewer agents" {
  echo "5" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:02Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:03Z","event":"agent.stop","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:04Z","event":"agent.stop","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile skips when no JSONL file exists" {
  echo "3" > "$COOLANT_COUNTER"
  rm -f "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(_reconcile_counter)
  [ "$result" = "3" ]
}

@test "reconcile floors at zero" {
  echo "2" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.stop","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "0" ]
}

@test "reconcile leaves counter alone when it matches JSONL" {
  echo "2" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(_reconcile_counter)
  [ "$result" = "2" ]
  # Counter file unchanged — no reconciliation log
  ! grep -q "reconciled" "$COOLANT_LOG"
}

@test "reconcile ignores non-agent events in JSONL" {
  echo "5" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"gate.suppress","command":"tsc"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:02Z","event":"parallel.engaged"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile counts only events after last counter.reset" {
  echo "5" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:02Z","event":"counter.reset"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:03Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile uses last of multiple counter.reset markers" {
  echo "5" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"counter.reset"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:02Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:03Z","event":"counter.reset"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:04Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile returns zero when counter.reset is last line" {
  echo "3" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"counter.reset"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  local result
  result=$(_reconcile_counter)
  [ "$result" = "0" ]
  [ "$(cat "$COOLANT_COUNTER")" = "0" ]
}

@test "reconcile counts mixed pre-versioning and schema:1 events" {
  echo "0" > "$COOLANT_COUNTER"
  # Pre-versioning shape (no schema field) — historical events still in file.
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.stop","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  # Post-deploy shape (schema:1) — same events should fold identically.
  printf '{"ts":"2025-01-01T00:00:02Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:03Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  # 3 starts - 1 stop = 2 active across both eras.
  [ "$(cat "$COOLANT_COUNTER")" = "2" ]
}

@test "reconcile filters by session_id" {
  echo "0" > "$COOLANT_COUNTER"
  # 3 starts in s1, 2 starts in s2; with sid=s1, only s1 events count.
  printf '{"ts":"2025-01-01T00:00:00Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:02Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:03Z","schema":1,"event":"agent.start","session_id":"s2"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:04Z","schema":1,"event":"agent.start","session_id":"s2"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "3" ]
}

@test "reconcile drops empty session_id agent events when sid set" {
  echo "0" > "$COOLANT_COUNTER"
  # Pre-deploy lines without session_id should NOT count when sid is configured.
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile counts all when sid unset (degraded fallback)" {
  unset COOLANT_SESSION_ID
  rm -f "$COOLANT_SESSION_FILE"
  echo "0" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s2"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "2" ]
}

@test "reconcile reads sid from sidecar when env unset" {
  unset COOLANT_SESSION_ID
  echo "s1" > "$COOLANT_SESSION_FILE"
  echo "0" > "$COOLANT_COUNTER"
  printf '{"ts":"2025-01-01T00:00:00Z","event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  printf '{"ts":"2025-01-01T00:00:01Z","event":"agent.start","session_id":"s2"}\n' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}

@test "reconcile ignores event substrings inside other fields" {
  echo "0" > "$COOLANT_COUNTER"
  # Real agent.start.
  printf '{"ts":"2025-01-01T00:00:00Z","schema":1,"event":"agent.start","session_id":"s1"}\n' >> "$COOLANT_EVENTS"
  # Pathological line: a future or hand-crafted event leaks the literal
  # bytes "event":"agent.start" inside another field's value, unescaped
  # (followed by a space, not a JSON value-closer). The naive grep
  # treats this as a second agent.start; the anchored awk pattern
  # `"event":"agent\.start"[,}]` only matches when followed by , or },
  # i.e. the actual end of the event field's value.
  echo '{"ts":"2025-01-01T00:00:01Z","schema":1,"event":"gate.cap","leak":"foo "event":"agent.start" bar"}' >> "$COOLANT_EVENTS"
  source "$PROJECT_ROOT/scripts/common.sh"
  _reconcile_counter > /dev/null
  [ "$(cat "$COOLANT_COUNTER")" = "1" ]
}
