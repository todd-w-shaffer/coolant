#!/usr/bin/env bash
# cc-viz skyline — vertical stack view of live process counts
# Shows one column per tick, scrolling left to right.
# Each column is a bottom-to-top stack of type labels, colored by type.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Terminal geometry ───────────────────────────────────
COLS=$(tput cols 2>/dev/null || echo 120)
ROWS=$(tput lines 2>/dev/null || echo 40)
COL_WIDTH=3          # type letter + space + separator

refresh_geometry() {
  COLS=$(tput cols 2>/dev/null || echo 120)
  ROWS=$(tput lines 2>/dev/null || echo 40)
}
trap 'refresh_geometry' WINCH

# ─── Ring buffer ─────────────────────────────────────────
# Each slot holds a snapshot: space-separated sorted type letters.
# e.g. "C G N N S V" means 6 procs of those types.
MAX_SNAPS=0
SNAP_COUNT=0
# Arrays indexed 0..MAX_SNAPS-1 (circular)
SNAP_HEAD=0          # next write position

recalc_columns() {
  MAX_SNAPS=$(( COLS / COL_WIDTH ))
  if (( MAX_SNAPS < 1 )); then MAX_SNAPS=1; fi
  # If buffer shrunk, adjust count
  if (( SNAP_COUNT > MAX_SNAPS )); then SNAP_COUNT=$MAX_SNAPS; fi
}

snap_push() {
  local types="$1"
  eval "SNAP_${SNAP_HEAD}=\"\$types\""
  SNAP_HEAD=$(( (SNAP_HEAD + 1) % MAX_SNAPS ))
  if (( SNAP_COUNT < MAX_SNAPS )); then SNAP_COUNT=$(( SNAP_COUNT + 1 )); fi
}

snap_get() {
  # Return snapshot at visual position $1 (0 = oldest visible)
  local vis_pos=$1
  local start=$(( (SNAP_HEAD - SNAP_COUNT + MAX_SNAPS) % MAX_SNAPS ))
  local idx=$(( (start + vis_pos) % MAX_SNAPS ))
  eval "echo \"\${SNAP_${idx}:-}\""
}

# ─── Type colors ─────────────────────────────────────────
type_color() {
  case "$1" in
    N) get_type_color "N" ;;
    G) get_type_color "G" ;;
    V) get_type_color "V" ;;
    S) get_type_color "S" ;;
    R) get_type_color "R" ;;
    F) get_type_color "F" ;;
    C) get_type_color "C" ;;
    P) get_type_color "P" ;;
    T) get_type_color "T" ;;
    X) get_type_color "X" ;;
    *) printf '\033[37m' ;;
  esac
}

# ─── Parse a snapshot from JSONL ─────────────────────────
# Given a JSONL line, extracts type letters,
# sorts them alphabetically, returns space-separated.
snapshot_types() {
  local line="$1"
  [[ -z "$line" ]] && { echo ""; return; }

  # eval the parse output to get _proc_types array
  eval "$(parse_jsonl_line "$line")"
  # Sort types alphabetically for stable column order
  local sorted
  sorted=$(printf '%s\n' ${_proc_types[*]} | sort | tr '\n' ' ')
  echo "${sorted% }"
}

# ─── Rendering (use colors from common.sh) ──────────────
BLD="$BOLD"
WHT="$FG_WHITE"
YLW="$FG_YELLOW"
RED="$FG_RED"

render() {
  local max_height=$(( ROWS - 2 ))  # separator + footer
  if (( max_height < 1 )); then max_height=1; fi
  local visible=$SNAP_COUNT

  # Build column data: for each visible snapshot, split into array of types
  # Column c has types in COLTYPES_c_0, COLTYPES_c_1, ... COLTYPES_c_count
  local c t_count
  for (( c = 0; c < visible; c++ )); do
    local snap
    snap=$(snap_get "$c")
    local i=0
    if [[ -n "$snap" ]]; then
      local letter
      for letter in $snap; do
        eval "COLTYPES_${c}_${i}=\"\$letter\""
        (( i++ ))
      done
    fi
    eval "COLCOUNT_${c}=$i"
  done

  # Render rows top to bottom. Row 0 = top of stack area.
  local row col col_count letter color
  for (( row = 0; row < max_height; row++ )); do
    local line_buf=""
    # Which stack position does this row represent? (bottom = 0)
    local stack_pos=$(( max_height - 1 - row ))

    for (( col = 0; col < visible; col++ )); do
      eval "col_count=\${COLCOUNT_${col}}"

      if (( stack_pos < col_count )); then
        # Check if truncated: more items than rows, and this is the top row
        if (( col_count > max_height && row == 0 )); then
          line_buf="${line_buf}${DIM}^${RST}  "
        else
          # Adjust stack_pos if truncated: skip items above max_height
          local actual_pos=$stack_pos
          if (( col_count > max_height )); then
            actual_pos=$(( stack_pos + (col_count - max_height) ))
          fi
          eval "letter=\"\${COLTYPES_${col}_${actual_pos}}\""
          color=$(type_color "$letter")
          line_buf="${line_buf}${color}${letter}${RST}  "
        fi
      else
        line_buf="${line_buf}   "
      fi
    done

    # Pad remainder of terminal width with spaces
    local used_width=$(( visible * COL_WIDTH ))
    local pad=$(( COLS - used_width ))
    if (( pad < 0 )); then pad=0; fi
    printf '%s%*s\n' "$line_buf" "$pad" ""
  done

  # Separator line
  local sep=""
  local i
  for (( i = 0; i < COLS; i++ )); do
    sep="${sep}─"
  done
  printf '%s%s%s\n' "${DIM}${WHT}" "$sep" "${RST}"

  # Footer: total count per column, right-aligned in 3-char cell
  local footer=""
  for (( col = 0; col < visible; col++ )); do
    eval "col_count=\${COLCOUNT_${col}}"
    local count_color="${BLD}${WHT}"
    if (( col_count >= 50 )); then
      count_color="${BLD}${RED}"
    elif (( col_count >= 20 )); then
      count_color="${BLD}${YLW}"
    fi
    footer="${footer}$(printf '%s%2d%s ' "$count_color" "$col_count" "$RST")"
  done
  local footer_used=$(( visible * COL_WIDTH ))
  local footer_pad=$(( COLS - footer_used ))
  if (( footer_pad < 0 )); then footer_pad=0; fi
  printf '%s%*s' "$footer" "$footer_pad" ""
}

# ─── Cleanup ─────────────────────────────────────────────
cleanup() {
  tput cnorm 2>/dev/null
  printf '%s\n' "$RST"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# ─── Main loop ───────────────────────────────────────────

main() {
  tput civis 2>/dev/null
  clear
  recalc_columns

  local prev_cols=$COLS

  while IFS= read -r line; do
    [[ -z "$line" ]] && continue

    refresh_geometry
    recalc_columns

    if (( COLS != prev_cols )); then
      clear
      prev_cols=$COLS
    fi

    local types
    types=$(snapshot_types "$line")
    snap_push "$types"

    # Buffered render: capture frame, paint in one shot — no flash
    local frame
    frame=$(render)
    tput home
    printf '%s' "$frame"
    tput ed
  done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main
