#!/usr/bin/env bats

# Statusline tests. The per-session statusline surface deliberately
# does NOT signal coolant install staleness — that's a system-wide
# concern owned by thermo's notification banner.
#
# Assertions must propagate their own failure with `|| return 1` — see
# the note above the has/lacks helpers.

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  TESTS_DIR="$(cd "$(dirname "${BATS_TEST_FILENAME}")" && pwd)"
  PROJECT_ROOT="$(cd "$TESTS_DIR/.." && pwd)"
  export TMPDIR="$TEST_TMPDIR/"
  export USER="testuser"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# bats does not apply `set -e` to these test bodies: a failing command
# mid-body is ignored and only the LAST command decides pass/fail. Every
# assertion therefore has to propagate its own failure with `|| return 1`
# — a bare `[[ ]]` on any line but the last is inert.
has() {
  case "$output" in
    *"$1"*) return 0 ;;
    *) echo "expected output to contain: $1" >&2; return 1 ;;
  esac
}
lacks() {
  case "$output" in
    *"$1"*) echo "expected output NOT to contain: $1" >&2; return 1 ;;
    *) return 0 ;;
  esac
}

# Render the statusline with $1 as stdin JSON, into $output/$status.
render() {
  run bash -c "printf '%s' '$1' | bash '$PROJECT_ROOT/claude-statusline/statusline.sh' 2>&1"
}

# Write a two-response transcript: 2000 cumulative input, 500 output.
# The interleaved user line has no usage and must not be counted.
make_transcript() {
  cat > "$1" <<'JSONL'
{"type":"assistant","message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":700,"output_tokens":50}}}
{"type":"user","message":{"content":"no usage here"}}
{"type":"assistant","message":{"usage":{"input_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":1000,"output_tokens":450}}}
JSONL
}

# ── version comment (read by scripts/upgrade.sh) ────

@test "statusline has VERSION comment" {
  grep -q '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh"
}

@test "VERSION comment is valid semver" {
  version=$(grep '^# VERSION:' "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1 | sed 's/# VERSION: *//')
  echo "$version" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'
}

# ── coolant-staleness signal belongs in thermo, not statusline ─────

@test "statusline output never shows the upgrade glyph" {
  # System-wide staleness (coolant install version drift) belongs in
  # thermo's notification banner, not the per-session statusline.
  # Even with a cache asserting a much newer remote version exists,
  # the statusline must not emit ⬆.
  cache="${TMPDIR}coolant-${USER}.latest-version"
  echo "99.0.0" > "$cache"

  render '{"context_window":{"used_percentage":0,"total_input_tokens":0,"total_output_tokens":0},"rate_limits":{"five_hour":{"used_percentage":0,"resets_at":0},"seven_day":{"used_percentage":0}},"cwd":"."}'
  lacks "⬆" || return 1
}

# ── renders cleanly with no subscription data ───────────────────────

@test "renders without printf errors when resets_at is absent" {
  # The countdown placeholder is the literal string '--:--'. Passed to
  # printf as a format it parses as an option and errors out. Bedrock
  # never sends rate_limits, so this fires on every single render.
  tp="$TEST_TMPDIR/t.jsonl"; make_transcript "$tp"

  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"transcript_path":"%s","cwd":"."}' "$tp")"

  [ "$status" -eq 0 ] || return 1
  lacks "invalid option" || return 1
  lacks "usage: printf" || return 1
}

# ── session token totals (Bedrock-safe) ─────────────────────────────
#
# The statusline JSON has NO session-cumulative token field.
# context_window.total_input_tokens/total_output_tokens are the *current
# context window* from the most recent API response — the output figure
# falls when a short reply follows a long one. True session totals are
# summed from the transcript at .transcript_path, which is provider
# agnostic and therefore works on Bedrock where rate_limits is absent.

@test "session totals are summed from the transcript, not the context window" {
  tp="$TEST_TMPDIR/t.jsonl"; make_transcript "$tp"

  # context_window carries deliberately different numbers. If the
  # statusline reads those instead of the transcript, 1000k appears.
  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":999999,"total_output_tokens":12345},"transcript_path":"%s","cwd":"."}' "$tp")"

  has "2k" || return 1
  lacks "1000k" || return 1
  lacks "12k" || return 1
}

@test "session output total accumulates across responses" {
  tp="$TEST_TMPDIR/t.jsonl"; make_transcript "$tp"

  # 50 + 450 = 500. The last response alone was 450 — asserting 500
  # proves accumulation rather than last-response-only.
  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":450},"transcript_path":"%s","cwd":"."}' "$tp")"

  has "500" || return 1
}

# ── adapts when rate_limits is absent (Bedrock, API-key auth) ────────

@test "omits the sesh and week bars when rate_limits is absent" {
  tp="$TEST_TMPDIR/t.jsonl"; make_transcript "$tp"

  # rate_limits appears only for Claude.ai Pro/Max subscribers. On
  # Bedrock it never arrives, so rendering two permanently-empty bars
  # is dead chrome.
  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"transcript_path":"%s","cwd":"."}' "$tp")"

  lacks "sesh" || return 1
  lacks "week" || return 1
  has "context" || return 1
}

@test "omits the reset countdown when rate_limits is absent" {
  tp="$TEST_TMPDIR/t.jsonl"; make_transcript "$tp"

  # A countdown to a window that does not exist is noise.
  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"transcript_path":"%s","cwd":"."}' "$tp")"

  lacks "--:--" || return 1
}

@test "keeps the sesh and week bars when rate_limits is present" {
  render '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"rate_limits":{"five_hour":{"used_percentage":20,"resets_at":0},"seven_day":{"used_percentage":30}},"cwd":"."}'

  has "sesh" || return 1
  has "week" || return 1
}

# ── degrades without crashing ───────────────────────────────────────

@test "survives a missing transcript_path" {
  render '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"cwd":"."}'
  [ "$status" -eq 0 ] || return 1
}

@test "survives a transcript_path that points at nothing" {
  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"transcript_path":"%s/nope.jsonl","cwd":"."}' "$TEST_TMPDIR")"
  [ "$status" -eq 0 ] || return 1
}

@test "survives a transcript with no usage records at all" {
  # Bedrock may return responses without usage. Totals should read 0,
  # not crash and not print an empty field.
  tp="$TEST_TMPDIR/t.jsonl"
  printf '%s\n' '{"type":"user","message":{"content":"hi"}}' > "$tp"

  render "$(printf '{"context_window":{"used_percentage":10,"total_input_tokens":0,"total_output_tokens":0},"transcript_path":"%s","cwd":"."}' "$tp")"

  [ "$status" -eq 0 ] || return 1
  has "0" || return 1
}
