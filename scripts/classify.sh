#!/bin/bash
# Shared classification library — sourced by .githooks/pre-push and
# .claude/hooks/classify-staged.sh. Exports one public function:
#
#   classify_paths <path1> [path2 ...]
#
# For each blocked path, prints one stanza to stderr in the Terminal
# format defined in docs/backlog/commit-classification-hooks.spec.md.
# Returns 0 if no paths blocked, 1 if any blocked. Never writes stdout.
#
# Data files:
#   .githooks/blocklist.txt  — read from working tree
#   .githooks/allowlist.txt  — read from HEAD: (committed version) so a
#                              malicious or careless commit can't authorize
#                              its own leak by also editing the allowlist.
#                              Bootstrap fallback to working tree when HEAD
#                              has no allowlist.

# BASH_SOURCE[0] (not $0) so the path is correct when this file is sourced.
CLASSIFY_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

_classify_load_allowlist() {
  _CLASSIFY_ALLOWLIST=""
  _CLASSIFY_ALLOWLIST_BOOTSTRAP=""
  if _CLASSIFY_ALLOWLIST=$(git show HEAD:.githooks/allowlist.txt 2>/dev/null); then
    return 0
  fi
  local wt="$CLASSIFY_LIB_DIR/../.githooks/allowlist.txt"
  if [ -f "$wt" ]; then
    _CLASSIFY_ALLOWLIST=$(cat "$wt")
    _CLASSIFY_ALLOWLIST_BOOTSTRAP=1
  fi
}

_classify_load_blocklist() {
  _CLASSIFY_BLOCKLIST=""
  _CLASSIFY_BLOCKLIST_LOWER=""
  _CLASSIFY_BLOCKLIST_MISSING=""
  local bl="$CLASSIFY_LIB_DIR/../.githooks/blocklist.txt"
  if [ -f "$bl" ]; then
    _CLASSIFY_BLOCKLIST=$(cat "$bl")
    # Pre-lowercase once for case-insensitive keyword matching; avoids
    # forking tr(1) per (path × keyword) pair inside _classify_one.
    _CLASSIFY_BLOCKLIST_LOWER=$(printf '%s' "$_CLASSIFY_BLOCKLIST" | tr '[:upper:]' '[:lower:]')
  else
    # Fail-open is the most-feared regression here (silent ALLOW for
    # everything). Leave the buffer empty but flag the load so
    # classify_paths can warn loudly.
    _CLASSIFY_BLOCKLIST_MISSING=1
  fi
}

# Classify one path against the loaded lists.
# Sets _verdict to "ALLOW" or "BLOCK", _rule_id, _rule_arg, _human on
# block. Using out-vars instead of stdout echo avoids a subshell per call.
_classify_one() {
  local p="$1" line spec
  _verdict="ALLOW"; _rule_id=""; _rule_arg=""; _human=""

  while IFS= read -r line; do
    case "$line" in ''|'#'*) continue ;; esac
    if [[ "$line" == path:* ]]; then
      spec="${line#path:}"
      case "$p" in "$spec"*) return ;; esac
    elif [ "$p" = "$line" ]; then
      return
    fi
  done <<< "$_CLASSIFY_ALLOWLIST"

  while IFS= read -r line; do
    case "$line" in ''|'#'*) continue ;; esac
    if [[ "$line" == path:* ]]; then
      spec="${line#path:}"
      case "$p" in
        "$spec"*)
          _verdict="BLOCK"
          _rule_id="blocklist-path"
          _rule_arg="$spec"
          _human="path is in a private-only prefix"
          return ;;
      esac
    fi
  done <<< "$_CLASSIFY_BLOCKLIST"

  # Keyword match is case-insensitive against the pre-lowered blocklist;
  # keywords in blocklist.txt are authored lowercase by convention.
  local p_lower
  p_lower=$(printf '%s' "$p" | tr '[:upper:]' '[:lower:]')
  while IFS= read -r line; do
    case "$line" in ''|'#'*) continue ;; esac
    if [[ "$line" == keyword:* ]]; then
      spec="${line#keyword:}"
      case "$p_lower" in
        *"$spec"*)
          _verdict="BLOCK"
          _rule_id="blocklist-keyword"
          _rule_arg="$spec"
          _human="path contains strategy keyword \"$spec\""
          return ;;
      esac
    fi
  done <<< "$_CLASSIFY_BLOCKLIST_LOWER"

  case "$p" in
    docs/*.md|docs/*/*.md|docs/*/*/*.md|docs/*/*/*/*.md)
      _verdict="BLOCK"
      _rule_id="docs-default-private"
      _rule_arg=""
      _human="new docs/*.md files default private; allowlist if public-safe"
      ;;
  esac
}

_classify_is_hook_path() {
  case "$1" in
    .githooks/*|.claude/hooks/classify-staged.sh|scripts/classify.sh) return 0 ;;
  esac
  return 1
}

classify_paths() {
  if [ "$#" -eq 0 ]; then
    return 0
  fi

  _classify_load_allowlist
  _classify_load_blocklist

  if [ -n "$_CLASSIFY_BLOCKLIST_MISSING" ]; then
    printf '[coolant-classify] WARNING: blocklist.txt not found at %s — only docs-default-private will fire. Did scripts/classify.sh move?\n' \
      "$CLASSIFY_LIB_DIR/../.githooks/blocklist.txt" >&2
  fi
  if [ -n "$_CLASSIFY_ALLOWLIST_BOOTSTRAP" ]; then
    printf '[coolant-classify] HEAD has no .githooks/allowlist.txt; using working tree (expected only for the install bootstrap commit).\n' >&2
  fi

  local blocked=0
  local seen=" "
  local notice_printed=0
  local p rule_token

  for p in "$@"; do
    case "$seen" in *" $p "*) continue ;; esac
    seen="$seen$p "

    if [ "$notice_printed" -eq 0 ] && _classify_is_hook_path "$p"; then
      printf '[coolant-classify] NOTICE: this commit modifies hook enforcement code (%s). Review with care.\n' "$p" >&2
      notice_printed=1
    fi

    _classify_one "$p"
    [ "$_verdict" = "ALLOW" ] && continue

    rule_token="$_rule_id"
    [ -n "$_rule_arg" ] && rule_token="${_rule_id}:${_rule_arg}"

    {
      printf '[coolant-classify] BLOCKED: %s\n' "$p"
      printf '  rule: %s\n' "$rule_token"
      printf '  reason: %s\n' "$_human"
      printf '  fix:  move to ../thermal-enterprise/%s (if you have the private repo)\n' "$p"
      printf '        or add to .githooks/allowlist.txt in a prior commit if this is legitimately public\n'
      printf '        (for renames: add the new path to allowlist, not the old)\n'
    } >&2

    blocked=$((blocked + 1))
  done

  if [ "$blocked" -gt 0 ]; then
    {
      printf '[coolant-classify] %d path(s) blocked.\n' "$blocked"
      printf 'Classification rules: see coolant/.githooks/README.md (short version)\n'
      printf 'or thermal-enterprise/docs/repo-split.md (full guide, private repo).\n'
      printf 'Bypasses (all deliberate, none automatic):\n'
      printf '  git push --no-verify\n'
      printf '  git -c core.hooksPath=/dev/null push    # also skips hooks, documented\n'
    } >&2
    return 1
  fi

  return 0
}
