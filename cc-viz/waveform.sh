#!/usr/bin/env bash
# cc-viz/waveform.sh — Braille waveform visualization of spawn/death rates
# Overlaid oscilloscope traces: green=spawns, red=deaths, yellow=overlap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=cc-viz/common.sh
source "$SCRIPT_DIR/common.sh"

# ---------------------------------------------------------------------------
# Terminal geometry (cached before subshell)
# ---------------------------------------------------------------------------
COLS=$(tput cols 2>/dev/null || echo 60)
ROWS=$(tput lines 2>/dev/null || echo 24)

# ---------------------------------------------------------------------------
# Ring buffer — parallel arrays for spawn and death values
# Uses eval-based indexing for bash 3.2 compat (no associative arrays).
# ---------------------------------------------------------------------------
RING_SIZE=0
RING_COUNT=0
RING_HEAD=0       # next write position

recalc_ring() {
    # Each braille char covers 2 data points (left+right columns),
    # so chart_width chars need chart_width * 2 slots.
    local chart_width=$(( COLS - 4 ))  # 2 border + 2 padding
    if (( chart_width < 1 )); then chart_width=1; fi
    RING_SIZE=$(( chart_width * 2 ))
    if (( RING_COUNT > RING_SIZE )); then RING_COUNT=$RING_SIZE; fi
}

ring_push() {
    local spawns="$1" deaths="$2"
    eval "RING_S_${RING_HEAD}=$spawns"
    eval "RING_D_${RING_HEAD}=$deaths"
    RING_HEAD=$(( (RING_HEAD + 1) % RING_SIZE ))
    if (( RING_COUNT < RING_SIZE )); then
        RING_COUNT=$(( RING_COUNT + 1 ))
    fi
}

# Emit ring buffer contents to stdout, one "spawn death" pair per line,
# oldest first. Used to pipe into awk.
ring_dump() {
    local start=$(( (RING_HEAD - RING_COUNT + RING_SIZE) % RING_SIZE ))
    local i idx s d
    for (( i = 0; i < RING_COUNT; i++ )); do
        idx=$(( (start + i) % RING_SIZE ))
        eval "s=\${RING_S_${idx}:-0}"
        eval "d=\${RING_D_${idx}:-0}"
        printf '%d %d\n' "$s" "$d"
    done
}

# ---------------------------------------------------------------------------
# Peak tracking
# ---------------------------------------------------------------------------
peak_spawn=0
peak_death=0

# ---------------------------------------------------------------------------
# render — produce the full frame to stdout
#
# Arguments: $1=curr_spawns $2=curr_deaths $3=curr_net
# Uses global: COLS, ROWS, RING_COUNT, ring buffer
# ---------------------------------------------------------------------------
render() {
    local curr_spawns="${1:-0}" curr_deaths="${2:-0}" curr_net="${3:-0}"

    local chart_width=$(( COLS - 4 ))
    if (( chart_width < 1 )); then chart_width=1; fi
    local chart_rows=$(( ROWS - 3 ))  # header + legend + border padding
    if (( chart_rows < 1 )); then chart_rows=1; fi

    # ── Header line ──
    local net_sign=""
    if (( curr_net >= 0 )); then net_sign="+"; fi
    local nc
    local abs_net=$curr_net
    if (( abs_net < 0 )); then abs_net=$(( -abs_net )); fi
    nc=$(threshold_color "$abs_net" "$NET_WARN" "$NET_CRIT")

    local sc
    sc=$(threshold_color "$curr_spawns" "$SPAWN_WARN" "$SPAWN_CRIT")

    printf ' %s%s RATE%s  %s+%d/s%s  %s-%d/s%s  %snet:%s%d%s' \
        "$BOLD" "$FG_CYAN" "$RST" \
        "$sc" "$curr_spawns" "$RST" \
        "$FG_RED" "$curr_deaths" "$RST" \
        "$nc" "$net_sign" "$curr_net" "$RST"

    # Pad header to full width
    local header_visible_len=$(( 6 + 1 + ${#curr_spawns} + 3 + 1 + ${#curr_deaths} + 3 + 4 + 1 + ${#net_sign} + ${#curr_net} ))
    local hpad=$(( COLS - header_visible_len ))
    if (( hpad > 0 )); then
        printf "%${hpad}s" ""
    fi
    printf '\n'

    # ── Braille waveform chart (via awk) ──
    ring_dump | awk -v rows="$chart_rows" -v width="$chart_width" -v capacity="$RING_SIZE" '
    BEGIN {
        n = 0
        split("1 2 4 64", left_bits)
        split("8 16 32 128", right_bits)

        green  = "\033[32m"
        red    = "\033[31m"
        yellow = "\033[33m"
        dim    = "\033[2m"
        reset  = "\033[0m"
    }
    {
        sv[n] = $1 + 0
        dv[n] = $2 + 0
        n++
    }
    END {
        data_cap = width * 2

        # Right-align: pad with zeros on the left
        for (i = 0; i < data_cap; i++) {
            if (i < data_cap - n) {
                sd[i] = 0
                dd[i] = 0
            } else {
                sd[i] = sv[i - (data_cap - n)]
                dd[i] = dv[i - (data_cap - n)]
            }
        }

        # Auto-scale: find peak in visible window
        peak = 1
        for (i = 0; i < data_cap; i++) {
            if (sd[i] > peak) peak = sd[i]
            if (dd[i] > peak) peak = dd[i]
        }

        total_dots = rows * 4

        # Render row by row (top to bottom)
        for (r = 0; r < rows; r++) {
            line = ""
            # Left padding
            line = line "  "

            for (c = 0; c < width; c++) {
                vl_s = sd[c * 2]
                vr_s = sd[c * 2 + 1]
                vl_d = dd[c * 2]
                vr_d = dd[c * 2 + 1]

                # Map values to dot-row heights
                dy_ls = int(vl_s * total_dots / peak)
                dy_rs = int(vr_s * total_dots / peak)
                dy_ld = int(vl_d * total_dots / peak)
                dy_rd = int(vr_d * total_dots / peak)

                base = (rows - 1 - r) * 4

                bits = 0
                spawn_bits = 0
                death_bits = 0

                for (dot = 0; dot < 4; dot++) {
                    dot_row = base + dot
                    lb = left_bits[dot + 1]
                    rb = right_bits[dot + 1]

                    # Spawn trace (filled area)
                    if (dot_row < dy_ls) {
                        bits += lb
                        spawn_bits += lb
                    }
                    if (dot_row < dy_rs) {
                        bits += rb
                        spawn_bits += rb
                    }

                    # Death trace (filled area) — only add bits not already set
                    if (dot_row < dy_ld) {
                        if (dot_row >= dy_ls) {
                            bits += lb
                        }
                        death_bits += lb
                    }
                    if (dot_row < dy_rd) {
                        if (dot_row >= dy_rs) {
                            bits += rb
                        }
                        death_bits += rb
                    }
                }

                codepoint = 10240 + bits

                # Bottom row empty: show subtle canvas marker
                if (bits == 0 && r == rows - 1) {
                    codepoint = 10242  # U+2802
                }

                # Color: yellow if overlap, else dominant trace color
                if (bits == 0) {
                    col = dim green
                } else if (spawn_bits > 0 && death_bits > 0) {
                    yellow_flag = 0
                    # Check for actual overlap (both traces set in same dot positions)
                    for (dot = 0; dot < 4; dot++) {
                        dot_row = base + dot
                        if (dot_row < dy_ls && dot_row < dy_ld) yellow_flag = 1
                        if (dot_row < dy_rs && dot_row < dy_rd) yellow_flag = 1
                    }
                    if (yellow_flag) {
                        col = yellow
                    } else {
                        # Both present but no overlap: color by dominant
                        max_s = vl_s; if (vr_s > max_s) max_s = vr_s
                        max_d = vl_d; if (vr_d > max_d) max_d = vr_d
                        if (max_d > max_s) col = red
                        else col = green
                    }
                } else if (death_bits > 0) {
                    col = red
                } else {
                    col = green
                }

                # UTF-8 encode (BMP: 3 bytes)
                b1 = 224 + int(codepoint / 4096)
                b2 = 128 + int((codepoint % 4096) / 64)
                b3 = 128 + (codepoint % 64)

                line = line col sprintf("%c%c%c", b1, b2, b3)
            }
            line = line reset
            printf "%s\n", line
        }
    }'

    # ── Legend line ──
    printf '  %s●%s spawns  %s●%s deaths' \
        "$FG_GREEN" "$RST" "$FG_RED" "$RST"

    # Pad legend to full width
    local legend_visible_len=$(( 2 + 1 + 8 + 1 + 7 ))
    local lpad=$(( COLS - legend_visible_len ))
    if (( lpad > 0 )); then
        printf "%${lpad}s" ""
    fi
    printf '\n'
}

# ---------------------------------------------------------------------------
# Main loop — tail JSONL data, compute deltas, render waveform
# ---------------------------------------------------------------------------
main() {
    # Hide cursor, restore on exit
    printf '\033[?25l'
    trap 'printf "\033[?25h"; exit 0' INT TERM EXIT

    local prev_line=""
    local curr_line=""

    # Clear once on startup
    clear

    COLS=$(tput cols 2>/dev/null || echo 60)
    ROWS=$(tput lines 2>/dev/null || echo 24)
    recalc_ring

    # Tail the data file, processing each new JSONL line
    while IFS= read -r line; do
        prev_line="$curr_line"
        curr_line="$line"

        # Compute deltas
        eval "$(compute_deltas "$prev_line" "$curr_line")"

        # Push into ring buffer
        ring_push "$_spawns" "$_deaths"

        # Track peaks
        if (( _spawns > peak_spawn )); then peak_spawn=$_spawns; fi
        if (( _deaths > peak_death )); then peak_death=$_deaths; fi

        # Cap history on terminal resize
        COLS=$(tput cols 2>/dev/null || echo 60)
        ROWS=$(tput lines 2>/dev/null || echo 24)
        recalc_ring

        # Buffered render: capture frame, paint in one shot — no flash
        local frame
        frame=$(render "$_spawns" "$_deaths" "$_net")
        tput home
        printf '%s' "$frame"
        tput ed
    done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main "$@"
