#!/usr/bin/env bash
# Sparkline chart rendering for coolant monitor
# Sourced by monitor.sh — not run directly.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Colors (duplicated from monitor for standalone sourcing) ──
_SP_RST=$'\033[0m'
_SP_DIM=$'\033[2m'
_SP_BLD=$'\033[1m'
_SP_CYN=$'\033[36m'

# ─── History management ─────────────────────────────────────────

# history_push ARRAY_NAME value max_length
#   Appends value to the named array. Trims from the front if over max_length.
#   Uses bash 3.2 compatible nameref-free approach (eval).
history_push() {
  local _arr_name="$1"
  local _val="$2"
  local _max="$3"

  eval "${_arr_name}+=(\"\$_val\")"
  local _len
  eval "_len=\${#${_arr_name}[@]}"
  if (( _len > _max )); then
    eval "${_arr_name}=(\"\${${_arr_name}[@]:$((_len - _max))}\")"
  fi
}

# ─── Sparkline chart renderer ────────────────────────────────────

# sparkline_chart rows width val1 val2 val3 ...
#   Renders a filled-area braille sparkline chart.
#   - rows: number of character rows (each = 4 dot-rows)
#   - width: number of character columns (each = 2 data points)
#   - remaining args: data values (0-100), right-aligned in the chart
#   Output: $rows lines of colored braille characters to stdout.
sparkline_chart() {
  local rows="$1" width="$2"
  shift 2

  # Pipe values to awk, one per line
  local i
  for i in "$@"; do
    printf '%s\n' "$i"
  done | awk -v rows="$rows" -v width="$width" '
  BEGIN {
    n = 0
    max_val = 100
    # Braille dot bit positions
    # Left column:  row0=0x01, row1=0x02, row2=0x04, row3=0x40
    # Right column: row0=0x08, row1=0x10, row2=0x20, row3=0x80
    split("1 2 4 64", left_bits)
    split("8 16 32 128", right_bits)

    # ANSI color codes
    green = "\033[32m"
    yellow = "\033[33m"
    red = "\033[31m"
    dim = "\033[2m"
    reset = "\033[0m"
  }
  {
    v[n] = $1 + 0
    n++
  }
  END {
    capacity = width * 2

    # Right-align: pad with zeros on the left if fewer values than capacity
    for (i = 0; i < capacity; i++) {
      if (i < capacity - n) {
        d[i] = 0
      } else {
        d[i] = v[i - (capacity - n)]
      }
    }

    total_dots = rows * 4

    # Render row by row (top to bottom)
    for (r = 0; r < rows; r++) {
      line = ""
      for (c = 0; c < width; c++) {
        vl = d[c * 2]
        vr = d[c * 2 + 1]

        # Map values to dot-row count (0 = no dots, total_dots = all dots)
        if (max_val > 0) {
          dy_l = int(vl * total_dots / max_val)
          dy_r = int(vr * total_dots / max_val)
        } else {
          dy_l = 0
          dy_r = 0
        }

        # Which dot-rows does this character row cover?
        # Row 0 = top, row (rows-1) = bottom
        # Dot-row 0 = bottom of chart, total_dots-1 = top
        base = (rows - 1 - r) * 4

        bits = 0
        for (dot = 0; dot < 4; dot++) {
          dot_row = base + dot
          # Filled area: light dot if dot_row < value height
          if (dot_row < dy_l) {
            bits += left_bits[dot + 1]
          }
          if (dot_row < dy_r) {
            bits += right_bits[dot + 1]
          }
        }

        codepoint = 10240 + bits  # 0x2800 = 10240

        # UTF-8 encode (BMP: 3 bytes)
        b1 = 224 + int(codepoint / 4096)
        b2 = 128 + int((codepoint % 4096) / 64)
        b3 = 128 + (codepoint % 64)

        # Zone color based on max of the two values
        zone_val = vl
        if (vr > zone_val) zone_val = vr
        if (bits == 0) {
          # Empty cell — dim to show the chart canvas
          col = dim green
        } else if (zone_val >= 70) {
          col = red
        } else if (zone_val >= 50) {
          col = yellow
        } else {
          col = green
        }

        # For empty cells on bottom row, show a subtle dot grid
        if (bits == 0 && r == rows - 1) {
          # Show ⠂ (U+2802 = 10242) as subtle canvas marker
          b1 = 224 + int(10242 / 4096)
          b2 = 128 + int((10242 % 4096) / 64)
          b3 = 128 + (10242 % 64)
        }

        line = line col sprintf("%c%c%c", b1, b2, b3)
      }
      line = line reset
      printf "%s\n", line
    }
  }'
}

# ─── Box-drawing frame helpers ───────────────────────────────────

# box_top title subtitle value width
#   Emits: ┌─ title ─── subtitle ──...── value ──┐
box_top() {
  local title="$1" subtitle="$2" value="$3" width="$4"

  # Build the content portion: ─ title ─── subtitle ──...── value ──
  local left="─ ${title} ─── ${subtitle} "
  local right=" ${value} ──"
  local left_len=${#left}
  local right_len=${#right}

  # Fill middle with ─
  local fill_len=$(( width - 2 - left_len - right_len ))
  (( fill_len < 0 )) && fill_len=0
  local fill=""
  local i
  for (( i = 0; i < fill_len; i++ )); do
    fill="${fill}─"
  done

  printf "${_SP_DIM}${_SP_CYN}┌${_SP_RST}"
  printf "${_SP_CYN}${_SP_BLD}─ %s${_SP_RST}" "$title"
  printf "${_SP_DIM}${_SP_CYN} ─── %s${_SP_RST}" "$subtitle"
  printf "${_SP_DIM}${_SP_CYN}%s${_SP_RST}" "$fill"
  printf "${_SP_BLD} %s${_SP_RST}" "$value"
  printf "${_SP_DIM}${_SP_CYN} ──┐${_SP_RST}"
  printf "\n"
}

# box_bottom width
#   Emits: └──...──┘
box_bottom() {
  local width="$1"
  local inner=$(( width - 2 ))
  local fill=""
  local i
  for (( i = 0; i < inner; i++ )); do
    fill="${fill}─"
  done
  printf "${_SP_DIM}${_SP_CYN}└%s┘${_SP_RST}\n" "$fill"
}

# box_line content width
#   Emits: │content...padded...│
#   content_display_width must be <= width - 2
box_line() {
  local content="$1" width="$2"
  printf "${_SP_DIM}${_SP_CYN}│${_SP_RST}%s${_SP_DIM}${_SP_CYN}│${_SP_RST}\n" "$content"
}
