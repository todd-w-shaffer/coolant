#!/usr/bin/env bats

load test_helper

# ── _parse_agent_telemetry helper ─────────────────────────────

@test "parse_agent_telemetry sums input/output tokens and counts tool_use" {
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "$PROJECT_ROOT/tests/fixtures/agent-transcript-typical.jsonl"
  # Typical fixture: 3 assistant messages with input_tokens 1500/2200/3300,
  # output_tokens 80/120/200, tool_use occurrences 1/2/0.
  [ "$_agent_tokens_in" = "7000" ]
  [ "$_agent_tokens_out" = "400" ]
  [ "$_agent_tool_call_count" = "3" ]
}

@test "parse_agent_telemetry returns zero on empty transcript" {
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "$PROJECT_ROOT/tests/fixtures/agent-transcript-empty.jsonl"
  # No assistant messages — parse succeeded but every counter is 0.
  # Distinct from parse-failure (empty out-vars).
  [ "$_agent_tokens_in" = "0" ]
  [ "$_agent_tokens_out" = "0" ]
  [ "$_agent_tool_call_count" = "0" ]
}

@test "parse_agent_telemetry returns empty on missing file" {
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "/no/such/file.jsonl"
  [ -z "$_agent_tokens_in" ]
  [ -z "$_agent_tokens_out" ]
  [ -z "$_agent_tool_call_count" ]
}

@test "parse_agent_telemetry returns empty on unset path" {
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry ""
  [ -z "$_agent_tokens_in" ]
  [ -z "$_agent_tokens_out" ]
  [ -z "$_agent_tool_call_count" ]
}

@test "parse_agent_telemetry tolerates malformed transcript" {
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "$PROJECT_ROOT/tests/fixtures/agent-transcript-malformed.jsonl"
  # Malformed fixture: line 1 user (no usage), line 2 truncated mid-output_tokens.
  # Partial parse: input_tokens=500 captured, output_tokens missing (no zero
  # injected because the parse pattern requires the full key:value match).
  # Pin the contract: the helper does NOT treat this as parse-failure; it
  # extracts what it can. Tool count is 0 (no tool_use seen).
  [ "$_agent_tokens_in" = "500" ]
  [ "$_agent_tokens_out" = "0" ]
  [ "$_agent_tool_call_count" = "0" ]
}

@test "parse_agent_telemetry counts each tool_use in a multi-tool message" {
  # Inline fixture: one message with three tool_use content items.
  cat > "$TEST_TMPDIR/transcript.jsonl" <<'EOF'
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Grep"},{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Edit"}],"usage":{"input_tokens":100,"output_tokens":50}}}
EOF
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "$TEST_TMPDIR/transcript.jsonl"
  [ "$_agent_tool_call_count" = "3" ]
  [ "$_agent_tokens_in" = "100" ]
  [ "$_agent_tokens_out" = "50" ]
}

# ── agent-stop emission integration ──────────────────────────

@test "agent-stop emits telemetry tail when transcript parses" {
  echo "1" > "$COOLANT_COUNTER"
  local payload
  payload=$(printf '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","agent_transcript_path":"%s"}' \
    "$PROJECT_ROOT/tests/fixtures/agent-transcript-typical.jsonl")
  echo "$payload" | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  grep -q '"event":"agent.stop"' "$COOLANT_EVENTS"
  grep -q '"tokens_in":7000' "$COOLANT_EVENTS"
  grep -q '"tokens_out":400' "$COOLANT_EVENTS"
  grep -q '"tool_call_count":3' "$COOLANT_EVENTS"
}

@test "agent-stop omits telemetry tail when transcript missing" {
  echo "1" > "$COOLANT_COUNTER"
  echo '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","agent_transcript_path":"/no/such/transcript.jsonl"}' \
    | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  grep -q '"event":"agent.stop"' "$COOLANT_EVENTS"
  ! grep -q '"tokens_in"' "$COOLANT_EVENTS"
  ! grep -q '"tokens_out"' "$COOLANT_EVENTS"
  ! grep -q '"tool_call_count"' "$COOLANT_EVENTS"
}

@test "agent-stop emits literal zeros for empty transcript" {
  echo "1" > "$COOLANT_COUNTER"
  local payload
  payload=$(printf '{"session_id":"s1","agent_id":"a1","agent_type":"Explore","agent_transcript_path":"%s"}' \
    "$PROJECT_ROOT/tests/fixtures/agent-transcript-empty.jsonl")
  echo "$payload" | bash "$PROJECT_ROOT/scripts/agent-stop.sh" > /dev/null

  # Empty transcript = parse succeeded with zero values; emission must
  # carry literal 0 to disambiguate from "parse failed."
  grep -q '"tokens_in":0' "$COOLANT_EVENTS"
  grep -q '"tokens_out":0' "$COOLANT_EVENTS"
  grep -q '"tool_call_count":0' "$COOLANT_EVENTS"
}

@test "parse_agent_telemetry dedupes iterations[] block input_tokens" {
  # Real Claude Code transcripts repeat input_tokens inside an
  # iterations[] array. The awk pattern uses match() which only
  # captures the FIRST occurrence per line, so the value adds once.
  cat > "$TEST_TMPDIR/transcript.jsonl" <<'EOF'
{"type":"assistant","message":{"role":"assistant","content":[],"usage":{"input_tokens":777,"output_tokens":40,"iterations":[{"input_tokens":777,"output_tokens":40},{"input_tokens":777,"output_tokens":40}]}}}
EOF
  source "$PROJECT_ROOT/scripts/common.sh"
  _parse_agent_telemetry "$TEST_TMPDIR/transcript.jsonl"
  [ "$_agent_tokens_in" = "777" ]
  [ "$_agent_tokens_out" = "40" ]
}
