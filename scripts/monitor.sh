#!/usr/bin/env bash
# coolant monitor — real-time thermal dashboard for Claude Code
# Run in a separate terminal/tmux pane alongside your Claude session.
#
# Usage:
#   bash scripts/monitor.sh              # auto-detect Claude sessions
#   bash scripts/monitor.sh --pid 12345  # monitor specific PID
#   COOLANT_REFRESH=5 bash scripts/monitor.sh  # custom refresh interval
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Config ───────────────────────────────────────────────────
REFRESH="${COOLANT_REFRESH:-2}"
MAX_EVENTS=8
TARGET_PID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pid) TARGET_PID="$2"; shift 2 ;;
    --refresh) REFRESH="$2"; shift 2 ;;
    *) shift ;;
  esac
done

# ─── Colors ───────────────────────────────────────────────────
RST=$'\033[0m'
BLD=$'\033[1m'
DIM=$'\033[2m'
RED=$'\033[31m'
GRN=$'\033[32m'
YLW=$'\033[33m'
CYN=$'\033[36m'
WHT=$'\033[37m'
BGRED=$'\033[41m'
BGYLW=$'\033[43m'
BGGRN=$'\033[42m'

# ─── Terminal setup ───────────────────────────────────────────
cleanup() {
  tput cnorm 2>/dev/null
  printf "%s\n" "$RST"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
tput civis 2>/dev/null

# ─── Rendering helpers ───────────────────────────────────────

cols() { tput cols 2>/dev/null || echo 60; }

hr() {
  local w; w=$(cols)
  printf "${DIM}${CYN}"
  printf '%*s' "$w" '' | tr ' ' '─'
  printf "${RST}\n"
}

section() {
  local w; w=$(cols)
  local title="$1"
  printf "${CYN}${BLD} %s${RST}\n" "$title"
  hr
}

bar() {
  local pct=${1:-0} w=${2:-20} warn=${3:-70} crit=${4:-90}
  (( pct > 100 )) && pct=100
  (( pct < 0 )) && pct=0
  local filled=$(( pct * w / 100 ))
  local empty=$(( w - filled ))
  local c="${GRN}"
  (( pct >= warn )) && c="${YLW}"
  (( pct >= crit )) && c="${RED}"

  # Braille density levels: ⣿ (full) ⣶ (3/4) ⡖ (half) ⠒ (1/4) ⠀ (empty)
  printf "%s" "$c"
  for (( i=0; i<filled; i++ )); do printf '⣿'; done
  printf "%s" "$DIM"
  for (( i=0; i<empty; i++ )); do printf '⠂'; done
  printf "%s" "$RST"
}

pressure_label() {
  local mem_pct=${1:-0} swap_pct=${2:-0}
  if (( mem_pct >= 90 || swap_pct >= 50 )); then
    printf "${BGRED}${WHT}${BLD} CRITICAL ${RST}"
  elif (( mem_pct >= 75 || swap_pct >= 10 )); then
    printf "${BGYLW}${WHT}${BLD} WARN ${RST}"
  else
    printf "${BGGRN}${WHT} NORMAL ${RST}"
  fi
}

# ─── Sensors ──────────────────────────────────────────────────

read_cpu() {
  local load ncpu load1 pct
  load=$(sysctl -n vm.loadavg 2>/dev/null | tr -d '{}' | xargs)
  ncpu=$(sysctl -n hw.ncpu 2>/dev/null || echo 1)
  load1=$(echo "$load" | awk -F' ' '{print $1}')
  pct=$(awk -v l="$load1" -v n="$ncpu" 'BEGIN {p=int((l/n)*100); if(p>100)p=100; print p}')
  CPU_PCT=$pct
  CPU_LOAD="$load"
  CPU_NCPU=$ncpu
}

read_mem() {
  local page_size total_bytes total_mb
  page_size=$(sysctl -n hw.pagesize 2>/dev/null || echo 16384)
  total_bytes=$(sysctl -n hw.memsize 2>/dev/null || echo 0)
  total_mb=$(( total_bytes / 1024 / 1024 ))

  local stats active wired compressed used_pages used_mb
  stats=$(vm_stat 2>/dev/null)
  active=$(echo "$stats" | awk '/Pages active/ {gsub(/\./,"",$NF); print $NF}')
  wired=$(echo "$stats" | awk '/Pages wired/ {gsub(/\./,"",$NF); print $NF}')
  compressed=$(echo "$stats" | awk '/Pages occupied by compressor/ {gsub(/\./,"",$NF); print $NF}')

  used_pages=$(( ${active:-0} + ${wired:-0} + ${compressed:-0} ))
  used_mb=$(( used_pages * page_size / 1024 / 1024 ))
  local pct=0
  (( total_mb > 0 )) && pct=$(( used_mb * 100 / total_mb ))

  MEM_USED=$used_mb
  MEM_TOTAL=$total_mb
  MEM_PCT=$pct
}

read_swap() {
  local line total used
  line=$(sysctl vm.swapusage 2>/dev/null || echo "")
  total=$(echo "$line" | sed 's/.*total = \([0-9.]*\)M.*/\1/' | cut -d. -f1)
  used=$(echo "$line" | sed 's/.*used = \([0-9.]*\)M.*/\1/' | cut -d. -f1)
  local pct=0
  (( ${total:-0} > 0 )) && pct=$(( ${used:-0} * 100 / ${total:-0} ))

  SWAP_USED="${used:-0}"
  SWAP_TOTAL="${total:-0}"
  SWAP_PCT=$pct
}

read_coolant_status() {
  if [[ -f "$COOLANT_LOCKFILE" ]]; then
    COOL_MODE="PARALLEL"
    COOL_AGENTS=$(cat "$COOLANT_COUNTER" 2>/dev/null || echo "0")
  else
    COOL_MODE="OFF"
    COOL_AGENTS=$(cat "$COOLANT_COUNTER" 2>/dev/null || echo "0")
  fi
}

# ─── Process tree ─────────────────────────────────────────────

ALL_PROCS=""

collect_procs() {
  ALL_PROCS=$(ps -Ao pid=,ppid=,%cpu=,rss=,args= 2>/dev/null)
}

find_claude_pids() {
  if [[ -n "$TARGET_PID" ]]; then
    echo "$TARGET_PID"
    return
  fi
  # Match CLI claude processes (not Claude.app helpers)
  ps -eo pid=,comm= 2>/dev/null | awk '$2 == "claude" {print $1}'
}

# Render an entire process tree in a SINGLE awk call.
# No per-node subprocess spawning — avoids fd exhaustion on large trees.
render_tree() {
  local root_pid=$1
  local max_depth=${2:-12}
  echo "$ALL_PROCS" | awk -v root="$root_pid" -v maxd="$max_depth" \
    -v BLD=$'\033[1m' -v DIM=$'\033[2m' -v WHT=$'\033[37m' -v RST=$'\033[0m' '
  {
    pid = $1; ppid = $2; cpu[pid] = $3; rss[pid] = int($4/1024)
    cmd = ""; for (i=5; i<=NF; i++) cmd = cmd (i>5?" ":"") $i
    if (length(cmd) > 45) cmd = substr(cmd, 1, 45)
    command[pid] = cmd
    # Build adjacency list (space-separated children per parent)
    if (children[ppid] == "") children[ppid] = pid
    else children[ppid] = children[ppid] " " pid
  }
  END {
    if (!(root in cpu)) { print "  " DIM "pid " root " not found" RST; exit }

    # Iterative DFS using parallel arrays as a stack
    sp = 1
    s_pid[sp] = root; s_prefix[sp] = ""; s_last[sp] = -1; s_depth[sp] = 0
    count = 0; tcpu = 0; tmem = 0

    while (sp > 0) {
      # Pop
      p     = s_pid[sp]
      pfx   = s_prefix[sp]
      islst = s_last[sp]
      dep   = s_depth[sp]
      sp--

      if (!(p in cpu)) continue
      if (dep > maxd) continue

      count++
      tcpu += cpu[p] + 0
      tmem += rss[p] + 0

      # Determine connector
      if (islst == -1) {
        # Root node
        printf "  %s%d%s  %5s%%  %5dM  %s%s%s\n", BLD, p, RST, cpu[p], rss[p], WHT, command[p], RST
        child_pfx = "  "
      } else if (islst == 1) {
        printf "  %s%s└─%s%-6d %5s%%  %5dM  %s\n", DIM, pfx, RST, p, cpu[p], rss[p], command[p]
        child_pfx = pfx "  "
      } else {
        printf "  %s%s├─%s%-6d %5s%%  %5dM  %s\n", DIM, pfx, RST, p, cpu[p], rss[p], command[p]
        child_pfx = pfx "│ "
      }

      # Collect real children
      n = split(children[p], kids, " ")
      rc = 0
      for (j = 1; j <= n; j++) {
        if (kids[j] != "" && kids[j] != p) { rc++; rkids[rc] = kids[j] }
      }

      # Push children in reverse order so first child is processed first (LIFO)
      for (j = rc; j >= 1; j--) {
        sp++
        s_pid[sp]    = rkids[j]
        s_prefix[sp] = child_pfx
        s_depth[sp]  = dep + 1
        s_last[sp]   = (j == rc) ? 1 : 0
      }
    }

    printf "  %s──────%s\n", DIM, RST
    printf "  %ssubtree%s: %.1f%% cpu  %dM rss  %d procs\n", BLD, RST, tcpu, tmem, count
  }'
}

# ─── Event log ────────────────────────────────────────────────

read_events() {
  if [[ -f "$COOLANT_LOG" ]]; then
    EVENTS=$(tail -n "$MAX_EVENTS" "$COOLANT_LOG" 2>/dev/null || echo "")
  else
    EVENTS=""
  fi
}

# ─── Render ───────────────────────────────────────────────────

render() {
  # Collect all data
  read_cpu
  read_mem
  read_swap
  read_coolant_status
  read_events
  collect_procs

  local claude_pids=()
  while IFS= read -r _cpid; do
    [[ -n "$_cpid" ]] && claude_pids+=("$_cpid")
  done < <(find_claude_pids)

  # Clear and draw
  clear

  # ── Header ──
  local now
  now=$(date '+%H:%M:%S')
  printf "\n"
  if [[ "$COOL_MODE" == "PARALLEL" ]]; then
    printf " ${RED}${BLD}COOLANT${RST}  ${BGYLW}${WHT}${BLD} PARALLEL ${RST}"
  else
    printf " ${CYN}${BLD}COOLANT${RST}  ${DIM}idle${RST}"
  fi
  printf "%*s\n" $(( $(cols) - 22 )) "$now"

  printf " agents: ${BLD}%s${RST}  threshold: %s" \
    "$COOL_AGENTS" "$COOLANT_THRESHOLD"
  printf "\n\n"

  # ── System ──
  section "SYSTEM"

  local bw=20
  (( $(cols) < 50 )) && bw=12

  printf " CPU  "; bar "$CPU_PCT" "$bw"
  printf "  %3d%%  ${DIM}load %s  (%s cores)${RST}\n" "$CPU_PCT" "$CPU_LOAD" "$CPU_NCPU"

  printf " MEM  "; bar "$MEM_PCT" "$bw" 75 90
  local mem_g; mem_g=$(awk -v u="$MEM_USED" -v t="$MEM_TOTAL" 'BEGIN {printf "%.1fG / %.1fG", u/1024, t/1024}')
  printf "  %3d%%  ${DIM}%s${RST}\n" "$MEM_PCT" "$mem_g"

  printf " SWAP "; bar "$SWAP_PCT" "$bw" 10 50
  printf "  %3d%%  ${DIM}%sM / %sM${RST}\n" "$SWAP_PCT" "$SWAP_USED" "$SWAP_TOTAL"

  printf " PRES "; pressure_label "$MEM_PCT" "$SWAP_PCT"
  printf "\n"
  printf "\n"

  # ── Process trees ──
  section "PROCESSES"

  if [[ ${#claude_pids[@]} -eq 0 || ( ${#claude_pids[@]} -eq 1 && -z "${claude_pids[0]}" ) ]]; then
    printf "  ${DIM}no claude sessions detected${RST}\n"
  else
    for cpid in "${claude_pids[@]}"; do
      [[ -z "$cpid" ]] && continue
      render_tree "$cpid"
    done
  fi
  printf "\n"

  # ── Events ──
  section "EVENTS"

  if [[ -z "$EVENTS" ]]; then
    printf "  ${DIM}no recent events${RST}\n"
  else
    while IFS= read -r line; do
      printf "  ${DIM}%s${RST}\n" "$line"
    done <<< "$EVENTS"
  fi
  printf "\n"

  # ── Footer ──
  printf "${DIM} refresh %ss │ q: quit │ coolant v2${RST}\n" "$REFRESH"
}

# ─── Main loop ────────────────────────────────────────────────

main() {
  while true; do
    render
    # read with timeout doubles as our sleep — also catches 'q' to quit
    if read -rsn1 -t "$REFRESH" key 2>/dev/null; then
      [[ "$key" == "q" ]] && exit 0
    fi
  done
}

main
