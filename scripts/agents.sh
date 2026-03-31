#!/usr/bin/env bash
# Agent tracker, slot manager, job extractor for coolant monitor
# Sourced by monitor.sh — not run directly.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"
source "${SCRIPT_DIR}/sparkline.sh"

# ─── Agent slot variables (bash 3.2: no nested structures) ──

MAX_AGENT_SLOTS=8
ACTIVE_AGENT_COUNT=0

AGENT_SLOT_PID_0="" ; AGENT_SLOT_PID_1="" ; AGENT_SLOT_PID_2="" ; AGENT_SLOT_PID_3=""
AGENT_SLOT_PID_4="" ; AGENT_SLOT_PID_5="" ; AGENT_SLOT_PID_6="" ; AGENT_SLOT_PID_7=""

AGENT_HIST_0=() ; AGENT_HIST_1=() ; AGENT_HIST_2=() ; AGENT_HIST_3=()
AGENT_HIST_4=() ; AGENT_HIST_5=() ; AGENT_HIST_6=() ; AGENT_HIST_7=()

AGENT_JOB_0="" ; AGENT_JOB_1="" ; AGENT_JOB_2="" ; AGENT_JOB_3=""
AGENT_JOB_4="" ; AGENT_JOB_5="" ; AGENT_JOB_6="" ; AGENT_JOB_7=""

# ─── agent_slot_assign(pid) ─────────────────────────────────
# Find first empty slot, assign PID, clear history. Print slot number.
# Returns 0 on success, 1 if all slots full.
agent_slot_assign() {
  local pid="$1"
  local i
  for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
    local current
    eval "current=\"\$AGENT_SLOT_PID_${i}\""
    if [[ -z "$current" ]]; then
      eval "AGENT_SLOT_PID_${i}=\"\$pid\""
      eval "AGENT_HIST_${i}=()"
      eval "AGENT_JOB_${i}=\"\""
      echo "$i"
      return 0
    fi
  done
  return 1
}

# ─── agent_slot_release(slot) ────────────────────────────────
# Push a 0 to history (flatline), clear PID and JOB.
agent_slot_release() {
  local slot="$1"
  history_push "AGENT_HIST_${slot}" 0 20
  eval "AGENT_SLOT_PID_${slot}=\"\""
  eval "AGENT_JOB_${slot}=\"\""
}

# ─── agent_count() ──────────────────────────────────────────
# Count occupied slots, print count.
agent_count() {
  local count=0
  local i
  for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
    local current
    eval "current=\"\$AGENT_SLOT_PID_${i}\""
    if [[ -n "$current" ]]; then
      (( count++ ))
    fi
  done
  echo "$count"
}

# ─── find_claude_pids(all_procs_data) ───────────────────────
# Extract claude PIDs from process data. Shared by scan_agents and monitor.
find_claude_pids() {
  local procs="$1"
  echo "$procs" | awk '{
    cmd = $5
    n = split(cmd, parts, "/")
    base = parts[n]
    split(base, bp, " ")
    if (bp[1] == "claude") print $1
  }'
}

# ─── agent_job(pid, all_procs_data) ─────────────────────────
# AWK pass over process data to find most interesting child command.
# Priority: build(1) > test(2) > runtime(3) > vcs/pkg(4) > idle
agent_job() {
  local pid="$1"
  local procs="$2"

  echo "$procs" | awk -v root="$pid" '
  BEGIN {
    split("tsc eslint webpack vite esbuild", build_cmds)
    for (i in build_cmds) pri[build_cmds[i]] = 1
    split("bats jest vitest pytest", test_cmds)
    for (i in test_cmds) pri[test_cmds[i]] = 2
    split("node python cargo go", rt_cmds)
    for (i in rt_cmds) pri[rt_cmds[i]] = 3
    split("git npm yarn", vcs_cmds)
    for (i in vcs_cmds) pri[vcs_cmds[i]] = 4

    best_pri = 999
    best_cmd = "idle"
    kids[root] = 1
  }
  {
    pid = $1
    ppid = $2
    args = ""
    for (i = 5; i <= NF; i++) {
      if (args != "") args = args " "
      args = args $i
    }

    if (ppid in kids) {
      kids[pid] = 1
    }

    n = split(args, parts, "/")
    cmd_with_args = parts[n]
    split(cmd_with_args, cmd_parts, " ")
    basename_cmd = cmd_parts[1]

    if ((pid in kids) && pid != root) {
      if (basename_cmd in pri && pri[basename_cmd] < best_pri) {
        best_pri = pri[basename_cmd]
        best_cmd = basename_cmd
      }
    }
  }
  END {
    print best_cmd
  }
  '
}

# ─── _bulk_agent_data(all_procs_data, pid_list) ────────────
# Single awk pass to extract CPU% and job for all tracked PIDs.
# Output: one line per PID: "pid cpu_int job"
_bulk_agent_data() {
  local procs="$1"
  local pids="$2"  # space-separated list of PIDs

  echo "$procs" | awk -v tracked="$pids" '
  BEGIN {
    split("tsc eslint webpack vite esbuild", build_cmds)
    for (i in build_cmds) pri[build_cmds[i]] = 1
    split("bats jest vitest pytest", test_cmds)
    for (i in test_cmds) pri[test_cmds[i]] = 2
    split("node python cargo go", rt_cmds)
    for (i in rt_cmds) pri[rt_cmds[i]] = 3
    split("git npm yarn", vcs_cmds)
    for (i in vcs_cmds) pri[vcs_cmds[i]] = 4

    n = split(tracked, tp, " ")
    for (i = 1; i <= n; i++) {
      is_tracked[tp[i]] = 1
      best_pri[tp[i]] = 999
      best_cmd[tp[i]] = "idle"
      cpu_val[tp[i]] = 0
      kids[tp[i]] = 1
    }
  }
  {
    pid = $1
    ppid = $2
    cpu = $3

    args = ""
    for (i = 5; i <= NF; i++) {
      if (args != "") args = args " "
      args = args $i
    }

    # Direct CPU match for tracked PIDs
    if (pid in is_tracked) {
      cpu_val[pid] = int(cpu + 0)
    }

    # Track children recursively for job detection
    if (ppid in kids) {
      kids[pid] = 1
      # Find which root this child belongs to
      for (root in is_tracked) {
        if (ppid == root || (ppid in child_of && child_of[ppid] == root)) {
          child_of[pid] = root

          # Check if this child is an interesting command
          n2 = split(args, parts, "/")
          cmd_with_args = parts[n2]
          split(cmd_with_args, cmd_parts, " ")
          basename_cmd = cmd_parts[1]

          if (basename_cmd in pri && pri[basename_cmd] < best_pri[root]) {
            best_pri[root] = pri[basename_cmd]
            best_cmd[root] = basename_cmd
          }
          break
        }
      }
    }
  }
  END {
    for (pid in is_tracked) {
      printf "%s %d %s\n", pid, cpu_val[pid], best_cmd[pid]
    }
  }
  '
}

# ─── Burst detection (sensed agents) ───────────────────────
# Rolling window to detect agent-like activity from CPU + child burst pattern.

SENSE_HISTORY=()
SENSED_ACTIVE=0

# sense_activity(lightweight_ps_data, claude_pid, cpu_thresh, child_thresh)
# Single-sample check. Returns "1" if claude subtree has elevated CPU AND enough
# bash children, "0" otherwise. Takes lightweight ps output (pid ppid cpu).
# Sums CPU across claude PID + all its children (subtree total).
sense_activity() {
  local procs="$1"
  local claude_pid="$2"
  local cpu_thresh="${3:-10}"
  local child_thresh="${4:-2}"

  echo "$procs" | awk -v cpid="$claude_pid" -v ct="$cpu_thresh" -v cht="$child_thresh" '
  {
    pid = $1; ppid = $2; cpu = $3 + 0
    if (pid == cpid) { subtree_cpu += cpu; found = 1 }
    if (ppid == cpid && pid != cpid) { children++; subtree_cpu += cpu }
  }
  END {
    if (found && subtree_cpu + 0 >= ct && children + 0 >= cht) print 1
    else print 0
  }'
}

# sense_push(signal, window_size, trip_count)
# Push a 0/1 signal into the rolling window. Sets SENSED_ACTIVE.
sense_push() {
  local signal="$1"
  local window="${2:-10}"
  local trip="${3:-4}"

  SENSE_HISTORY+=("$signal")

  # Trim to window size
  local len=${#SENSE_HISTORY[@]}
  if (( len > window )); then
    SENSE_HISTORY=("${SENSE_HISTORY[@]:$((len - window))}")
  fi

  # Sum positive signals
  local sum=0
  local v
  for v in "${SENSE_HISTORY[@]}"; do
    (( sum += v ))
  done

  if (( sum >= trip )); then
    SENSED_ACTIVE=1
  else
    SENSED_ACTIVE=0
  fi
}

# ─── scan_agents(all_procs_data) ────────────────────────────
# Main per-tick function. Detects claude PIDs, manages slots, updates history.
# Sets ACTIVE_AGENT_COUNT as a side effect.
scan_agents() {
  local procs="$1"

  local live_pids
  live_pids=$(find_claude_pids "$procs")

  # Release slots whose PID is no longer live
  local i
  for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
    local slot_pid
    eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
    if [[ -n "$slot_pid" ]]; then
      local found=0
      local lp
      for lp in $live_pids; do
        if [[ "$lp" == "$slot_pid" ]]; then
          found=1
          break
        fi
      done
      if [[ "$found" -eq 0 ]]; then
        agent_slot_release "$i"
      fi
    fi
  done

  # Assign new live PIDs that aren't in any slot
  local lp
  for lp in $live_pids; do
    local already=0
    for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
      local slot_pid
      eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
      if [[ "$slot_pid" == "$lp" ]]; then
        already=1
        break
      fi
    done
    if [[ "$already" -eq 0 ]]; then
      agent_slot_assign "$lp" >/dev/null
    fi
  done

  # Build list of occupied slot PIDs for bulk query
  local tracked_pids=""
  ACTIVE_AGENT_COUNT=0
  for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
    local slot_pid
    eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
    if [[ -n "$slot_pid" ]]; then
      tracked_pids="${tracked_pids}${tracked_pids:+ }${slot_pid}"
      (( ACTIVE_AGENT_COUNT++ ))
    fi
  done

  # Single awk pass for all CPU + job data
  if [[ -n "$tracked_pids" ]]; then
    local bulk_data
    bulk_data=$(_bulk_agent_data "$procs" "$tracked_pids")

    for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
      local slot_pid
      eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
      if [[ -n "$slot_pid" ]]; then
        local cpu_val job_val
        eval "$(echo "$bulk_data" | awk -v pid="$slot_pid" '$1 == pid { printf "cpu_val=%d; job_val=%s", $2, $3 }')"
        cpu_val="${cpu_val:-0}"
        job_val="${job_val:-idle}"
        history_push "AGENT_HIST_${i}" "$cpu_val" 20
        eval "AGENT_JOB_${i}=\"\$job_val\""
      fi
    done
  fi
}
