#!/bin/bash
set -euo pipefail
# One-time contributor bootstrap: configure git to use .githooks/ as the
# hooks directory and chmod the hook scripts executable. Idempotent.
#
# Any existing hooks in .git/hooks/ (e.g., graphify's post-commit) are
# copied into .githooks/ so they keep running under the new hooksPath.
# Existing .githooks/ files are never overwritten.

usage() {
  cat <<EOF
Usage: $0 [-h|--help]

Sets core.hooksPath=.githooks for commit-classification enforcement.
Copies any existing .git/hooks/ scripts into .githooks/ so they keep
running under the new path.
EOF
}

for arg in "$@"; do
  case "$arg" in
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
# .git/hooks (absolute or relative) is the default — safe to replace.
# Any other custom value could be intentional; refuse and let the user reconcile.
git_hooks_abs="$(cd "$repo_root" && cd .git/hooks 2>/dev/null && pwd)"
if [ -n "$current" ] && [ "$current" != ".git/hooks" ] && [ "$current" != "$git_hooks_abs" ]; then
  echo "refusing to overwrite existing core.hooksPath=$current" >&2
  echo "reconcile manually: git config --unset core.hooksPath" >&2
  exit 1
fi

# Copy existing .git/hooks/ scripts into .githooks/ so they keep
# running under the new hooksPath. Skip git's stock *.sample files.
# Never overwrite files already in .githooks/.
if [ -d .git/hooks ]; then
  for f in .git/hooks/*; do
    [ -e "$f" ] || continue
    case "$f" in *.sample) continue ;; esac
    name="${f##.git/hooks/}"
    if [ ! -e ".githooks/$name" ]; then
      cp "$f" ".githooks/$name"
      chmod +x ".githooks/$name"
      echo "  copied .git/hooks/$name → .githooks/$name"
    fi
  done
fi

git config core.hooksPath .githooks
[ -f .githooks/pre-push ]                 && chmod +x .githooks/pre-push
[ -f .claude/hooks/classify-staged.sh ]   && chmod +x .claude/hooks/classify-staged.sh

echo "installed coolant commit-classification hooks (core.hooksPath=.githooks)"
