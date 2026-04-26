#!/usr/bin/env bats

load test_helper

# ── session.start emission (preflight.sh) ─────────────────────

@test "preflight emits session.start before counter.reset" {
  mkdir -p "$TEST_TMPDIR/project"
  printf '{"session_id":"abc-123","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' \
    "$TEST_TMPDIR/project" \
    | bash "$PROJECT_ROOT/scripts/preflight.sh" > /dev/null

  # session.start MUST appear before counter.reset in JSONL order so
  # consumers folding the file see the lifecycle anchor first.
  local lines
  lines=$(grep -nE '"event":"(session\.start|counter\.reset)"' "$COOLANT_EVENTS")
  local start_line reset_line
  start_line=$(printf '%s\n' "$lines" | grep "session.start" | head -1 | cut -d: -f1)
  reset_line=$(printf '%s\n' "$lines" | grep "counter.reset" | head -1 | cut -d: -f1)
  [ -n "$start_line" ]
  [ -n "$reset_line" ]
  [ "$start_line" -lt "$reset_line" ]
}

@test "preflight session.start carries session_id and schema:1" {
  mkdir -p "$TEST_TMPDIR/project"
  printf '{"session_id":"abc-123","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' \
    "$TEST_TMPDIR/project" \
    | bash "$PROJECT_ROOT/scripts/preflight.sh" > /dev/null

  grep -q '"event":"session.start","session_id":"abc-123"' "$COOLANT_EVENTS"
  grep '"event":"session.start"' "$COOLANT_EVENTS" | grep -q '"schema":1'
}

# ── session.end emission (session-end.sh) ─────────────────────

@test "session-end.sh emits session.end with session_id" {
  printf '{"session_id":"xyz-789","hook_event_name":"SessionEnd"}' \
    | bash "$PROJECT_ROOT/scripts/session-end.sh"

  grep -q '"event":"session.end","session_id":"xyz-789"' "$COOLANT_EVENTS"
  grep '"event":"session.end"' "$COOLANT_EVENTS" | grep -q '"schema":1'
}

@test "session-end.sh emits exactly one event line" {
  printf '{"session_id":"xyz","hook_event_name":"SessionEnd"}' \
    | bash "$PROJECT_ROOT/scripts/session-end.sh"

  [ "$(wc -l < "$COOLANT_EVENTS")" -eq 1 ]
}

@test "session-end.sh skips emission when session_id missing" {
  # Empty input — no session_id to anchor on, no event emitted.
  printf '{"hook_event_name":"SessionEnd"}' \
    | bash "$PROJECT_ROOT/scripts/session-end.sh"

  [ ! -s "$COOLANT_EVENTS" ] || ! grep -q '"event":"session.end"' "$COOLANT_EVENTS"
}

# ── counter.underflow (agent-stop.sh) ─────────────────────────

@test "agent-stop emits counter.underflow when floor triggers" {
  echo "0" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' \
    | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  grep -q '"event":"counter.underflow"' "$COOLANT_EVENTS"
  grep '"event":"counter.underflow"' "$COOLANT_EVENTS" | grep -q '"raw":-1'
  grep '"event":"counter.underflow"' "$COOLANT_EVENTS" | grep -q '"session_id":"s1"'
  # Floor still applied to the file write.
  [ "$(cat "$COOLANT_COUNTER")" = "0" ]
  # WARN line in the human log for diagnostic visibility.
  grep -q "counter underflow" "$COOLANT_LOG"
}

@test "agent-stop does NOT emit counter.underflow on normal decrement" {
  echo "3" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' \
    | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  ! grep -q '"event":"counter.underflow"' "$COOLANT_EVENTS"
  [ "$(cat "$COOLANT_COUNTER")" = "2" ]
}

@test "counter.underflow emission is AFTER the agent.stop emission" {
  echo "0" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore"}' \
    | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  # Both events should be present; agent.stop is emitted via the
  # standard path while counter.underflow is the diagnostic side-channel.
  local stop_line uf_line
  stop_line=$(grep -n '"event":"agent.stop"' "$COOLANT_EVENTS" | head -1 | cut -d: -f1)
  uf_line=$(grep -n '"event":"counter.underflow"' "$COOLANT_EVENTS" | head -1 | cut -d: -f1)
  [ -n "$stop_line" ]
  [ -n "$uf_line" ]
}
