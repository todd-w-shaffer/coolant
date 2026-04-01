#!/usr/bin/env bash
# cc-viz/spawnrate.sh — Spawn Rate sparkline pane
# Shows rate of process creation/destruction as scrolling sparklines.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=cc-viz/common.sh
source "$SCRIPT_DIR/common.sh"

# ---------------------------------------------------------------------------
# Sparkline config
# ---------------------------------------------------------------------------
SPARK_CHARS=(▁ ▂ ▃ ▄ ▅ ▆ ▇ █)
SPARK_MAX="${SPARK_MAX:-40}"       # max value before clamping to █
LABEL_WIDTH=10                     # fixed left column width
COLS=60                            # cached terminal width (refreshed before render)

# ---------------------------------------------------------------------------
# State arrays — parallel indexed, one entry per column
# We store raw numeric values; sparkline chars computed at render time.
# ---------------------------------------------------------------------------
spawn_history=()
death_history=()
net_history=()
peak_net=0

# ---------------------------------------------------------------------------
# spark_char — map a value to a sparkline character
# Usage: spark_char <value> <max>
# ---------------------------------------------------------------------------
spark_char() {
    local val="$1" max="$2"
    if [ "$val" -lt 0 ]; then val=0; fi
    if [ "$val" -gt "$max" ]; then val="$max"; fi
    if [ "$max" -le 0 ]; then
        printf '%s' "${SPARK_CHARS[0]}"
        return
    fi
    # Map to 0..7
    local idx=$(( (val * 7) / max ))
    if [ "$idx" -gt 7 ]; then idx=7; fi
    printf '%s' "${SPARK_CHARS[$idx]}"
}

# ---------------------------------------------------------------------------
# render_sparkline_row — draw one labeled sparkline row with per-char color
# Usage: render_sparkline_row <label> <color_func> <max> <values...>
#   color_func: "spawn" | "death" | "net"
# ---------------------------------------------------------------------------
render_sparkline_row() {
    local label="$1" cfunc="$2" max="$3"
    shift 3
    local values=("$@")

    # Pad label to LABEL_WIDTH
    printf ' %-*s' "$LABEL_WIDTH" "$label"

    # Available sparkline width: total cols minus label minus 1 trailing space
    local spark_width=$((COLS - LABEL_WIDTH - 1))
    if [ "$spark_width" -lt 1 ]; then spark_width=1; fi

    # Only show the last spark_width values
    local total=${#values[@]}
    local start=0
    if [ "$total" -gt "$spark_width" ]; then
        start=$((total - spark_width))
    fi

    local drawn=0
    local i
    for (( i = start; i < total; i++ )); do
        local v="${values[$i]}"
        local ch
        ch=$(spark_char "$v" "$max")
        local color
        color=$(spark_color "$cfunc" "$v")
        printf '%s%s%s' "$color" "$ch" "$RST"
        drawn=$((drawn + 1))
    done

    # Pad remaining columns so line fills exactly to COLS
    if [ "$drawn" -lt "$spark_width" ]; then
        local pad=$((spark_width - drawn))
        printf "%${pad}s" ""
    fi

    printf '\n'
}

# ---------------------------------------------------------------------------
# spark_color — return ANSI color for a sparkline character
# ---------------------------------------------------------------------------
spark_color() {
    local cfunc="$1" val="$2"
    case "$cfunc" in
        spawn)
            if [ "$val" -ge "$SPAWN_CRIT" ]; then
                printf '%s' "$FG_RED"
            elif [ "$val" -ge "$SPAWN_WARN" ]; then
                printf '%s' "$FG_YELLOW"
            else
                printf '%s' "$FG_GREEN"
            fi
            ;;
        death)
            printf '%s' "$FG_GRAY"
            ;;
        net)
            # Use absolute value for threshold comparison
            local abs_val="$val"
            if [ "$abs_val" -lt 0 ]; then abs_val=$(( -abs_val )); fi
            if [ "$abs_val" -ge "$NET_CRIT" ]; then
                printf '%s' "$FG_RED"
            elif [ "$abs_val" -ge "$NET_WARN" ]; then
                printf '%s' "$FG_YELLOW"
            else
                printf '%s' "$FG_GREEN"
            fi
            ;;
    esac
}

# ---------------------------------------------------------------------------
# render — full redraw
# ---------------------------------------------------------------------------
render() {
    local curr_spawns="${1:-0}" curr_deaths="${2:-0}" curr_net="${3:-0}"

    render_sparkline_row "+spawns" "spawn" "$SPARK_MAX" "${spawn_history[@]}"
    render_sparkline_row "-deaths" "death" "$SPARK_MAX" "${death_history[@]}"
    render_sparkline_row " net"    "net"   "$SPARK_MAX" "${net_history[@]}"

    printf '\n'

    # Summary line
    local sc dc nc
    sc=$(threshold_color "$curr_spawns" "$SPAWN_WARN" "$SPAWN_CRIT")
    dc="$FG_GRAY"
    nc=$(threshold_color "${curr_net#-}" "$NET_WARN" "$NET_CRIT")
    local pc
    pc=$(threshold_color "$peak_net" "$NET_WARN" "$NET_CRIT")

    local net_sign=""
    if [ "$curr_net" -ge 0 ] 2>/dev/null; then net_sign="+"; fi

    local peak_sign=""
    if [ "$peak_net" -ge 0 ] 2>/dev/null; then peak_sign="+"; fi

    # Compact summary — fits narrow panes
    printf '  %s+%d/s%s %s-%d/s%s %snet:%s%d/s%s %spk:%s%d%s\n' \
        "$sc" "$curr_spawns" "$RST" \
        "$dc" "$curr_deaths" "$RST" \
        "$nc" "$net_sign" "$curr_net" "$RST" \
        "$pc" "$peak_sign" "$peak_net" "$RST"
}

# ---------------------------------------------------------------------------
# Main loop — tail JSONL data and update sparklines
# ---------------------------------------------------------------------------
main() {
    # Hide cursor, restore on exit
    printf '\033[?25l'
    trap 'printf "\033[?25h"; exit 0' INT TERM EXIT

    local prev_line=""
    local curr_line=""

    # Clear once on startup
    clear

    # Tail the data file, processing each new JSONL line
    while IFS= read -r line; do
        prev_line="$curr_line"
        curr_line="$line"

        # Compute deltas
        eval "$(compute_deltas "$prev_line" "$curr_line")"

        # Append to history
        spawn_history+=("$_spawns")
        death_history+=("$_deaths")

        # For net, store absolute value for sparkline but keep signed for display
        local abs_net="$_net"
        if [ "$abs_net" -lt 0 ]; then abs_net=$(( -abs_net )); fi
        net_history+=("$abs_net")

        # Track peak net
        if [ "$_net" -gt "$peak_net" ]; then
            peak_net="$_net"
        fi

        # Cap history length to prevent unbounded growth
        local max_hist=$((COLS * 2))
        if [ "${#spawn_history[@]}" -gt "$max_hist" ]; then
            spawn_history=("${spawn_history[@]:1}")
            death_history=("${death_history[@]:1}")
            net_history=("${net_history[@]:1}")
        fi

        # Cache tput cols before subshell capture (tput fails inside $())
        COLS=$(tput cols 2>/dev/null || echo 60)
        # Buffered render: capture frame, paint in one shot — no flash
        local frame
        frame=$(render "$_spawns" "$_deaths" "$_net")
        tput home
        printf '%s' "$frame"
        tput ed
    done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main "$@"
