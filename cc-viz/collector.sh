#!/usr/bin/env bash
# cc-viz/collector.sh — Data collector daemon
#
# Scrapes child processes of a Claude Code parent PID every second,
# maps them to type labels, and writes JSONL snapshots to disk.
#
# Usage:
#   collector.sh [--pid <PID>] [--demo]

set -euo pipefail

# ---------------------------------------------------------------------------
# Source shared functions
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# ---------------------------------------------------------------------------
# Defaults
# ---------------------------------------------------------------------------
TARGET_PID=""
DEMO_MODE=0
MAX_LINES=10000
TICK=0

# ---------------------------------------------------------------------------
# Parse arguments
# ---------------------------------------------------------------------------
while [ $# -gt 0 ]; do
    case "$1" in
        --pid)
            TARGET_PID="$2"
            shift 2
            ;;
        --demo)
            DEMO_MODE=1
            shift
            ;;
        *)
            echo "Usage: $0 [--pid <PID>] [--demo]" >&2
            exit 1
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Auto-detect Claude Code PID if not specified and not in demo mode
# ---------------------------------------------------------------------------
if [ "$DEMO_MODE" -eq 0 ] && [ -z "$TARGET_PID" ]; then
    TARGET_PID=$(pgrep -f "claude" 2>/dev/null | head -1 || true)
    if [ -z "$TARGET_PID" ]; then
        echo "Error: Could not find Claude Code process. Use --pid or --demo." >&2
        exit 1
    fi
    echo "Auto-detected Claude Code PID: $TARGET_PID" >&2
fi

# ---------------------------------------------------------------------------
# Global return variables (avoid subshells to preserve state)
# ---------------------------------------------------------------------------
RET=""          # generic string return
RET_INT=0       # generic integer return

# ---------------------------------------------------------------------------
# comm_to_type — map a process comm name to a single-char type label
# Sets RET to the type letter.
# ---------------------------------------------------------------------------
comm_to_type() {
    case "$1" in
        node)              RET="N" ;;
        grep)              RET="G" ;;
        vitest)            RET="V" ;;
        sed)               RET="S" ;;
        rg|ripgrep)        RET="R" ;;
        find)              RET="F" ;;
        cat)               RET="C" ;;
        python|python3)    RET="P" ;;
        tsc)               RET="T" ;;
        *)                 RET="X" ;;
    esac
}

# ---------------------------------------------------------------------------
# nth_word — extract the Nth word (1-indexed) from a string
# Sets RET to the word.
# ---------------------------------------------------------------------------
nth_word() {
    local str="$1" n="$2" i=0
    for w in $str; do
        i=$((i + 1))
        if [ "$i" -eq "$n" ]; then
            RET="$w"
            return
        fi
    done
    RET=""
}

# ---------------------------------------------------------------------------
# First-seen tracking for age computation (live mode)
# Parallel arrays — bash 3.2 compatible.
# ---------------------------------------------------------------------------
_seen_pids=""
_seen_times=""

# get_first_seen — look up or record first-seen time for a PID
# Sets RET_INT to the age in seconds.
get_first_seen() {
    local pid="$1" now="$2"
    local idx=0

    for p in $_seen_pids; do
        if [ "$p" = "$pid" ]; then
            nth_word "$_seen_times" $((idx + 1))
            RET_INT=$((now - RET))
            return
        fi
        idx=$((idx + 1))
    done

    # Not found — record it
    if [ -z "$_seen_pids" ]; then
        _seen_pids="$pid"
        _seen_times="$now"
    else
        _seen_pids="$_seen_pids $pid"
        _seen_times="$_seen_times $now"
    fi
    RET_INT=0
}

# prune_seen — remove PIDs no longer in the current snapshot
prune_seen() {
    local current_pids="$1"
    local new_pids="" new_times=""
    local idx=0 found

    for p in $_seen_pids; do
        found=0
        for c in $current_pids; do
            if [ "$p" = "$c" ]; then
                found=1
                break
            fi
        done
        if [ "$found" -eq 1 ]; then
            nth_word "$_seen_times" $((idx + 1))
            if [ -z "$new_pids" ]; then
                new_pids="$p"
                new_times="$RET"
            else
                new_pids="$new_pids $p"
                new_times="$new_times $RET"
            fi
        fi
        idx=$((idx + 1))
    done

    _seen_pids="$new_pids"
    _seen_times="$new_times"
}

# ---------------------------------------------------------------------------
# collect_live — scrape real child processes, write JSONL to stdout
# ---------------------------------------------------------------------------
collect_live() {
    local now
    now=$(date +%s)

    # Get all processes with pid, ppid, comm (macOS compatible)
    local ps_out
    ps_out=$(ps -Ao pid=,ppid=,comm= 2>/dev/null) || return 1

    # Build list of descendant PIDs iteratively (breadth-first)
    local pids_to_check="$TARGET_PID"
    local all_children=""
    local found_new=1

    while [ "$found_new" -eq 1 ]; do
        found_new=0
        local new_pids=""
        while IFS= read -r line; do
            local cpid cppid
            # Use shell word splitting (safe — fields are numeric + path)
            set -- $line
            cpid="$1"
            cppid="$2"
            for check_pid in $pids_to_check; do
                if [ "$cppid" = "$check_pid" ]; then
                    local already=0
                    for known in $all_children $pids_to_check; do
                        if [ "$cpid" = "$known" ]; then
                            already=1
                            break
                        fi
                    done
                    if [ "$already" -eq 0 ]; then
                        new_pids="$new_pids $cpid"
                        found_new=1
                    fi
                    break
                fi
            done
        done <<< "$ps_out"

        if [ -n "$new_pids" ]; then
            all_children="$all_children $new_pids"
            pids_to_check="$new_pids"
        fi
    done

    # Trim leading space
    all_children="${all_children# }"

    if [ -z "$all_children" ]; then
        printf '{"tick":%d,"ts":%d,"count":0,"procs":[]}\n' "$TICK" "$now"
        return
    fi

    prune_seen "$all_children"

    # Build JSON procs array
    local procs_json="["
    local first=1

    for child_pid in $all_children; do
        # Get comm for this PID from ps_out
        local comm_line comm_name
        comm_line=$(echo "$ps_out" | awk -v p="$child_pid" '$1 == p {print $3; exit}')
        if [ -z "$comm_line" ]; then
            continue
        fi
        # Strip path to get just the binary name
        comm_name="${comm_line##*/}"

        comm_to_type "$comm_name"
        local type_label="$RET"

        get_first_seen "$child_pid" "$now"
        local age="$RET_INT"

        if [ "$first" -eq 1 ]; then
            first=0
        else
            procs_json="$procs_json,"
        fi
        procs_json="$procs_json{\"pid\":$child_pid,\"type\":\"$type_label\",\"age\":$age}"
    done

    procs_json="$procs_json]"
    local count=0
    for _ in $all_children; do count=$((count + 1)); done

    printf '{"tick":%d,"ts":%d,"count":%d,"procs":%s}\n' "$TICK" "$now" "$count" "$procs_json"
}

# ---------------------------------------------------------------------------
# Demo mode — synthetic data generator
# ---------------------------------------------------------------------------

# PRNG — sets RET_INT (avoids subshell so seed persists)
_demo_seed=${RANDOM:-$$}

demo_rand() {
    local max="$1"
    _demo_seed=$(( (_demo_seed * 48271) % 2147483647 ))
    if [ "$max" -gt 0 ]; then
        RET_INT=$(( _demo_seed % max ))
    else
        RET_INT=0
    fi
}

# Pool of type labels weighted toward common ones
_TYPE_POOL="N N N N N G G G R R F S C P T X"
_TYPE_POOL_SIZE=16

# demo_random_type — sets RET to a random type letter
demo_random_type() {
    demo_rand "$_TYPE_POOL_SIZE"
    local idx=$((RET_INT + 1))
    nth_word "$_TYPE_POOL" "$idx"
    # RET is already set by nth_word
}

# Persistent demo state — parallel arrays of pid, type, age
_demo_pids=""
_demo_types=""
_demo_ages=""
_demo_next_pid=1000

demo_add_proc() {
    demo_random_type
    local type_label="$RET"
    if [ -z "$_demo_pids" ]; then
        _demo_pids="$_demo_next_pid"
        _demo_types="$type_label"
        _demo_ages="0"
    else
        _demo_pids="$_demo_pids $_demo_next_pid"
        _demo_types="$_demo_types $type_label"
        _demo_ages="$_demo_ages 0"
    fi
    _demo_next_pid=$((_demo_next_pid + 1))
}

demo_remove_random() {
    local count=0
    for _ in $_demo_pids; do count=$((count + 1)); done
    if [ "$count" -le 0 ]; then
        return
    fi
    demo_rand "$count"
    local target_idx="$RET_INT"

    local new_pids="" new_types="" new_ages=""
    local i=0

    # Walk all three arrays in lockstep using positional params
    # Save types and ages first
    local saved_types="$_demo_types"
    local saved_ages="$_demo_ages"

    for p in $_demo_pids; do
        if [ "$i" -ne "$target_idx" ]; then
            nth_word "$saved_types" $((i + 1))
            local t="$RET"
            nth_word "$saved_ages" $((i + 1))
            local a="$RET"
            if [ -z "$new_pids" ]; then
                new_pids="$p"
                new_types="$t"
                new_ages="$a"
            else
                new_pids="$new_pids $p"
                new_types="$new_types $t"
                new_ages="$new_ages $a"
            fi
        fi
        i=$((i + 1))
    done

    _demo_pids="$new_pids"
    _demo_types="$new_types"
    _demo_ages="$new_ages"
}

demo_age_all() {
    local new_ages=""
    for a in $_demo_ages; do
        local aged=$((a + 1))
        if [ -z "$new_ages" ]; then
            new_ages="$aged"
        else
            new_ages="$new_ages $aged"
        fi
    done
    _demo_ages="$new_ages"
}

# collect_demo — generate one synthetic JSONL snapshot
# Writes directly to the data file (no subshell needed).
collect_demo() {
    local now
    now=$(date +%s)

    # Determine phase from tick (loops at 90)
    local phase_tick=$((TICK % 90))
    local spawns=0 deaths=0

    if [ "$phase_tick" -lt 30 ]; then
        # Calm baseline: 3-8 procs, occasional spawn/death
        demo_rand 6
        local target=$((3 + RET_INT))
        local current=0
        for _ in $_demo_pids; do current=$((current + 1)); done
        if [ "$current" -lt "$target" ]; then
            spawns=$((target - current))
        elif [ "$current" -gt "$target" ]; then
            deaths=$((current - target))
        fi
        # Small random churn
        demo_rand 3
        if [ "$RET_INT" -eq 0 ] && [ "$current" -gt 2 ]; then
            spawns=$((spawns + 1))
            deaths=$((deaths + 1))
        fi

    elif [ "$phase_tick" -lt 45 ]; then
        # Ramp up: spawn rate increases linearly
        local ramp=$((phase_tick - 30))
        spawns=$((1 + ramp / 2))
        demo_rand 2
        deaths="$RET_INT"

    elif [ "$phase_tick" -lt 60 ]; then
        # Explosion: 20-40 new spawns per tick
        demo_rand 21
        spawns=$((20 + RET_INT))
        demo_rand 5
        deaths=$((RET_INT + 1))

    elif [ "$phase_tick" -lt 75 ]; then
        # Plateau: high count but spawn rate stabilizes
        demo_rand 5
        spawns=$((2 + RET_INT))
        demo_rand 5
        deaths=$((2 + RET_INT))

    else
        # Die-off: death rate exceeds spawn rate
        local dieoff=$((phase_tick - 75))
        demo_rand 2
        spawns="$RET_INT"
        deaths=$((3 + dieoff * 2))
        local current=0
        for _ in $_demo_pids; do current=$((current + 1)); done
        if [ "$deaths" -gt "$current" ]; then
            deaths="$current"
        fi
    fi

    # Apply spawns
    local s=0
    while [ "$s" -lt "$spawns" ]; do
        demo_add_proc
        s=$((s + 1))
    done

    # Apply deaths
    local d=0
    while [ "$d" -lt "$deaths" ]; do
        demo_remove_random
        d=$((d + 1))
    done

    # Age all procs
    demo_age_all

    # Build JSON
    local procs_json="["
    local first=1
    local idx=0
    local saved_types="$_demo_types"
    local saved_ages="$_demo_ages"

    for p in $_demo_pids; do
        nth_word "$saved_types" $((idx + 1))
        local t="$RET"
        nth_word "$saved_ages" $((idx + 1))
        local a="$RET"
        if [ "$first" -eq 1 ]; then
            first=0
        else
            procs_json="$procs_json,"
        fi
        procs_json="$procs_json{\"pid\":$p,\"type\":\"$t\",\"age\":$a}"
        idx=$((idx + 1))
    done

    procs_json="$procs_json]"
    local count=0
    for _ in $_demo_pids; do count=$((count + 1)); done

    printf '{"tick":%d,"ts":%d,"count":%d,"procs":%s}\n' "$TICK" "$now" "$count" "$procs_json"
}

# ---------------------------------------------------------------------------
# Ring buffer — cap file at MAX_LINES
# ---------------------------------------------------------------------------
cap_file() {
    if [ ! -f "$CC_VIZ_DATA" ]; then
        return
    fi
    local lines
    lines=$(wc -l < "$CC_VIZ_DATA" | tr -d ' ')
    if [ "$lines" -gt "$MAX_LINES" ]; then
        local tmp="${CC_VIZ_DATA}.tmp"
        tail -n "$MAX_LINES" "$CC_VIZ_DATA" > "$tmp"
        mv "$tmp" "$CC_VIZ_DATA"
    fi
}

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------
echo "cc-viz collector starting (demo=$DEMO_MODE, data=$CC_VIZ_DATA)" >&2

# Seed demo state with a few initial procs
if [ "$DEMO_MODE" -eq 1 ]; then
    _init_i=0
    while [ "$_init_i" -lt 5 ]; do
        demo_add_proc
        _init_i=$((_init_i + 1))
    done
fi

# Truncate data file on start
: > "$CC_VIZ_DATA"

trap 'echo "cc-viz collector stopped." >&2; exit 0' INT TERM

while true; do
    if [ "$DEMO_MODE" -eq 1 ]; then
        collect_demo >> "$CC_VIZ_DATA"
    else
        collect_live >> "$CC_VIZ_DATA"
    fi

    # Periodic ring-buffer trim (every 100 ticks)
    if [ $((TICK % 100)) -eq 0 ]; then
        cap_file
    fi

    TICK=$((TICK + 1))
    sleep 1
done
