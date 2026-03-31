#!/usr/bin/env bats

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  export COOLANT_LOCKFILE="${TEST_TMPDIR}/coolant.lock"
  export COOLANT_COUNTER="${TEST_TMPDIR}/coolant.count"
  export COOLANT_LOG="${TEST_TMPDIR}/coolant.log"
  export COOLANT_THRESHOLD=3

  source "${PROJECT_ROOT}/scripts/agents.sh"
}

teardown() {
  rm -rf "$TEST_TMPDIR"
}

# ─── agent_slot_assign ──────────────────────────────────

@test "agent_slot_assign assigns to first empty slot" {
  agent_slot_assign 1234 > "${TEST_TMPDIR}/slot_out"
  local slot
  slot=$(cat "${TEST_TMPDIR}/slot_out")
  [[ "$slot" == "0" ]]
  [[ "$AGENT_SLOT_PID_0" == "1234" ]]
}

@test "agent_slot_assign fills slots sequentially" {
  agent_slot_assign 1000 >/dev/null
  agent_slot_assign 2000 >/dev/null
  [[ "$AGENT_SLOT_PID_0" == "1000" ]]
  [[ "$AGENT_SLOT_PID_1" == "2000" ]]
}

@test "agent_slot_assign returns 1 when all slots full" {
  for i in 0 1 2 3 4 5 6 7; do
    agent_slot_assign "$((1000 + i))" >/dev/null
  done
  run agent_slot_assign 9999
  [[ "$status" -eq 1 ]]
}

# ─── agent_slot_release ─────────────────────────────────

@test "agent_slot_release clears PID and job" {
  agent_slot_assign 1234 >/dev/null
  AGENT_JOB_0="tsc"
  agent_slot_release 0
  [[ "$AGENT_SLOT_PID_0" == "" ]]
  [[ "$AGENT_JOB_0" == "" ]]
}

@test "agent_slot_release pushes zero to history" {
  agent_slot_assign 1234 >/dev/null
  history_push AGENT_HIST_0 50 20
  history_push AGENT_HIST_0 60 20
  agent_slot_release 0
  local len=${#AGENT_HIST_0[@]}
  [[ "${AGENT_HIST_0[$((len - 1))]}" == "0" ]]
}

# ─── agent_count ─────────────────────────────────────────

@test "agent_count returns number of occupied slots" {
  agent_slot_assign 1000 >/dev/null
  agent_slot_assign 2000 >/dev/null
  agent_slot_assign 3000 >/dev/null
  local count
  count=$(agent_count)
  [[ "$count" == "3" ]]
}

# ─── agent_job ───────────────────────────────────────────

@test "agent_job extracts tsc from child process" {
  local procs
  procs="100 1 5.0 1024 /usr/local/bin/claude --mode auto
200 100 25.0 512 /usr/local/bin/tsc --watch"
  local job
  job=$(agent_job 100 "$procs")
  [[ "$job" == "tsc" ]]
}

@test "agent_job returns idle when no interesting children" {
  local procs
  procs="100 1 5.0 1024 /usr/local/bin/claude --mode auto"
  local job
  job=$(agent_job 100 "$procs")
  [[ "$job" == "idle" ]]
}

@test "agent_job picks highest priority child" {
  local procs
  procs="100 1 5.0 1024 /usr/local/bin/claude --mode auto
200 100 1.0 256 /usr/bin/node /tmp/script.js
300 100 25.0 512 /usr/local/bin/tsc --watch"
  local job
  job=$(agent_job 100 "$procs")
  [[ "$job" == "tsc" ]]
}

# ─── scan_agents ─────────────────────────────────────────

@test "scan_agents assigns new PIDs to slots" {
  local procs
  procs="100 1 5.0 1024 /usr/local/bin/claude --mode auto
200 100 25.0 512 /usr/local/bin/tsc --watch"
  scan_agents "$procs"
  [[ "$AGENT_SLOT_PID_0" == "100" ]]
}

@test "scan_agents releases slots for exited PIDs" {
  agent_slot_assign 100 >/dev/null
  local procs
  procs="500 1 2.0 512 /usr/bin/bash"
  scan_agents "$procs"
  [[ "$AGENT_SLOT_PID_0" == "" ]]
}

@test "scan_agents preserves slot across ticks" {
  local procs
  procs="100 1 5.0 1024 /usr/local/bin/claude --mode auto"
  scan_agents "$procs"
  [[ "$AGENT_SLOT_PID_0" == "100" ]]
  scan_agents "$procs"
  [[ "$AGENT_SLOT_PID_0" == "100" ]]
}

@test "scan_agents pushes CPU to slot history" {
  local procs
  procs="100 1 42.5 1024 /usr/local/bin/claude --mode auto"
  scan_agents "$procs"
  [[ ${#AGENT_HIST_0[@]} -ge 1 ]]
  [[ "${AGENT_HIST_0[0]}" == "42" ]]
}

# ─── sense_activity ─────────────────────────────────────

@test "sense_activity returns 1 when claude CPU elevated and bash children present" {
  local procs
  procs="100 1 25.0
200 100 0.0
300 100 0.0"
  local result
  result=$(sense_activity "$procs" 100 10 2)
  [[ "$result" == "1" ]]
}

@test "sense_activity returns 0 when claude CPU low" {
  local procs
  procs="100 1 3.0
200 100 0.0
300 100 0.0"
  local result
  result=$(sense_activity "$procs" 100 10 2)
  [[ "$result" == "0" ]]
}

@test "sense_activity trips on subtree CPU even when claude PID itself is low" {
  local procs
  procs="100 1 4.0
200 100 8.0
300 100 6.0"
  local result
  # claude=4 + child1=8 + child2=6 = 18 subtree total, threshold 10
  result=$(sense_activity "$procs" 100 10 2)
  [[ "$result" == "1" ]]
}

@test "sense_activity returns 0 when no children despite high CPU" {
  local procs
  procs="100 1 25.0
500 1 0.0"
  local result
  result=$(sense_activity "$procs" 100 10 2)
  [[ "$result" == "0" ]]
}

# ─── sense_push / SENSED_ACTIVE ────────────────────────

@test "sense_push trips SENSED_ACTIVE after threshold positive samples" {
  SENSE_HISTORY=()
  SENSED_ACTIVE=0
  # Push 4 positive signals (default trip count is 4, window is 10)
  sense_push 1 10 4
  sense_push 1 10 4
  sense_push 1 10 4
  [[ "$SENSED_ACTIVE" == "0" ]]
  sense_push 1 10 4
  [[ "$SENSED_ACTIVE" == "1" ]]
}

@test "sense_push clears SENSED_ACTIVE when signals drop" {
  SENSE_HISTORY=()
  SENSED_ACTIVE=0
  # Trip it
  for i in 1 2 3 4 5; do sense_push 1 10 4; done
  [[ "$SENSED_ACTIVE" == "1" ]]
  # Push enough zeros to dilute below threshold
  for i in 1 2 3 4 5 6 7; do sense_push 0 10 4; done
  [[ "$SENSED_ACTIVE" == "0" ]]
}
