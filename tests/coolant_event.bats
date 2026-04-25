#!/usr/bin/env bats

load test_helper

# ── envelope shape ─────────────────────────────────────────

@test "coolant_event injects schema:1 into envelope" {
  source "$PROJECT_ROOT/scripts/common.sh"
  coolant_event '"event":"smoke"'

  [ -f "$COOLANT_EVENTS" ]
  local line
  line=$(tail -1 "$COOLANT_EVENTS")
  # Schema field present plus existing ts and caller body preserved.
  [[ "$line" == *'"schema":1'* ]] \
    && [[ "$line" == *'"ts":'* ]] \
    && [[ "$line" == *'"event":"smoke"'* ]]
}

# ── parallel write lock ────────────────────────────────────

@test "parallel writes never splice — every line valid JSON" {
  source "$PROJECT_ROOT/scripts/common.sh"

  # macOS regular-file appends are atomic for small writes but splice
  # for large ones. Use 50KB filler × 20 concurrent workers — a probe
  # without the lock confirms ~50% of lines splice at this scale.
  local big
  big=$(printf '%.0sx' {1..50000})

  local n=20 i
  local pids=()
  for i in $(seq 1 "$n"); do
    coolant_event '"event":"big.payload","i":'"$i"',"filler":"'"$big"'"' &
    pids+=("$!")
  done
  for pid in "${pids[@]}"; do
    wait "$pid"
  done

  # Exactly N lines.
  local count
  count=$(wc -l < "$COOLANT_EVENTS")
  [ "$count" -eq "$n" ] || { echo "expected $n lines, got $count" >&2; return 1; }

  # Every line valid JSON. Use python (always present on macOS) for
  # the parse rather than installing jq.
  while IFS= read -r line; do
    printf '%s' "$line" | python3 -c 'import sys,json; json.loads(sys.stdin.read())' \
      || { echo "spliced/invalid line: ${line:0:120}..." >&2; return 1; }
  done < "$COOLANT_EVENTS"
}

# ── lock-failure fallback ──────────────────────────────────

@test "lock-failure fallback emits event, increments degraded counter, logs" {
  source "$PROJECT_ROOT/scripts/common.sh"

  # Pre-create the mutex dir so coolant_lock can't acquire it. The
  # internal _COOLANT_MUTEX path is derived from COOLANT_COUNTER.
  mkdir -p "${COOLANT_COUNTER}.lock"

  coolant_event '"event":"smoke"'

  # Mutex should still be held (we created it pre-test) — clean up so
  # later tests / teardown don't trip on it. coolant_unlock is a no-op
  # when the dir doesn't exist.
  rmdir "${COOLANT_COUNTER}.lock" 2>/dev/null || true

  # 1. Event still landed in JSONL.
  [ -f "$COOLANT_EVENTS" ]
  grep -q '"event":"smoke"' "$COOLANT_EVENTS"

  # 2. Degraded counter file accumulated one newline.
  [ -f "$COOLANT_DEGRADED_COUNT" ]
  local degraded_lines
  degraded_lines=$(wc -l < "$COOLANT_DEGRADED_COUNT")
  [ "$degraded_lines" -eq 1 ]

  # 3. Human-readable log carries the degradation note.
  grep -q "lock failed" "$COOLANT_LOG"
}

@test "degraded counter accumulates across repeated lock failures" {
  source "$PROJECT_ROOT/scripts/common.sh"

  local i
  for i in 1 2 3; do
    mkdir -p "${COOLANT_COUNTER}.lock"
    coolant_event '"event":"smoke","i":'"$i"
    rmdir "${COOLANT_COUNTER}.lock" 2>/dev/null || true
  done

  local degraded_lines
  degraded_lines=$(wc -l < "$COOLANT_DEGRADED_COUNT")
  [ "$degraded_lines" -eq 3 ]
}
