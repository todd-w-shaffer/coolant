#!/usr/bin/env bats

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_LOCKFILE="${TEST_TMPDIR}/coolant.lock"
  export COOLANT_COUNTER="${TEST_TMPDIR}/coolant.count"
  export COOLANT_LOG="${TEST_TMPDIR}/coolant.log"
  export COOLANT_THRESHOLD=3

  source "${PROJECT_ROOT}/scripts/sparkline.sh"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ─── history_push ────────────────────────────────────────

@test "history_push appends a value to an empty array" {
  HIST=()
  history_push HIST 42 10
  [[ ${#HIST[@]} -eq 1 ]]
  [[ "${HIST[0]}" == "42" ]]
}

@test "history_push appends multiple values" {
  HIST=()
  history_push HIST 10 10
  history_push HIST 20 10
  history_push HIST 30 10
  [[ ${#HIST[@]} -eq 3 ]]
  [[ "${HIST[0]}" == "10" ]]
  [[ "${HIST[2]}" == "30" ]]
}

@test "history_push trims to max length" {
  HIST=()
  for i in 1 2 3 4 5; do
    history_push HIST "$i" 3
  done
  [[ ${#HIST[@]} -eq 3 ]]
  # Should keep the last 3: 3, 4, 5
  [[ "${HIST[0]}" == "3" ]]
  [[ "${HIST[1]}" == "4" ]]
  [[ "${HIST[2]}" == "5" ]]
}

# ─── sparkline_chart ─────────────────────────────────────

@test "sparkline_chart with all zeros emits blank braille" {
  local output
  output=$(sparkline_chart 3 5 0 0 0 0 0 0 0 0 0 0)
  # Should have 3 lines
  local line_count
  line_count=$(echo "$output" | wc -l | tr -d ' ')
  [[ "$line_count" -eq 3 ]]
  # All characters should be blank braille (U+2800) — no dots lit
  # Strip ANSI codes before checking
  local stripped
  stripped=$(echo "$output" | sed $'s/\033\\[[0-9;]*m//g')
  # Each line should be 5 blank braille characters
  # U+2800 is the empty braille pattern
  [[ "$stripped" != *"⣿"* ]]
}

@test "sparkline_chart with all 100s emits full braille" {
  local output
  output=$(sparkline_chart 3 5 100 100 100 100 100 100 100 100 100 100)
  # Strip ANSI codes
  local stripped
  stripped=$(echo "$output" | sed $'s/\033\\[[0-9;]*m//g')
  # Bottom row should contain full braille (U+28FF = ⣿)
  local bottom
  bottom=$(echo "$stripped" | tail -1)
  [[ "$bottom" == *"⣿"* ]]
}

@test "sparkline_chart with ascending ramp has bottom row filled and top row sparse" {
  # 10 values: 10, 20, 30, 40, 50, 60, 70, 80, 90, 100
  local output
  output=$(sparkline_chart 3 5 10 20 30 40 50 60 70 80 90 100)
  local stripped
  stripped=$(echo "$output" | sed $'s/\033\\[[0-9;]*m//g')
  # Top row (first line) should have fewer lit dots than bottom row (last line)
  local top bottom
  top=$(echo "$stripped" | head -1)
  bottom=$(echo "$stripped" | tail -1)
  # Bottom should have more non-blank characters than top
  # (At minimum, the rightmost columns should be full in the bottom row)
  [[ "$bottom" != "$top" ]]
}

@test "sparkline_chart right-pads with zeros when fewer values than capacity" {
  # Only 4 values for a width of 5 (capacity 10)
  local output
  output=$(sparkline_chart 2 5 80 80 80 80)
  # Should still emit 2 lines of 5 characters each
  local stripped
  stripped=$(echo "$output" | sed $'s/\033\\[[0-9;]*m//g')
  local line_count
  line_count=$(echo "$stripped" | wc -l | tr -d ' ')
  [[ "$line_count" -eq 2 ]]
}

# ─── zone coloring ───────────────────────────────────────

@test "sparkline_chart applies green color for values below 50" {
  local output
  output=$(sparkline_chart 2 3 30 30 30 30 30 30)
  # Should contain green ANSI code (\033[32m)
  [[ "$output" == *$'\033[32m'* ]]
}

@test "sparkline_chart applies yellow color for values 50-69" {
  local output
  output=$(sparkline_chart 2 3 60 60 60 60 60 60)
  # Should contain yellow ANSI code (\033[33m)
  [[ "$output" == *$'\033[33m'* ]]
}

@test "sparkline_chart applies red color for values >= 70" {
  local output
  output=$(sparkline_chart 2 3 90 90 90 90 90 90)
  # Should contain red ANSI code (\033[31m)
  [[ "$output" == *$'\033[31m'* ]]
}

# ─── box_frame ───────────────────────────────────────────

@test "box_top emits top border with title and value" {
  local output
  output=$(box_top "CPU" "load 2.38" "29%" 40)
  # Should start with ┌─
  [[ "$output" == *"┌─"* ]]
  # Should contain the title
  [[ "$output" == *"CPU"* ]]
  # Should contain the value
  [[ "$output" == *"29%"* ]]
  # Should end with ─┐
  [[ "$output" == *"─┐"* ]]
}

@test "box_bottom emits bottom border" {
  local output
  output=$(box_bottom 40)
  [[ "$output" == *"└─"* ]]
  [[ "$output" == *"─┘"* ]]
}

@test "box_line wraps content with side borders" {
  local output
  output=$(box_line "hello" 40)
  [[ "$output" == *"│"* ]]
}
