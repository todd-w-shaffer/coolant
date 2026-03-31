#!/usr/bin/env bash
# cc-viz/breakdown.sh — Process Type Breakdown pane
# Horizontal bar chart of currently alive processes grouped by type.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=cc-viz/common.sh
source "$SCRIPT_DIR/common.sh"

# ---------------------------------------------------------------------------
# Bar characters
# ---------------------------------------------------------------------------
BAR_FULL="█"
BAR_EMPTY="░"

# ---------------------------------------------------------------------------
# Layout constants
# ---------------------------------------------------------------------------
LABEL_COL=4       # "  N " — 2 space indent + letter + space
COUNT_COL=6       # right-aligned count + trailing space

# ---------------------------------------------------------------------------
# count_types — parse JSONL line, output "TYPE COUNT" pairs sorted desc
# Usage: count_types "$line"
# ---------------------------------------------------------------------------
count_types() {
    local line="$1"
    printf '%s' "$line" | jq -r '
        [.procs[].type] | group_by(.) |
        map({type: .[0], count: length}) |
        sort_by(-.count) |
        .[] | "\(.type) \(.count)"
    ' 2>/dev/null
}

# ---------------------------------------------------------------------------
# render — full redraw of breakdown chart
# ---------------------------------------------------------------------------
render() {
    local line="$1"

    # Parse type counts into parallel arrays (bash 3.2 compatible)
    local types=""
    local counts=""
    local max_count=0
    local total=0
    local num_types=0

    local pair
    while IFS= read -r pair; do
        if [ -z "$pair" ]; then continue; fi
        local t="${pair%% *}"
        local c="${pair##* }"
        types="$types $t"
        counts="$counts $c"
        if [ "$c" -gt "$max_count" ]; then
            max_count="$c"
        fi
        total=$((total + c))
        num_types=$((num_types + 1))
    done <<EOF
$(count_types "$line")
EOF

    # Available width for bars (use cached COLS from main loop)
    local term_width="$COLS"
    local bar_width=$((term_width - LABEL_COL - COUNT_COL))
    if [ "$bar_width" -lt 1 ]; then bar_width=1; fi

    # Render each type row
    local i=1
    for t in $types; do
        local c
        c=$(echo "$counts" | cut -d' ' -f$((i + 1)))

        local color
        color=$(get_type_color "$t")

        # Compute bar length proportional to max_count
        local filled=0
        if [ "$max_count" -gt 0 ]; then
            filled=$(( (c * bar_width) / max_count ))
            if [ "$filled" -lt 1 ] && [ "$c" -gt 0 ]; then
                filled=1
            fi
        fi
        local empty=$((bar_width - filled))

        # Label
        printf '  %s%s%s ' "$color" "$t" "$RST"

        # Filled bar
        local j
        for (( j = 0; j < filled; j++ )); do
            printf '%s%s%s' "$color" "$BAR_FULL" "$RST"
        done

        # Empty bar
        printf '%s' "$FG_GRAY"
        for (( j = 0; j < empty; j++ )); do
            printf '%s' "$BAR_EMPTY"
        done
        printf '%s' "$RST"

        # Right-aligned count
        printf ' %*d\n' "$((COUNT_COL - 2))" "$c"

        i=$((i + 1))
    done

    # Footer
    printf '\n'
    local total_color
    total_color=$(threshold_color "$total" "$TOTAL_WARN" "$TOTAL_CRIT")
    printf '  %stotal: %d%s    types: %d\n' \
        "$total_color" "$total" "$RST" "$num_types"
}

# ---------------------------------------------------------------------------
# Main loop — tail JSONL data and update chart
# ---------------------------------------------------------------------------
COLS=60

main() {
    # Hide cursor, restore on exit
    printf '\033[?25l'
    trap 'printf "\033[?25h"; exit 0' INT TERM EXIT

    # Clear once on startup
    clear
    printf '  %swaiting for data...%s\n' "$FG_GRAY" "$RST"

    # Tail the data file, processing each new JSONL line
    tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null | while IFS= read -r line; do
        if [ -z "$line" ]; then continue; fi
        # Cache tput cols before subshell capture (tput fails inside $())
        COLS=$(tput cols 2>/dev/null || echo 60)
        # Buffered render: capture frame, paint in one shot — no flash
        local frame
        frame=$(render "$line")
        tput home
        printf '%s' "$frame"
        tput ed
    done
}

main "$@"
