#!/usr/bin/env bash
# cc-viz/waterfall.sh — Process Lifetime Waterfall pane
# Horizontal swim-lane bars showing individual process lifetimes,
# colored by type with age-based intensity. Oldest at top, newest at bottom.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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
LABEL_COL=4        # "  N " — 2-space indent + type letter + space
HEADER_ROWS=1      # header line
FOOTER_ROWS=1      # bottom border / overflow notice

# ---------------------------------------------------------------------------
# age_style — return ANSI prefix for age-based intensity
# Usage: style=$(age_style "$age")
# ---------------------------------------------------------------------------
age_style() {
    local age="$1"
    if [ "$age" -le 2 ] 2>/dev/null; then
        printf '%s' "$BOLD"
    elif [ "$age" -le 10 ] 2>/dev/null; then
        printf ''
    elif [ "$age" -le 30 ] 2>/dev/null; then
        printf '%s' "$DIM"
    else
        printf '%s' "$DIM"
    fi
}

# ---------------------------------------------------------------------------
# age_color — return ANSI color for a process given its type and age
# Usage: color=$(age_color "$type" "$age")
# ---------------------------------------------------------------------------
age_color() {
    local t="$1" age="$2"
    local style color
    style=$(age_style "$age")
    if [ "$age" -gt 30 ] 2>/dev/null; then
        color="$FG_GRAY"
    else
        color=$(get_type_color "$t")
    fi
    printf '%s%s' "$style" "$color"
}

# ---------------------------------------------------------------------------
# render — full redraw of waterfall chart
# Globals: COLS, ROWS (set by caller before subshell)
# ---------------------------------------------------------------------------
render() {
    local line="$1"

    # Parse the JSONL snapshot
    eval "$(parse_jsonl_line "$line")"

    local total="$_count"
    local avail_rows=$(( ROWS - HEADER_ROWS - FOOTER_ROWS ))
    if [ "$avail_rows" -lt 1 ]; then avail_rows=1; fi

    # Build sorted index list: age descending (oldest first)
    local order=""
    order=$(
        i=0
        while [ "$i" -lt "$total" ]; do
            printf '%s %s\n' "${_proc_ages[$i]}" "$i"
            i=$((i + 1))
        done | sort -t' ' -k1,1 -rn | while IFS=' ' read -r _sa si; do
            printf '%s ' "$si"
        done
    )

    # Find max age for scaling
    local max_age=1
    for idx in $order; do
        local a="${_proc_ages[$idx]}"
        if [ "$a" -gt "$max_age" ] 2>/dev/null; then
            max_age="$a"
        fi
    done

    # Count total sorted entries
    local num_procs=0
    for _ in $order; do
        num_procs=$((num_procs + 1))
    done

    # Header
    local hdr_color
    hdr_color=$(threshold_color "$total" "$TOTAL_WARN" "$TOTAL_CRIT")
    printf '%s%s WATERFALL %s── %s%d alive%s' \
        "$BOLD" "$FG_WHITE" "$RST" "$hdr_color" "$total" "$RST"
    # Pad header to full width
    local hdr_text_len=$((22 + ${#total}))
    local hdr_pad=$((COLS - hdr_text_len))
    if [ "$hdr_pad" -gt 0 ]; then
        printf '%*s' "$hdr_pad" ""
    fi
    printf '\n'

    # Determine how many to skip (overflow)
    local skip=0
    if [ "$num_procs" -gt "$avail_rows" ]; then
        skip=$((num_procs - avail_rows))
    fi

    # Bar width
    local bar_width=$((COLS - LABEL_COL))
    if [ "$bar_width" -lt 1 ]; then bar_width=1; fi

    # Render rows
    local row=0
    local proc_idx=0
    local shown=0
    for idx in $order; do
        # Skip oldest processes that overflow
        if [ "$proc_idx" -lt "$skip" ]; then
            proc_idx=$((proc_idx + 1))
            continue
        fi

        # Overflow notice on first visible row
        if [ "$proc_idx" -eq "$skip" ] && [ "$skip" -gt 0 ]; then
            printf '  %s(%d more above)%s' "$DIM" "$skip" "$RST"
            local overflow_text_len=$((15 + ${#skip}))
            local overflow_pad=$((COLS - overflow_text_len))
            if [ "$overflow_pad" -gt 0 ]; then
                printf '%*s' "$overflow_pad" ""
            fi
            printf '\n'
            avail_rows=$((avail_rows - 1))
        fi

        if [ "$shown" -ge "$avail_rows" ]; then
            break
        fi

        local t="${_proc_types[$idx]}"
        local a="${_proc_ages[$idx]}"

        # Compute filled bar length proportional to max_age
        local filled=0
        if [ "$max_age" -gt 0 ]; then
            filled=$(( (a * bar_width) / max_age ))
            if [ "$filled" -lt 1 ] && [ "$a" -gt 0 ]; then
                filled=1
            fi
        fi
        local empty=$((bar_width - filled))

        # Color for this process
        local color
        color=$(age_color "$t" "$a")
        local label_color
        label_color=$(get_type_color "$t")

        # Label: 2-space indent + colored type letter + space
        printf '  %s%s%s ' "$label_color" "$t" "$RST"

        # Filled portion
        local j
        for (( j = 0; j < filled; j++ )); do
            printf '%s%s%s' "$color" "$BAR_FULL" "$RST"
        done

        # Empty portion
        printf '%s' "$FG_GRAY"
        for (( j = 0; j < empty; j++ )); do
            printf '%s' "$BAR_EMPTY"
        done
        printf '%s\n' "$RST"

        shown=$((shown + 1))
        proc_idx=$((proc_idx + 1))
    done

    # Blank-fill remaining rows so old content is overwritten
    while [ "$shown" -lt "$avail_rows" ]; do
        printf '%*s\n' "$COLS" ""
        shown=$((shown + 1))
    done
}

# ---------------------------------------------------------------------------
# Main loop — tail JSONL data and update waterfall
# ---------------------------------------------------------------------------
COLS=60
ROWS=40

main() {
    # Hide cursor, restore on exit
    printf '\033[?25l'
    trap 'printf "\033[?25h"; exit 0' INT TERM EXIT

    # Clear once on startup
    clear
    printf '  %swaiting for data...%s\n' "$FG_GRAY" "$RST"

    # Tail the data file, processing each new JSONL line
    while IFS= read -r line; do
        if [ -z "$line" ]; then continue; fi
        # Cache terminal size before subshell capture
        COLS=$(tput cols 2>/dev/null || echo 60)
        ROWS=$(tput lines 2>/dev/null || echo 40)
        # Buffered render: capture frame, paint in one shot
        local frame
        frame=$(render "$line")
        tput home
        printf '%s' "$frame"
        tput ed
    done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main "$@"
