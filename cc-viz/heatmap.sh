#!/usr/bin/env bash
# cc-viz/heatmap.sh — Heatmap spectrogram of process types over time
# Rows = process types (sticky, sorted alphabetically)
# Columns = time ticks, scrolling left
# Cell intensity encodes process count per type per tick.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Terminal geometry ───────────────────────────────────
COLS=$(tput cols 2>/dev/null || echo 120)
ROWS=$(tput lines 2>/dev/null || echo 40)
MARGIN=4             # "  T " — 2-space indent + type letter + space

refresh_geometry() {
  COLS=$(tput cols 2>/dev/null || echo 120)
  ROWS=$(tput lines 2>/dev/null || echo 40)
}
trap 'refresh_geometry' WINCH

# ─── Sticky type registry ───────────────────────────────
# Space-delimited string of unique type letters seen, kept sorted.
# Once a type appears it stays forever (sticky rows).
KNOWN_TYPES=""

register_types() {
  local new_types="$1"
  local letter changed=0
  for letter in $new_types; do
    # Check if already known
    case " $KNOWN_TYPES " in
      *" $letter "*) ;;
      *)
        KNOWN_TYPES="$KNOWN_TYPES $letter"
        changed=1
        ;;
    esac
  done
  if (( changed )); then
    # Re-sort alphabetically
    KNOWN_TYPES=$(printf '%s\n' $KNOWN_TYPES | sort -u | tr '\n' ' ')
    KNOWN_TYPES="${KNOWN_TYPES% }"
  fi
}

# ─── Ring buffer ─────────────────────────────────────────
# Each slot stores a snapshot: space-separated "TYPE:COUNT" pairs.
# e.g. "C:2 G:1 N:5 V:3"
MAX_SNAPS=0
SNAP_COUNT=0
SNAP_HEAD=0

recalc_columns() {
  MAX_SNAPS=$(( COLS - MARGIN ))
  if (( MAX_SNAPS < 1 )); then MAX_SNAPS=1; fi
  if (( SNAP_COUNT > MAX_SNAPS )); then SNAP_COUNT=$MAX_SNAPS; fi
}

snap_push() {
  local counts="$1"
  eval "SNAP_${SNAP_HEAD}=\"\$counts\""
  SNAP_HEAD=$(( (SNAP_HEAD + 1) % MAX_SNAPS ))
  if (( SNAP_COUNT < MAX_SNAPS )); then SNAP_COUNT=$(( SNAP_COUNT + 1 )); fi
}

snap_get() {
  local vis_pos=$1
  local start=$(( (SNAP_HEAD - SNAP_COUNT + MAX_SNAPS) % MAX_SNAPS ))
  local idx=$(( (start + vis_pos) % MAX_SNAPS ))
  eval "echo \"\${SNAP_${idx}:-}\""
}

# ─── Count extraction ────────────────────────────────────
# Given a snapshot string and a type letter, return the count.
# Returns 0 if the type isn't present.
get_count_for_type() {
  local snap="$1" target="$2"
  local pair
  for pair in $snap; do
    case "$pair" in
      "${target}:"*)
        echo "${pair#*:}"
        return
        ;;
    esac
  done
  echo 0
}

# ─── Cell color ──────────────────────────────────────────
# Maps a count to a 256-color ANSI background escape.
cell_bg() {
  local count=$1
  if (( count == 0 )); then
    printf ''  # default terminal background
  elif (( count == 1 )); then
    printf '\033[48;5;232m'
  elif (( count <= 3 )); then
    printf '\033[48;5;235m'
  elif (( count <= 7 )); then
    printf '\033[48;5;130m'
  elif (( count <= 15 )); then
    printf '\033[48;5;208m'
  else
    printf '\033[48;5;196m'
  fi
}

# ─── Parse snapshot from JSONL ───────────────────────────
# Extracts type letters, counts occurrences of each, returns
# space-separated "TYPE:COUNT" pairs via jq (one subshell).
snapshot_counts() {
  local line="$1"
  [[ -z "$line" ]] && { echo ""; return; }

  # Let jq do the heavy lifting: group by type, count, emit "T:N" pairs
  printf '%s' "$line" | jq -r '
    [.procs[].type] | group_by(.) |
    map("\(.[0]):\(length)") | join(" ")
  ' 2>/dev/null
}

# ─── Rendering ───────────────────────────────────────────
render() {
  local visible=$SNAP_COUNT
  local type_letter type_color_code count

  # Pre-fetch all visible snapshots into COL_<n> vars (one eval per column,
  # not per cell) to avoid repeated subshells inside the inner loop.
  local col
  for (( col = 0; col < visible; col++ )); do
    local start=$(( (SNAP_HEAD - SNAP_COUNT + MAX_SNAPS) % MAX_SNAPS ))
    local idx=$(( (start + col) % MAX_SNAPS ))
    eval "COL_${col}=\"\${SNAP_${idx}:-}\""
  done

  # One row per known type
  for type_letter in $KNOWN_TYPES; do
    type_color_code=$(get_type_color "$type_letter")
    # Left margin: "  T "
    printf '  %s%s%s ' "$type_color_code" "$type_letter" "$RST"

    # One cell per visible tick — inline count lookup to avoid subshells
    for (( col = 0; col < visible; col++ )); do
      local snap pair
      eval "snap=\"\${COL_${col}}\""
      count=0
      for pair in $snap; do
        case "$pair" in
          "${type_letter}:"*)
            count="${pair#*:}"
            break
            ;;
        esac
      done

      # Inline cell_bg to avoid subshell per cell
      if (( count == 0 )); then
        printf ' '
      elif (( count == 1 )); then
        printf '\033[48;5;232m \033[0m'
      elif (( count <= 3 )); then
        printf '\033[48;5;235m \033[0m'
      elif (( count <= 7 )); then
        printf '\033[48;5;130m \033[0m'
      elif (( count <= 15 )); then
        printf '\033[48;5;208m \033[0m'
      else
        printf '\033[48;5;196m \033[0m'
      fi
    done

    # Pad remainder of row
    local used=$(( MARGIN + visible ))
    local pad=$(( COLS - used ))
    if (( pad > 0 )); then
      printf '%*s' "$pad" ""
    fi
    printf '\n'
  done

  # Fill remaining rows with blanks so tput ed clears cleanly
  local type_count=0
  for _ in $KNOWN_TYPES; do type_count=$(( type_count + 1 )); done
  local remaining=$(( ROWS - type_count ))
  local r
  for (( r = 0; r < remaining; r++ )); do
    printf '%*s\n' "$COLS" ""
  done
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

    # Parse JSONL into type:count pairs and register any new types
    local counts types_only
    counts=$(snapshot_counts "$line")

    # Extract just the type letters for registration
    types_only=""
    local pair
    for pair in $counts; do
      types_only="$types_only ${pair%%:*}"
    done
    register_types "${types_only# }"

    snap_push "$counts"

    # Buffered render: capture frame, paint in one shot
    local frame
    frame=$(render)
    tput home
    printf '%s' "$frame"
    tput ed
  done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main
