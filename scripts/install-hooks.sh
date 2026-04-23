#!/bin/bash
set -euo pipefail
# One-time contributor bootstrap: configure git to use .githooks/ as the
# hooks directory and chmod the hook scripts executable. Idempotent.
#
# Refuses to silently disable an existing .git/hooks/ hook (graphify's
# post-commit knowledge-graph regenerator is the known case). Requires
# both --force AND a typed MIGRATE confirmation to proceed past that
# warning — two gates so a habitual --force doesn't lose work.

usage() {
  cat <<EOF
Usage: $0 [--force]

Sets core.hooksPath=.githooks for commit-classification enforcement.
--force  Proceed past the .git/hooks/ pre-existing-file warning after
         typing MIGRATE at the interactive prompt.
EOF
}

force=0
for arg in "$@"; do
  case "$arg" in
    --force) force=1 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; usage >&2; exit 2 ;;
  esac
done

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "not a git repository" >&2
  exit 1
}
cd "$repo_root"

current=$(git config --get core.hooksPath 2>/dev/null || echo "")
if [ "$current" = ".githooks" ]; then
  echo "coolant hooks: already configured (core.hooksPath=.githooks)"
  exit 0
fi
if [ -n "$current" ]; then
  echo "refusing to overwrite existing core.hooksPath=$current" >&2
  echo "reconcile manually: git config --unset core.hooksPath" >&2
  exit 1
fi

# Pre-flight: detect .git/hooks/ files that would stop running after
# install. Skip git's stock *.sample files.
existing=""
if [ -d .git/hooks ]; then
  for f in .git/hooks/*; do
    [ -e "$f" ] || continue
    case "$f" in *.sample) continue ;; esac
    existing="$existing ${f##.git/hooks/}"
  done
fi
existing="${existing# }"

if [ -n "$existing" ]; then
  cat >&2 <<EOF
WARNING: setting core.hooksPath=.githooks will stop running these hooks:
  $existing
(graphify's post-commit knowledge-graph regenerator lives at
 .git/hooks/post-commit by default.)

Options:
  (a) move each file into .githooks/<name> so it runs under the new hooksPath
  (b) re-run with --force AND type MIGRATE when prompted
  (c) abort and reconcile manually
EOF
  if [ "$force" -ne 1 ]; then
    exit 1
  fi
  printf 'Type MIGRATE to confirm: ' >&2
  read -r reply || reply=""
  if [ "$reply" != "MIGRATE" ]; then
    echo "aborted: confirmation string did not match" >&2
    exit 1
  fi
fi

git config core.hooksPath .githooks
[ -f .githooks/pre-push ]                 && chmod +x .githooks/pre-push
[ -f .claude/hooks/classify-staged.sh ]   && chmod +x .claude/hooks/classify-staged.sh

echo "installed coolant commit-classification hooks (core.hooksPath=.githooks)"
