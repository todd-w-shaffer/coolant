#!/usr/bin/env bash
# Shared config and logging for coolant scripts

COOLANT_LOCKFILE="${COOLANT_LOCKFILE:-/tmp/coolant-${USER}.lock}"
COOLANT_COUNTER="${COOLANT_COUNTER:-/tmp/coolant-agents-${USER}.count}"
COOLANT_LOG="${COOLANT_LOG:-/tmp/coolant-${USER}.log}"
COOLANT_THRESHOLD="${COOLANT_THRESHOLD:-3}"

coolant_log() {
  echo "$(date '+%H:%M:%S')  $1" >> "$COOLANT_LOG"
}
