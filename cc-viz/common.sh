#!/usr/bin/env bash
# cc-viz/common.sh — Shared functions sourced by all pane scripts

# ---------------------------------------------------------------------------
# JSONL data path
# ---------------------------------------------------------------------------
CC_VIZ_DATA="${CC_VIZ_DATA:-/tmp/cc-procs.jsonl}"

# ---------------------------------------------------------------------------
# ANSI color codes
# ---------------------------------------------------------------------------
RST=$'\033[0m'
BOLD=$'\033[1m'
DIM=$'\033[2m'
UL=$'\033[4m'
BLINK=$'\033[5m'
INV=$'\033[7m'

FG_BLACK=$'\033[30m'
FG_RED=$'\033[31m'
FG_GREEN=$'\033[32m'
FG_YELLOW=$'\033[33m'
FG_BLUE=$'\033[34m'
FG_MAGENTA=$'\033[35m'
FG_CYAN=$'\033[36m'
FG_WHITE=$'\033[37m'
FG_GRAY=$'\033[90m'
FG_BRIGHT_YELLOW=$'\033[93m'
FG_BRIGHT_CYAN=$'\033[96m'

BG_BLACK=$'\033[40m'
BG_RED=$'\033[41m'
BG_GREEN=$'\033[42m'
BG_YELLOW=$'\033[43m'
BG_BLUE=$'\033[44m'
BG_MAGENTA=$'\033[45m'
BG_CYAN=$'\033[46m'
BG_WHITE=$'\033[47m'

# ---------------------------------------------------------------------------
# Threshold defaults (override via env vars)
# ---------------------------------------------------------------------------
SPAWN_WARN="${SPAWN_WARN:-10}"
SPAWN_CRIT="${SPAWN_CRIT:-20}"
NET_WARN="${NET_WARN:-5}"
NET_CRIT="${NET_CRIT:-15}"
TOTAL_WARN="${TOTAL_WARN:-50}"
TOTAL_CRIT="${TOTAL_CRIT:-100}"
TYPE_CRIT="${TYPE_CRIT:-30}"
NET_SUSTAIN="${NET_SUSTAIN:-3}"

# ---------------------------------------------------------------------------
# get_type_color — return ANSI color for a single-char type label
# Usage: color=$(get_type_color "N")
# ---------------------------------------------------------------------------
get_type_color() {
    local t="$1"
    case "$t" in
        N) printf '%s' "$FG_GREEN" ;;
        G) printf '%s' "$FG_YELLOW" ;;
        V) printf '%s' "$FG_RED" ;;
        S) printf '%s' "$FG_CYAN" ;;
        R) printf '%s' "$FG_MAGENTA" ;;
        F) printf '%s' "$FG_BLUE" ;;
        C) printf '%s' "$FG_WHITE" ;;
        P) printf '%s' "$FG_BRIGHT_YELLOW" ;;
        T) printf '%s' "$FG_BRIGHT_CYAN" ;;
        X) printf '%s' "$FG_GRAY" ;;
        *) printf '%s' "$RST" ;;
    esac
}

# ---------------------------------------------------------------------------
# threshold_color — return white/yellow/red based on value vs thresholds
# Usage: color=$(threshold_color "$value" "$warn" "$crit")
# ---------------------------------------------------------------------------
threshold_color() {
    local val="$1" warn="$2" crit="$3"
    if [ "$val" -ge "$crit" ] 2>/dev/null; then
        printf '%s' "$FG_RED"
    elif [ "$val" -ge "$warn" ] 2>/dev/null; then
        printf '%s' "$FG_YELLOW"
    else
        printf '%s' "$FG_WHITE"
    fi
}

# ---------------------------------------------------------------------------
# parse_jsonl_line — parse a JSONL line into shell variables
#
# Expects input format (one JSON object per line):
#   {"tick":5,"ts":1711843200,"count":12,"procs":[{"pid":123,"type":"N","age":5},...]}
#
# Outputs eval-safe variable assignments:
#   _tick=5
#   _ts=1711843200
#   _count=12
#   _procs='[{"pid":123,"type":"N","age":5},...]'  (raw JSON array)
#   _proc_pids=(123 456 ...)
#   _proc_types=(N G ...)
#   _proc_ages=(5 10 ...)
#
# Usage: eval "$(parse_jsonl_line "$line")"
# ---------------------------------------------------------------------------
parse_jsonl_line() {
    local line="$1"
    if [ -z "$line" ]; then
        return 1
    fi

    # Use jq to extract fields and emit shell assignments
    printf '%s' "$line" | jq -r '
        "_tick=\(.tick)\n" +
        "_ts=\(.ts)\n" +
        "_count=\(.count)\n" +
        "_procs=\(.procs | tostring)\n" +
        "_proc_pids=(\(.procs | map(.pid) | join(" ")))\n" +
        "_proc_types=(\(.procs | map(.type) | join(" ")))\n" +
        "_proc_ages=(\(.procs | map(.age) | join(" ")))"
    ' 2>/dev/null
}

# ---------------------------------------------------------------------------
# compute_deltas — compare two consecutive JSONL snapshots
#
# Usage: eval "$(compute_deltas "$prev_line" "$curr_line")"
# Outputs:
#   _spawns=<number of new PIDs>
#   _deaths=<number of gone PIDs>
#   _net=<spawns - deaths>
# ---------------------------------------------------------------------------
compute_deltas() {
    local prev="$1" curr="$2"

    if [ -z "$prev" ] || [ -z "$curr" ]; then
        echo "_spawns=0"
        echo "_deaths=0"
        echo "_net=0"
        return
    fi

    # Use jq to compute set differences on PID lists
    local prev_pids curr_pids
    prev_pids=$(printf '%s' "$prev" | jq -r '[.procs[].pid] | sort | map(tostring) | join(" ")' 2>/dev/null)
    curr_pids=$(printf '%s' "$curr" | jq -r '[.procs[].pid] | sort | map(tostring) | join(" ")' 2>/dev/null)

    # Convert to arrays (bash 3.2 compatible — word splitting)
    local spawns=0 deaths=0
    local p c found

    # Count spawns: PIDs in curr but not in prev
    for c in $curr_pids; do
        found=0
        for p in $prev_pids; do
            if [ "$c" = "$p" ]; then
                found=1
                break
            fi
        done
        if [ "$found" -eq 0 ]; then
            spawns=$((spawns + 1))
        fi
    done

    # Count deaths: PIDs in prev but not in curr
    for p in $prev_pids; do
        found=0
        for c in $curr_pids; do
            if [ "$p" = "$c" ]; then
                found=1
                break
            fi
        done
        if [ "$found" -eq 0 ]; then
            deaths=$((deaths + 1))
        fi
    done

    local net=$((spawns - deaths))
    echo "_spawns=$spawns"
    echo "_deaths=$deaths"
    echo "_net=$net"
}
