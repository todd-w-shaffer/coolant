#!/usr/bin/env bats
# scripts/coolant — the door shim. It finds a thermo that answers
# --door-version and execs `thermo run|queue`; with none, it warns once
# and runs the command unqueued.

load test_helper

DOOR="$PROJECT_ROOT/scripts/coolant"

# A fake thermo that has the door: answers the probe, then echoes how it
# was called so tests can see what the shim handed it.
make_door_thermo() {
  cat > "$TEST_TMPDIR/thermo" <<'EOF'
#!/bin/bash
if [ "$1" = "--door-version" ]; then echo 1; exit 0; fi
echo "thermo-called: $*"
EOF
  chmod +x "$TEST_TMPDIR/thermo"
}

# A fake thermo without the door: it rejects an unknown flag, as flag.Parse
# does, and starts its dashboard on a bare word.
make_old_thermo() {
  cat > "$TEST_TMPDIR/thermo" <<'EOF'
#!/bin/bash
case "$1" in -*) echo "flag provided but not defined: $1" >&2; exit 2 ;; esac
echo "TUI STARTED"
EOF
  chmod +x "$TEST_TMPDIR/thermo"
}

@test "door execs thermo run with the caller's arguments" {
  make_door_thermo
  run env COOLANT_THERMO="$TEST_TMPDIR/thermo" "$DOOR" run --class suite -- vitest run
  [ "$status" -eq 0 ]
  [ "$output" = "thermo-called: run --class suite -- vitest run" ]
}

@test "door execs thermo queue" {
  make_door_thermo
  run env COOLANT_THERMO="$TEST_TMPDIR/thermo" "$DOOR" queue --json
  [ "$output" = "thermo-called: queue --json" ]
}

@test "door passes queue weights and its flags to thermo" {
  make_door_thermo
  run env COOLANT_THERMO="$TEST_TMPDIR/thermo" "$DOOR" queue weights --toml --since 2026-11-02T00:00:00Z
  [ "$output" = "thermo-called: queue weights --toml --since 2026-11-02T00:00:00Z" ]
}

@test "door with no thermo runs the command unqueued" {
  run env COOLANT_THERMO="$TEST_TMPDIR/missing" "$DOOR" run --class suite -- sh -c 'echo ran; exit 7'
  [ "$status" -eq 7 ]
  [[ "$output" == *"ran"* ]]
}

@test "door degrade honours leading assignments like env" {
  run env COOLANT_THERMO="$TEST_TMPDIR/missing" "$DOOR" run --class suite -- CI=1 sh -c 'echo "ci=$CI"'
  [[ "$output" == *"ci=1"* ]]
}

@test "door with no thermo warns exactly once" {
  run env COOLANT_THERMO="$TEST_TMPDIR/missing" "$DOOR" run --class suite -- true
  [ "$(grep -c '^coolant:' <<< "$output")" -eq 1 ]
}

@test "door never starts a thermo that predates the door" {
  make_old_thermo
  run env COOLANT_THERMO="$TEST_TMPDIR/thermo" "$DOOR" run --class suite -- echo ran-unqueued
  [ "$status" -eq 0 ]
  [[ "$output" != *"TUI STARTED"* ]]
  [[ "$output" == *"ran-unqueued"* ]]
}

@test "door names a thermo that predates the door and how to upgrade it" {
  make_old_thermo
  run env COOLANT_THERMO="$TEST_TMPDIR/thermo" "$DOOR" run --class suite -- true
  [[ "$output" == *"$TEST_TMPDIR/thermo predates the queue door"* ]]
  [[ "$output" == *"thermo upgrade"* ]]
}

# The contract between this shim and the Go side: the protocol the shim
# expects is the one the built thermo answers (before flag.Parse).
@test "door protocol matches the built thermo" {
  [ -x "$PROJECT_ROOT/bin/thermo" ] || skip "bin/thermo not built"
  local want
  want="$(sed -n 's/^PROTOCOL=//p' "$DOOR")"
  [ "$("$PROJECT_ROOT/bin/thermo" --door-version)" = "$want" ]
}

@test "door queue with no thermo fails with a message" {
  run env COOLANT_THERMO="$TEST_TMPDIR/missing" "$DOOR" queue
  [ "$status" -eq 1 ]
  [[ "$output" == *"coolant:"* ]]
}

@test "door degrade without -- is a usage error" {
  run env COOLANT_THERMO="$TEST_TMPDIR/missing" "$DOOR" run --class suite vitest
  [ "$status" -eq 64 ]
}

@test "door with no verb is a usage error" {
  run "$DOOR"
  [ "$status" -eq 64 ]
}

@test "door resolves through a symlink to find the checkout's thermo" {
  make_door_thermo
  mkdir -p "$TEST_TMPDIR/checkout/scripts" "$TEST_TMPDIR/checkout/bin" "$TEST_TMPDIR/links"
  cp "$DOOR" "$TEST_TMPDIR/checkout/scripts/coolant"
  cp "$TEST_TMPDIR/thermo" "$TEST_TMPDIR/checkout/bin/thermo"
  ln -s "$TEST_TMPDIR/checkout/scripts/coolant" "$TEST_TMPDIR/links/coolant"
  run env -u COOLANT_THERMO PATH=/usr/bin:/bin "$TEST_TMPDIR/links/coolant" run --class build -- true
  [ "$output" = "thermo-called: run --class build -- true" ]
}

@test "door falls back to thermo on PATH" {
  make_door_thermo
  mkdir -p "$TEST_TMPDIR/lone/scripts" "$TEST_TMPDIR/pathbin"
  cp "$DOOR" "$TEST_TMPDIR/lone/scripts/coolant"
  cp "$TEST_TMPDIR/thermo" "$TEST_TMPDIR/pathbin/thermo"
  run env -u COOLANT_THERMO PATH="$TEST_TMPDIR/pathbin:/usr/bin:/bin" "$TEST_TMPDIR/lone/scripts/coolant" queue
  [ "$output" = "thermo-called: queue" ]
}
