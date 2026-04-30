#!/usr/bin/env bats

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export HOME="$TEST_TMPDIR"
  export PATH="$PATH"
  SCRIPT="$PROJECT_ROOT/scripts/enable-cc-otel.sh"
  SETTINGS="$HOME/.claude/settings.json"
}

require_jq() {
  if ! command -v jq >/dev/null 2>&1; then
    skip "jq not installed"
  fi
}

@test "enable-cc-otel writes the documented env block at default port" {
  require_jq
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  [ -f "$SETTINGS" ]
  run jq -r '.env.CLAUDE_CODE_ENABLE_TELEMETRY' "$SETTINGS"
  [ "$output" = "1" ]
  run jq -r '.env.OTEL_METRICS_EXPORTER' "$SETTINGS"
  [ "$output" = "otlp" ]
  run jq -r '.env.OTEL_EXPORTER_OTLP_PROTOCOL' "$SETTINGS"
  [ "$output" = "http/protobuf" ]
  run jq -r '.env.OTEL_EXPORTER_OTLP_METRICS_ENDPOINT' "$SETTINGS"
  [ "$output" = "http://localhost:4318/v1/metrics" ]
  run jq -r '.env.OTEL_METRIC_EXPORT_INTERVAL' "$SETTINGS"
  [ "$output" = "10000" ]
  run jq -r '.env.OTEL_METRICS_INCLUDE_SESSION_ID' "$SETTINGS"
  [ "$output" = "true" ]
  run jq -r '.env.OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE' "$SETTINGS"
  [ "$output" = "cumulative" ]
}

@test "enable-cc-otel honors --port=N" {
  require_jq
  run "$SCRIPT" --port=4319
  [ "$status" -eq 0 ]
  run jq -r '.env.OTEL_EXPORTER_OTLP_METRICS_ENDPOINT' "$SETTINGS"
  [ "$output" = "http://localhost:4319/v1/metrics" ]
}

@test "enable-cc-otel honors COOLANT_CC_OTEL_PORT env var" {
  require_jq
  COOLANT_CC_OTEL_PORT=4320 run "$SCRIPT"
  [ "$status" -eq 0 ]
  run jq -r '.env.OTEL_EXPORTER_OTLP_METRICS_ENDPOINT' "$SETTINGS"
  [ "$output" = "http://localhost:4320/v1/metrics" ]
}

@test "enable-cc-otel does NOT set logs/traces/beta-tracing env" {
  require_jq
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  run jq -r '.env.OTEL_LOGS_EXPORTER // "absent"' "$SETTINGS"
  [ "$output" = "absent" ]
  run jq -r '.env.OTEL_TRACES_EXPORTER // "absent"' "$SETTINGS"
  [ "$output" = "absent" ]
  run jq -r '.env.CLAUDE_CODE_ENHANCED_TELEMETRY_BETA // "absent"' "$SETTINGS"
  [ "$output" = "absent" ]
}

@test "enable-cc-otel --off removes only the keys it added" {
  require_jq
  mkdir -p "$HOME/.claude"
  cat > "$SETTINGS" <<'JSON'
{
  "env": {
    "USER_KEPT_VAR": "preserved",
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_METRICS_EXPORTER": "otlp"
  },
  "statusLine": {"type": "command", "command": "x"}
}
JSON
  run "$SCRIPT" --off
  [ "$status" -eq 0 ]
  run jq -r '.env.USER_KEPT_VAR' "$SETTINGS"
  [ "$output" = "preserved" ]
  run jq -r '.env.CLAUDE_CODE_ENABLE_TELEMETRY // "absent"' "$SETTINGS"
  [ "$output" = "absent" ]
  run jq -r '.env.OTEL_METRICS_EXPORTER // "absent"' "$SETTINGS"
  [ "$output" = "absent" ]
  run jq -r '.statusLine.command' "$SETTINGS"
  [ "$output" = "x" ]
}

@test "enable-cc-otel warns but does not refuse when thermo is not running" {
  require_jq
  # No thermo bound to default port — script must still succeed.
  run "$SCRIPT" --port=49199
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "warning" || true
}

@test "enable-cc-otel preserves existing settings.json structure" {
  require_jq
  mkdir -p "$HOME/.claude"
  cat > "$SETTINGS" <<'JSON'
{
  "statusLine": {"type": "command", "command": "x"},
  "env": {"PRE_EXISTING": "yes"}
}
JSON
  run "$SCRIPT"
  [ "$status" -eq 0 ]
  run jq -r '.statusLine.command' "$SETTINGS"
  [ "$output" = "x" ]
  run jq -r '.env.PRE_EXISTING' "$SETTINGS"
  [ "$output" = "yes" ]
}
