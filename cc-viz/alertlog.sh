#!/usr/bin/env bash
# cc-viz alert log — scrolling threshold alert pane
# Appends timestamped lines when metrics cross thresholds.
# Sources common.sh, tails CC_VIZ_DATA for process snapshots.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

# ─── Additional config ───────────────────────────────────
SUMMARY_INTERVAL="${SUMMARY_INTERVAL:-30}"

# ─── State ───────────────────────────────────────────────
prev_line=""
net_warn_streak=0
in_alert=0  # 1 if any WARN/CRIT was active last tick
last_summary=0

# ─── Output helpers ──────────────────────────────────────

log_info() {
  local ts
  ts=$(date '+%H:%M:%S')
  printf "%s ${DIM}${FG_WHITE}INFO${RST}  %s\n" "$ts" "$1"
}

log_warn() {
  local ts
  ts=$(date '+%H:%M:%S')
  printf "%s ${BOLD}${FG_YELLOW}WARN${RST}  ${FG_YELLOW}%s${RST}\n" "$ts" "$1"
}

log_crit() {
  local ts
  ts=$(date '+%H:%M:%S')
  printf "%s ${BOLD}${FG_RED}CRIT${RST}  ${FG_RED}%s${RST}\n" "$ts" "$1"
}

# ─── Check type explosion via jq ─────────────────────────
# Given a JSONL line, output "TYPE:COUNT" for any type exceeding threshold
check_type_explosion() {
  local line="$1" threshold="$2"
  printf '%s' "$line" | jq -r --argjson thresh "$threshold" '
    [.procs[].type] | group_by(.) |
    map({type: .[0], count: length}) |
    map(select(.count > $thresh)) |
    .[] | "\(.type):\(.count)"
  ' 2>/dev/null
}

# ─── Main loop ───────────────────────────────────────────

main() {
  log_info "alert log started, tailing ${CC_VIZ_DATA}"
  log_info "thresholds: spawn=${SPAWN_WARN}/${SPAWN_CRIT} net=${NET_WARN}/${NET_CRIT} total=${TOTAL_WARN}/${TOTAL_CRIT} type=${TYPE_CRIT}"
  last_summary=$(date +%s)

  while IFS= read -r line; do
    [ -z "$line" ] && continue

    # Parse current snapshot
    eval "$(parse_jsonl_line "$line")"
    local total="${_count:-0}"

    # Count spawns (age == 0)
    local spawns=0
    local i=0
    for age in ${_proc_ages:-}; do
      if [ "$age" -eq 0 ] 2>/dev/null; then
        spawns=$((spawns + 1))
      fi
      i=$((i + 1))
    done

    # Compute deltas (deaths, net)
    if [ -n "$prev_line" ]; then
      eval "$(compute_deltas "$prev_line" "$line")"
    else
      _spawns="$spawns"
      _deaths=0
      _net="$spawns"
    fi
    local deaths="${_deaths:-0}"
    local net="${_net:-0}"

    # Track alert state for calm-restored detection
    local alert_this_tick=0

    # ── Check spawn rate ──
    if [ "$spawns" -gt "$SPAWN_CRIT" ] 2>/dev/null; then
      log_crit "spawn rate ${spawns}/s exceeds threshold (${SPAWN_CRIT}/s)"
      alert_this_tick=1
    elif [ "$spawns" -gt "$SPAWN_WARN" ] 2>/dev/null; then
      log_warn "spawn rate ${spawns}/s exceeds threshold (${SPAWN_WARN}/s)"
      alert_this_tick=1
    fi

    # ── Check net rate ──
    if [ "$net" -gt "$NET_CRIT" ] 2>/dev/null; then
      log_crit "net rate +${net}/s exceeds threshold (${NET_CRIT}/s)"
      net_warn_streak=0
      alert_this_tick=1
    elif [ "$net" -gt "$NET_WARN" ] 2>/dev/null; then
      net_warn_streak=$((net_warn_streak + 1))
      if [ "$net_warn_streak" -ge "$NET_SUSTAIN" ]; then
        log_warn "net rate +${net}/s sustained for ${net_warn_streak} ticks"
      fi
      alert_this_tick=1
    else
      net_warn_streak=0
    fi

    # ── Check total count ──
    if [ "$total" -gt "$TOTAL_CRIT" ] 2>/dev/null; then
      log_crit "total count ${total} exceeds threshold (${TOTAL_CRIT})"
      alert_this_tick=1
    elif [ "$total" -gt "$TOTAL_WARN" ] 2>/dev/null; then
      log_warn "total count ${total} exceeds threshold (${TOTAL_WARN})"
      alert_this_tick=1
    fi

    # ── Check type explosion ──
    local explosions
    explosions=$(check_type_explosion "$line" "$TYPE_CRIT")
    if [ -n "$explosions" ]; then
      while IFS= read -r entry; do
        local etype="${entry%%:*}"
        local ecount="${entry##*:}"
        log_crit "type ${etype} count ${ecount} exceeds threshold (${TYPE_CRIT})"
      done <<< "$explosions"
      alert_this_tick=1
    fi

    # ── Calm restored ──
    if [ "$in_alert" -eq 1 ] && [ "$alert_this_tick" -eq 0 ]; then
      log_info "calm restored, all metrics below WARN"
    fi
    in_alert=$alert_this_tick

    # ── Periodic summary ──
    local now_epoch
    now_epoch=$(date +%s)
    if [ $((now_epoch - last_summary)) -ge "$SUMMARY_INTERVAL" ]; then
      local net_sign=""
      if [ "$net" -ge 0 ] 2>/dev/null; then net_sign="+"; fi
      log_info "total:${total} spawns:${spawns}/s net:${net_sign}${net}/s"
      last_summary=$now_epoch
    fi

    prev_line="$line"
  done < <(tail -n 0 -f "$CC_VIZ_DATA" 2>/dev/null)
}

main "$@"
