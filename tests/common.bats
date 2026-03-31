#!/usr/bin/env bats

load test_helper

@test "common.sh sets default config paths" {
  # Unset overrides so defaults kick in
  unset COOLANT_LOCKFILE COOLANT_COUNTER COOLANT_LOG COOLANT_THRESHOLD
  source "$PROJECT_ROOT/scripts/common.sh"

  [ "$COOLANT_LOCKFILE" = "/tmp/coolant-${USER}.lock" ]
  [ "$COOLANT_COUNTER" = "/tmp/coolant-agents-${USER}.count" ]
  [ "$COOLANT_LOG" = "/tmp/coolant-${USER}.log" ]
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
