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
source "${SCRIPT_DIR}/sparkline.sh"
source "${SCRIPT_DIR}/agents.sh"

# ─── Config ───────────────────────────────────────────────
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

# ─── History arrays ───────────────────────────────────────
CPU_HISTORY=()
MEM_HISTORY=()

# ─── Agent color palette (8 ANSI basic colors) ───────────
AGENT_COLOR_0=$'\033[32m'     # green
AGENT_COLOR_1=$'\033[36m'     # cyan
AGENT_COLOR_2=$'\033[33m'     # yellow
AGENT_COLOR_3=$'\033[35m'     # magenta
AGENT_COLOR_4=$'\033[34m'     # blue
AGENT_COLOR_5=$'\033[37m'     # white
AGENT_COLOR_6=$'\033[31m'     # red
AGENT_COLOR_7=$'\033[1;32m'   # bold green

# ─── Colors ───────────────────────────────────────────────
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

# ─── Terminal setup ───────────────────────────────────────
cleanup() {
  tput cnorm 2>/dev/null
  printf "%s\n" "$RST"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
tput civis 2>/dev/null

# ─── Rendering helpers ───────────────────────────────────

COLS=60
refresh_cols() { COLS=$(tput cols 2>/dev/null || echo 60); }
cols() { echo "$COLS"; }

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

  printf "%s" "$c"
  for (( i=0; i<filled; i++ )); do printf '⣿'; done
  printf "%s" "$DIM"
  for (( i=0; i<empty; i++ )); do printf '⠂'; done
  printf "%s" "$RST"
}

# ─── Pressure badge ──────────────────────────────────────

pressure_badge() {
  local agents=${1:-0} cpu_pct=${2:-0} mem_pct=${3:-0}
  if (( agents >= 6 || (agents >= 4 && cpu_pct >= 80) )); then
    printf "${BGRED}${WHT}${BLD} MELTDOWN ${RST}"
  elif (( agents >= 4 || cpu_pct >= 70 )); then
    printf "${BGRED}${WHT}${BLD} HOT ${RST}"
  elif (( agents >= 2 || cpu_pct >= 50 )); then
    printf "${BGYLW}${WHT}${BLD} WARM ${RST}"
  else
    printf "${BGGRN}${WHT} COOL ${RST}"
  fi
}

# ─── Sensors ──────────────────────────────────────────────

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
  else
    COOL_MODE="OFF"
  fi
}

# ─── Process tree ─────────────────────────────────────────

ALL_PROCS=""

collect_procs() {
  ALL_PROCS=$(ps -Ao pid=,ppid=,%cpu=,rss=,args= 2>/dev/null)
}

# Render an entire process tree in a SINGLE awk call.
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
    if (children[ppid] == "") children[ppid] = pid
    else children[ppid] = children[ppid] " " pid
  }
  END {
    if (!(root in cpu)) { print "  " DIM "pid " root " not found" RST; exit }

    sp = 1
    s_pid[sp] = root; s_prefix[sp] = ""; s_last[sp] = -1; s_depth[sp] = 0
    count = 0; tcpu = 0; tmem = 0

    while (sp > 0) {
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

      if (islst == -1) {
        printf "  %s%d%s  %5s%%  %5dM  %s%s%s\n", BLD, p, RST, cpu[p], rss[p], WHT, command[p], RST
        child_pfx = "  "
      } else if (islst == 1) {
        printf "  %s%s└─%s%-6d %5s%%  %5dM  %s\n", DIM, pfx, RST, p, cpu[p], rss[p], command[p]
        child_pfx = pfx "  "
      } else {
        printf "  %s%s├─%s%-6d %5s%%  %5dM  %s\n", DIM, pfx, RST, p, cpu[p], rss[p], command[p]
        child_pfx = pfx "│ "
      }

      n = split(children[p], kids, " ")
      rc = 0
      for (j = 1; j <= n; j++) {
        if (kids[j] != "" && kids[j] != p) { rc++; rkids[rc] = kids[j] }
      }

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

# ─── Event log ────────────────────────────────────────────

read_events() {
  if [[ -f "$COOLANT_LOG" ]]; then
    EVENTS=$(tail -n "$MAX_EVENTS" "$COOLANT_LOG" 2>/dev/null || echo "")
  else
    EVENTS=""
  fi
}

# ─── Agent legend ─────────────────────────────────────────

render_agent_legend() {
  local i slot_pid job color
  local legend_items=()

  for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
    eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
    if [[ -n "$slot_pid" ]]; then
      eval "job=\"\$AGENT_JOB_${i}\""
      eval "color=\"\$AGENT_COLOR_${i}\""
      legend_items+=("${color}●${RST} ${DIM}${job}${RST}")
    fi
  done

  if [[ ${#legend_items[@]} -gt 0 ]]; then
    local line=""
    for item in "${legend_items[@]}"; do
      if [[ -n "$line" ]]; then
        line="${line}  "
      fi
      line="${line}${item}"
    done
    printf " %s\n" "$line"
  fi
}

# ─── Render ───────────────────────────────────────────────

render() {
  read_coolant_status
  read_events
  collect_procs

  local w; w=$(cols)
  local chart_width=$(( w - 2 ))
  (( chart_width < 10 )) && chart_width=10

  # Run agent scan with collected process data (sets ACTIVE_AGENT_COUNT)
  scan_agents "$ALL_PROCS"
  local active_agents=$ACTIVE_AGENT_COUNT

  # ── Header + Agent Gauge ──
  local now
  now=$(date '+%H:%M:%S')
  printf "\n"
  if [[ "$COOL_MODE" == "PARALLEL" ]]; then
    printf " ${RED}${BLD}COOLANT${RST}  ${BGYLW}${WHT}${BLD} PARALLEL ${RST}"
  else
    printf " ${CYN}${BLD}COOLANT${RST}  ${DIM}idle${RST}"
  fi
  printf "%*s\n" $(( w - 22 )) "$now"

  # Agent gauge bar
  local gauge_width=10
  printf " "
  agent_gauge "$active_agents" "$MAX_AGENT_SLOTS" "$gauge_width"
  printf "  ${BLD}%s${RST} agents" "$active_agents"

  # Right-align pressure badge
  local label_width=$(( gauge_width + 12 ))  # gauge + "  N agents"
  local badge_pad=$(( w - label_width - 13 ))  # 13 = " MELTDOWN " + margin
  (( badge_pad < 1 )) && badge_pad=1
  printf "%*s" "$badge_pad" ""
  pressure_badge "$active_agents" "$CPU_PCT" "$MEM_PCT"
  printf "\n\n"

  # ── Agent Chart ──
  if (( active_agents > 0 )); then
    box_top "AGENTS" "${active_agents} active" "" "$w"

    # Build multi-trace color string and value args
    local trace_colors=""
    local trace_values=()
    local trace_count=0

    for (( i = 0; i < MAX_AGENT_SLOTS; i++ )); do
      local slot_pid
      eval "slot_pid=\"\$AGENT_SLOT_PID_${i}\""
      if [[ -n "$slot_pid" ]]; then
        local color
        eval "color=\"\$AGENT_COLOR_${i}\""
        if [[ -n "$trace_colors" ]]; then
          trace_colors="${trace_colors}|"
        fi
        trace_colors="${trace_colors}${color}"

        # Pass only actual history values; renderer handles right-alignment
        local hist_len
        eval "hist_len=\${#AGENT_HIST_${i}[@]}"
        local j
        for (( j = 0; j < hist_len; j++ )); do
          local val
          eval "val=\${AGENT_HIST_${i}[\$j]}"
          trace_values+=("$val")
        done
        (( trace_count++ ))
      fi
    done

    if (( trace_count > 0 )); then
      local agent_chart
      agent_chart=$(multitrace_chart 5 "$chart_width" "$trace_count" "$trace_colors" "${trace_values[@]}")
      while IFS= read -r _line; do
        box_line "$_line" "$w"
      done <<< "$agent_chart"
    fi
    box_bottom "$w"

    # Agent legend
    render_agent_legend
    printf "\n"
  else
    # Dark cockpit: minimal agent section
    box_top "AGENTS" "idle" "" "$w"
    local empty_chart
    empty_chart=$(multitrace_chart 2 "$chart_width" 1 "${DIM}" 0 0 0 0)
    while IFS= read -r _line; do
      box_line "$_line" "$w"
    done <<< "$empty_chart"
    box_bottom "$w"
    printf "\n"
  fi

  # ── CPU (demoted to 2 rows) ──
  local mem_g; mem_g=$(awk -v u="$MEM_USED" -v t="$MEM_TOTAL" 'BEGIN {printf "%.1fG / %.1fG", u/1024, t/1024}')
  box_top "CPU" "load ${CPU_LOAD} (${CPU_NCPU} cores)" "${CPU_PCT}%" "$w"
  local cpu_chart
  cpu_chart=$(sparkline_chart 2 "$chart_width" "${CPU_HISTORY[@]}")
  while IFS= read -r _line; do
    box_line "$_line" "$w"
  done <<< "$cpu_chart"
  box_bottom "$w"

  # ── MEM + SWAP + pressure inline ──
  printf " MEM "; bar "$MEM_PCT" 10 50 90
  printf "  %3d%%  ${DIM}%s${RST}" "$MEM_PCT" "$mem_g"
  printf "     SWAP "; bar "$SWAP_PCT" 5 10 50
  printf "  %3d%%  ${DIM}%sM / %sM${RST}" "$SWAP_PCT" "$SWAP_USED" "$SWAP_TOTAL"
  printf "\n\n"

  # ── Process trees ──
  section "PROCESSES"

  local claude_pids=()
  if [[ -n "$TARGET_PID" ]]; then
    claude_pids=("$TARGET_PID")
  else
    while IFS= read -r _cpid; do
      [[ -n "$_cpid" ]] && claude_pids+=("$_cpid")
    done < <(find_claude_pids "$ALL_PROCS")
  fi

  if [[ ${#claude_pids[@]} -eq 0 || ( ${#claude_pids[@]} -eq 1 && -z "${claude_pids[0]:-}" ) ]]; then
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
  printf "${DIM} refresh %ss │ q: quit │ coolant v3${RST}\n" "$REFRESH"
}

# ─── Main loop ────────────────────────────────────────────

main() {
  local prev_cols=0
  clear
  while true; do
    refresh_cols
    if (( COLS != prev_cols )); then
      clear
      prev_cols=$COLS
    fi
    # Collect sensor data in main shell so history arrays persist
    read_cpu
    read_mem
    read_swap
    local chart_width=$(( $(cols) - 2 ))
    (( chart_width < 10 )) && chart_width=10
    history_push CPU_HISTORY "$CPU_PCT" $(( chart_width * 2 ))
    history_push MEM_HISTORY "$MEM_PCT" $(( chart_width * 2 ))
    # Render into buffer, then paint in one shot
    local frame
    frame=$(render)
    tput home
    printf '%s' "$frame"
    tput ed
    # read with timeout doubles as our sleep
    if read -rsn1 -t "$REFRESH" key 2>/dev/null; then
      [[ "$key" == "q" ]] && exit 0
    fi
  done
}

main
