#!/usr/bin/env bash
# cc-viz — Claude Code subprocess visualizer launcher
# Creates a tmux 2x2 grid: heatmap, waveform, waterfall, breakdown + phase ring in status bar.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Defaults ────────────────────────────────────────────
CC_VIZ_SESSION="cc-viz"
CC_VIZ_REFRESH="${CC_VIZ_REFRESH:-1}"
TARGET_PID=""
COLLECTOR_ONLY=0
DEMO_MODE=0

# Alert thresholds (passed to children via env)
export SPAWN_WARN SPAWN_CRIT NET_WARN NET_CRIT NET_SUSTAIN
export TOTAL_WARN TOTAL_CRIT TYPE_CRIT
export CC_VIZ_DATA CC_VIZ_REFRESH

# ─── Parse args ──────────────────────────────────────────
while [ $# -gt 0 ]; do
  case "$1" in
    --pid)
      TARGET_PID="$2"
      shift 2
      ;;
    --collector-only)
      COLLECTOR_ONLY=1
      shift
      ;;
    --demo)
      DEMO_MODE=1
      shift
      ;;
    --refresh)
      CC_VIZ_REFRESH="$2"
      export CC_VIZ_REFRESH
      shift 2
      ;;
    --help|-h)
      printf "Usage: cc-viz.sh [OPTIONS]\n\n"
      printf "Options:\n"
      printf "  --pid <PID>        Monitor specific Claude Code PID\n"
      printf "  --collector-only   Run collector only, no tmux session\n"
      printf "  --demo             Generate synthetic data\n"
      printf "  --refresh <SEC>    Refresh interval (default: 1)\n"
      printf "  --help             Show this help\n"
      exit 0
      ;;
    *)
      printf "%sUnknown option: %s%s\n" "$FG_RED" "$1" "$RST" >&2
      exit 1
      ;;
  esac
done

# ─── Auto-detect Claude Code PID ─────────────────────────
detect_claude_pid() {
  local pid
  pid=$(ps -Ao pid=,args= 2>/dev/null \
    | awk '/claude.*--permission-mode/ && !/awk/ {print $1; exit}')
  echo "${pid:-}"
}

if [ -z "$TARGET_PID" ] && [ "$DEMO_MODE" -eq 0 ]; then
  TARGET_PID=$(detect_claude_pid)
  if [ -z "$TARGET_PID" ]; then
    printf "%sNo Claude Code process found. Use --pid <PID> or --demo%s\n" "$FG_RED" "$RST" >&2
    exit 1
  fi
  printf "%sAuto-detected Claude Code PID: %s%s\n" "$DIM" "$TARGET_PID" "$RST"
fi

export TARGET_PID

# ─── Collector ───────────────────────────────────────────
COLLECTOR_PID=""

start_collector() {
  local collector="${SCRIPT_DIR}/collector.sh"
  if [ ! -x "$collector" ]; then
    printf "%sCollector not found: %s%s\n" "$FG_RED" "$collector" "$RST" >&2
    exit 1
  fi

  local collector_args=""
  if [ "$DEMO_MODE" -eq 1 ]; then
    collector_args="--demo"
  elif [ -n "$TARGET_PID" ]; then
    collector_args="--pid ${TARGET_PID}"
  fi

  bash "$collector" $collector_args &
  COLLECTOR_PID=$!
}

# ─── Cleanup ─────────────────────────────────────────────
# Only runs when the user explicitly kills the launcher (Ctrl-C),
# NOT when tmux attach returns (detach is normal).
ATTACHED=0

cleanup() {
  # If we never attached or are being signaled, tear everything down
  if [ "$ATTACHED" -eq 0 ]; then
    if [ -n "$COLLECTOR_PID" ]; then
      kill "$COLLECTOR_PID" 2>/dev/null || true
      wait "$COLLECTOR_PID" 2>/dev/null || true
    fi
    rm -f "$CC_VIZ_DATA"
    tmux kill-session -t "$CC_VIZ_SESSION" 2>/dev/null || true
  fi
}

stop_all() {
  ATTACHED=0
  cleanup
  exit 0
}

trap stop_all INT TERM

# ─── Collector-only mode ─────────────────────────────────
if [ "$COLLECTOR_ONLY" -eq 1 ]; then
  start_collector
  printf "%sCollector running (PID %s), writing to %s%s\n" "$DIM" "$COLLECTOR_PID" "$CC_VIZ_DATA" "$RST"
  printf "%sPress Ctrl-C to stop%s\n" "$DIM" "$RST"
  wait "$COLLECTOR_PID"
  exit 0
fi

# ─── Verify tmux ─────────────────────────────────────────
if ! command -v tmux >/dev/null 2>&1; then
  printf "%stmux is required. Install with: brew install tmux%s\n" "$FG_RED" "$RST" >&2
  exit 1
fi

# ─── Env prefix for pane commands ────────────────────────
# Single-line env var prefix prepended to each pane's script invocation
PANE_ENV="CC_VIZ_DATA='${CC_VIZ_DATA}' CC_VIZ_REFRESH='${CC_VIZ_REFRESH}' SPAWN_WARN='${SPAWN_WARN}' SPAWN_CRIT='${SPAWN_CRIT}' NET_WARN='${NET_WARN}' NET_CRIT='${NET_CRIT}' NET_SUSTAIN='${NET_SUSTAIN}' TOTAL_WARN='${TOTAL_WARN}' TOTAL_CRIT='${TOTAL_CRIT}' TYPE_CRIT='${TYPE_CRIT}'"

# ─── Start collector ─────────────────────────────────────
start_collector
printf "%sCollector started (PID %s)%s\n" "$DIM" "$COLLECTOR_PID" "$RST"

# Give collector a moment to create the data file
sleep 1

# ─── Create tmux session with 2x2 grid + phase ring ─────
#
#  Layout:
#   ┌──────────────┬──────────────┐
#   │   heatmap    │   waveform   │
#   ├──────────────┼──────────────┤
#   │  waterfall   │  breakdown   │
#   └──────────────┴──────────────┘
#   phase ring in tmux status-right
#
tmux kill-session -t "$CC_VIZ_SESSION" 2>/dev/null || true

TERM_COLS=$(tput cols 2>/dev/null || echo 120)
TERM_LINES=$(tput lines 2>/dev/null || echo 40)

tmux new-session -d -s "$CC_VIZ_SESSION" -x "$TERM_COLS" -y "$TERM_LINES"
tmux split-window -h -t "${CC_VIZ_SESSION}:0.0"
tmux split-window -v -t "${CC_VIZ_SESSION}:0.0"
tmux split-window -v -t "${CC_VIZ_SESSION}:0.1"

# ─── Send commands to each pane ──────────────────────────
tmux send-keys -t "${CC_VIZ_SESSION}:0.0" "${PANE_ENV} bash '${SCRIPT_DIR}/heatmap.sh'" Enter
tmux send-keys -t "${CC_VIZ_SESSION}:0.1" "${PANE_ENV} bash '${SCRIPT_DIR}/waveform.sh'" Enter
tmux send-keys -t "${CC_VIZ_SESSION}:0.2" "${PANE_ENV} bash '${SCRIPT_DIR}/waterfall.sh'" Enter
tmux send-keys -t "${CC_VIZ_SESSION}:0.3" "${PANE_ENV} bash '${SCRIPT_DIR}/breakdown.sh'" Enter

# ─── Phase ring in tmux status bar ───────────────────────
tmux set -t "$CC_VIZ_SESSION" status on
tmux set -t "$CC_VIZ_SESSION" status-right-length 80
tmux set -t "$CC_VIZ_SESSION" status-right "#(${PANE_ENV} bash '${SCRIPT_DIR}/phase-ring.sh' --inline)"
tmux set -t "$CC_VIZ_SESSION" status-interval 1

# ─── Attach ──────────────────────────────────────────────
printf "%s%scc-viz%s %sattaching to tmux session...%s\n" "$FG_CYAN" "$BOLD" "$RST" "$DIM" "$RST"
printf "%sDetach: Ctrl-b d | Stop: tmux kill-session -t cc-viz%s\n" "$DIM" "$RST"
ATTACHED=1
tmux attach-session -t "$CC_VIZ_SESSION"

# After detach, leave everything running
printf "\n%scc-viz still running in background.%s\n" "$DIM" "$RST"
printf "%sReattach: tmux attach -t cc-viz%s\n" "$DIM" "$RST"
printf "%sStop:     tmux kill-session -t cc-viz && kill %s%s\n" "$DIM" "$COLLECTOR_PID" "$RST"
