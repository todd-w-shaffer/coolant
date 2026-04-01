#!/usr/bin/env bash
# cc-viz/phase-ring.sh — Phase Ring widget
# Classifies system state each tick into calm/ramping/exploding/cooling
# and displays the trajectory as a rolling sequence of colored dots.
#
# Modes:
#   (default)   Standalone pane with buffered rendering
#   --inline    Single-line output for tmux status bar or embedding

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=cc-viz/common.sh
source "$SCRIPT_DIR/common.sh"

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
RING_WIDTH="${RING_WIDTH:-20}"   # number of dots in the ring
DOT="●"                         # U+25CF

# ---------------------------------------------------------------------------
# Ring buffer — plain indexed array, bash 3.2 compatible
# ---------------------------------------------------------------------------
ring=()
ring_count=0

ring_push() {
    local phase="$1"
    ring+=("$phase")
    ring_count=$((ring_count + 1))
    # Trim oldest entries if over capacity
    if [ "$ring_count" -gt "$RING_WIDTH" ]; then
        ring=("${ring[@]:1}")
        ring_count=$RING_WIDTH
    fi
}

# ---------------------------------------------------------------------------
# Phase classification
# Priority: EXPLODING > RAMPING > COOLING > CALM
# ---------------------------------------------------------------------------
classify_phase() {
    local total="$1" spawns="$2" net="$3"

    # EXPLODING: any critical threshold breached
    if [ "$spawns" -ge "$SPAWN_CRIT" ] || [ "$net" -ge "$NET_CRIT" ] || [ "$total" -ge "$TOTAL_CRIT" ]; then
        echo "exploding"
        return
    fi

    # RAMPING: warning thresholds (but below critical)
    if [ "$spawns" -ge "$SPAWN_WARN" ] || [ "$net" -ge "$NET_WARN" ]; then
        echo "ramping"
        return
    fi

    # COOLING: elevated total but shrinking (net < 0)
    if [ "$total" -gt "$TOTAL_WARN" ] && [ "$net" -lt 0 ]; then
        echo "cooling"
        return
    fi

    echo "calm"
}

# ---------------------------------------------------------------------------
# Count spawns (age == 0) from parsed proc data
# ---------------------------------------------------------------------------
count_spawns_by_age() {
    local spawns=0
    local i
    for (( i = 0; i < ${#_proc_ages[@]}; i++ )); do
        if [ "${_proc_ages[$i]}" -eq 0 ] 2>/dev/null; then
            spawns=$((spawns + 1))
        fi
    done
    echo "$spawns"
}

# ---------------------------------------------------------------------------
# Dot rendering — colored dot for a phase
# ---------------------------------------------------------------------------
phase_dot() {
    case "$1" in
        calm)      printf '%s%s%s' "$FG_GREEN"  "$DOT" "$RST" ;;
        ramping)   printf '%s%s%s' "$FG_YELLOW" "$DOT" "$RST" ;;
        exploding) printf '%s%s%s' "$FG_RED"    "$DOT" "$RST" ;;
        cooling)   printf '%s%s%s' "$FG_CYAN"   "$DOT" "$RST" ;;
        *)         printf '%s%s%s' "$DIM"       "$DOT" "$RST" ;;
    esac
}

# ---------------------------------------------------------------------------
# Phase label — uppercase, colored
# ---------------------------------------------------------------------------
phase_label() {
    local phase="$1"
    local label
    label=$(echo "$phase" | tr '[:lower:]' '[:upper:]')
    case "$phase" in
        calm)      printf '%s%s%s%s' "$BOLD" "$FG_GREEN"  "$label" "$RST" ;;
        ramping)   printf '%s%s%s%s' "$BOLD" "$FG_YELLOW" "$label" "$RST" ;;
        exploding) printf '%s%s%s%s' "$BOLD" "$FG_RED"    "$label" "$RST" ;;
        cooling)   printf '%s%s%s%s' "$BOLD" "$FG_CYAN"   "$label" "$RST" ;;
        *)         printf '%s%s%s'   "$DIM"  "$label"     "$RST"         ;;
    esac
}

# ---------------------------------------------------------------------------
# render_ring_line — output the dot sequence + label as a single line
# ---------------------------------------------------------------------------
render_ring_line() {
    local current_phase="${1:-calm}"
    local buf=""
    local i
    for (( i = 0; i < ring_count; i++ )); do
        if [ "$i" -gt 0 ]; then
            buf="${buf} "
        fi
        buf="${buf}$(phase_dot "${ring[$i]}")"
    done
    # Pad with dim dots if ring not full yet
    for (( i = ring_count; i < RING_WIDTH; i++ )); do
        if [ "$i" -gt 0 ] || [ "$ring_count" -gt 0 ]; then
            buf="${buf} "
        fi
        buf="${buf}${DIM}${DOT}${RST}"
    done
    buf="${buf}  $(phase_label "$current_phase")"
    printf '%s' "$buf"
}

# ---------------------------------------------------------------------------
# --inline mode: read last N lines, classify each, render, exit
# ---------------------------------------------------------------------------
run_inline() {
    local prev_line="" curr_line="" current_phase="calm"

    # Seed ring from recent history so the first frame isn't empty
    if [ -f "$CC_VIZ_DATA" ]; then
        local seed
        seed=$(tail -n "$RING_WIDTH" "$CC_VIZ_DATA" 2>/dev/null || true)
        if [ -n "$seed" ]; then
            while IFS= read -r line; do
                [ -z "$line" ] && continue
                prev_line="$curr_line"
                curr_line="$line"
                eval "$(parse_jsonl_line "$curr_line")" || continue
                eval "$(compute_deltas "$prev_line" "$curr_line")"
                local total="${_count:-0}"
                local spawns
                spawns=$(count_spawns_by_age)
                current_phase=$(classify_phase "$total" "$spawns" "${_net:-0}")
                ring_push "$current_phase"
            done <<< "$seed"
        fi
    fi

    # Live tail — re-render the single line in place each tick
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        prev_line="$curr_line"
        curr_line="$line"

        eval "$(parse_jsonl_line "$curr_line")" || continue
        eval "$(compute_deltas "$prev_line" "$curr_line")"

        local total="${_count:-0}"
        local spawns
        spawns=$(count_spawns_by_age)
        current_phase=$(classify_phase "$total" "$spawns" "${_net:-0}")
        ring_push "$current_phase"

        # Overwrite the same line in place
        printf '\r\033[K'
        render_ring_line "$current_phase"
    done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

# ---------------------------------------------------------------------------
# Standalone pane mode: buffered rendering loop
# ---------------------------------------------------------------------------
render_pane() {
    local current_phase="$1"
    # Center the ring on one line, vertically centered
    local v_pad=$(( ROWS / 2 ))
    if [ "$v_pad" -lt 0 ]; then v_pad=0; fi

    # Build the entire frame as a sequence of full-width lines
    local i
    for (( i = 0; i < ROWS; i++ )); do
        if [ "$i" -eq "$v_pad" ]; then
            # Ring line — render inline, pad to COLS with trailing spaces
            local ring_buf
            ring_buf=$(render_ring_line "$current_phase")
            printf '  %s' "$ring_buf"
        fi
        # Erase to end of line to clear any previous content
        printf '\033[K\n'
    done
}

main() {
    # Hide cursor, restore on exit
    printf '\033[?25l'
    trap 'printf "\033[?25h"; exit 0' INT TERM EXIT

    local prev_line="" curr_line="" current_phase="calm"

    # Cache terminal geometry before subshell
    COLS=$(tput cols 2>/dev/null || echo 60)
    ROWS=$(tput lines 2>/dev/null || echo 10)

    clear

    while IFS= read -r line; do
        [ -z "$line" ] && continue

        prev_line="$curr_line"
        curr_line="$line"

        eval "$(parse_jsonl_line "$curr_line")" || continue
        eval "$(compute_deltas "$prev_line" "$curr_line")"

        local total="${_count:-0}"
        local spawns
        spawns=$(count_spawns_by_age)
        local net="${_net:-0}"

        current_phase=$(classify_phase "$total" "$spawns" "$net")
        ring_push "$current_phase"

        # Cache tput before subshell capture
        COLS=$(tput cols 2>/dev/null || echo 60)
        ROWS=$(tput lines 2>/dev/null || echo 10)

        # Buffered render
        local frame
        frame=$(render_pane "$current_phase")
        tput home
        printf '%s' "$frame"
        tput ed
    done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
case "${1:-}" in
    --inline) run_inline ;;
    *)        main ;;
esac
