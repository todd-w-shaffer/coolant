#!/usr/bin/env bash
# Shared config and logging for coolant scripts

COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-/tmp/coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-/tmp/coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-/tmp/coolant-${USER}.log}"
COOLANT_THRESHOLD="${COOLANT_THRESHOLD:-3}"

coolant_log() {
  echo "$(date '+%H:%M:%S')  $1" >> "$COOLANT_LOG"
}

# Atomic counter operations using mkdir as a POSIX mutex.
# mkdir is atomic — only one process can create the directory.
_COOLANT_MUTEX="${COOLANT_COUNTER}.lock"

coolant_lock() {
  local tries=0
  while ! mkdir "$_COOLANT_MUTEX" 2>/dev/null; do
    tries=$((tries + 1))
    if [ "$tries" -gt 100 ]; then
      # Stale lock after ~1s — break it
      rmdir "$_COOLANT_MUTEX" 2>/dev/null
      return 1
    fi
    sleep 0.01
  done
}

coolant_unlock() {
  rmdir "$_COOLANT_MUTEX" 2>/dev/null
}
