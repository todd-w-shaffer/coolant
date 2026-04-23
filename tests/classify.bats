#!/usr/bin/env bats
#
# Unit tests for scripts/classify.sh::classify_paths.
# The function's contract: given one or more paths as positional args,
# write one stanza per blocked path to stderr, return 0 if none blocked
# or 1 if any blocked. Never writes to stdout.

load test_helper

setup() {
  TEST_TMPDIR="$(mktemp -d)"
  setup_git_tmprepo
  # Source via the symlinked test-repo path so CLASSIFY_LIB_DIR resolves
  # to $TEST_TMPDIR/repo/scripts — i.e., classify.sh reads the test
  # repo's data files, not $PROJECT_ROOT's. Without this, mutations to
  # .githooks/{block,allow}list.txt inside a test would silently no-op.
  # shellcheck disable=SC1091
  source ./scripts/classify.sh
}

# ── rule: allowlist exact-path short-circuit ─────────────────────────────

@test "classify_paths allows a path on the allowlist" {
  run classify_paths docs/go-design.md
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "classify_paths does not block docs/*.md on the allowlist" {
  run classify_paths docs/event-schema.md
  [ "$status" -eq 0 ]
}

@test "classify_paths allowlist wins over blocklist prefix" {
  run classify_paths docs/backlog/README.md
  [ "$status" -eq 0 ]
}

# ── rule: allowlist path-prefix short-circuit ────────────────────────────

@test "classify_paths allowlist path-prefix exempts docs/archive/" {
  run classify_paths docs/archive/some-old-thing.md
  [ "$status" -eq 0 ]
}

@test "classify_paths allowlist path-prefix exempts docs/theming/" {
  run classify_paths docs/theming/00-color-audit.md
  [ "$status" -eq 0 ]
}

# ── rule: blocklist path-prefix ──────────────────────────────────────────

@test "classify_paths blocks a path under a blocklist prefix" {
  run classify_paths docs/backlog/foo.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"BLOCKED: docs/backlog/foo.md"* ]]
  [[ "$output" == *"blocklist-path:docs/backlog/"* ]]
}

# ── rule: blocklist keyword ──────────────────────────────────────────────

@test "classify_paths blocks a path matching a blocklist keyword" {
  run classify_paths notes/my-roadmap.txt
  [ "$status" -eq 1 ]
  [[ "$output" == *"BLOCKED: notes/my-roadmap.txt"* ]]
  [[ "$output" == *"blocklist-keyword:roadmap"* ]]
}

@test "classify_paths keyword matching is case-insensitive" {
  run classify_paths notes/ROADMAP.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"blocklist-keyword:roadmap"* ]]
}

# ── rule: docs-default-private meta-rule ─────────────────────────────────

@test "classify_paths blocks new docs/*.md via docs-default-private" {
  # Use a basename without blocklist keywords so the meta-rule fires,
  # not the keyword rule.
  run classify_paths docs/new-feature-guide.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"docs-default-private"* ]]
}

# ── multi-path and dedup ─────────────────────────────────────────────────

@test "classify_paths reports multiple blocked paths" {
  run classify_paths docs/backlog/foo.md docs/backlog/bar.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"docs/backlog/foo.md"* ]]
  [[ "$output" == *"docs/backlog/bar.md"* ]]
  [[ "$output" == *"2 path(s) blocked"* ]]
}

@test "classify_paths mixes blocked and allowed paths" {
  run classify_paths docs/go-design.md docs/backlog/foo.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"docs/backlog/foo.md"* ]]
  [[ "$output" != *"docs/go-design.md"* ]]
}

@test "classify_paths handles paths with spaces" {
  run classify_paths "docs/perplexity-investigations/my report.md"
  [ "$status" -eq 1 ]
  [[ "$output" == *"my report.md"* ]]
}

@test "classify_paths returns 0 for empty input" {
  run classify_paths
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "classify_paths dedupes repeated paths" {
  run classify_paths docs/backlog/foo.md docs/backlog/foo.md
  [ "$status" -eq 1 ]
  # exactly one BLOCKED stanza for the duplicate
  local count
  count=$(echo "$output" | grep -c "BLOCKED: docs/backlog/foo.md")
  [ "$count" -eq 1 ]
}

# ── allowlist sourced from HEAD, not index ───────────────────────────────

@test "classify_paths reads allowlist from HEAD, not working tree" {
  # Add 'docs/backlog/foo.md' to working-tree allowlist, stage it.
  echo "docs/backlog/foo.md" >> .githooks/allowlist.txt
  git add .githooks/allowlist.txt
  # HEAD still has the unmodified allowlist; classify_paths should
  # ignore the staged addition and still block.
  run classify_paths docs/backlog/foo.md
  [ "$status" -eq 1 ]
  [[ "$output" == *"BLOCKED: docs/backlog/foo.md"* ]]
}

@test "classify_paths falls back to working tree when HEAD has no allowlist" {
  # Create a new repo where HEAD has no .githooks/allowlist.txt.
  local alt="$TEST_TMPDIR/alt-repo"
  git init -q -b main "$alt"
  cd "$alt"
  git config user.email test@coolant.local
  git config user.name test
  git commit --allow-empty -q -m init
  mkdir -p .githooks
  cp "$PROJECT_ROOT/.githooks/blocklist.txt" .githooks/blocklist.txt
  cp "$PROJECT_ROOT/.githooks/allowlist.txt" .githooks/allowlist.txt

  run classify_paths docs/go-design.md
  [ "$status" -eq 0 ]
  [[ "$output" == *"HEAD has no .githooks/allowlist.txt"* ]]
}

# ── hook self-tamper informational notice ────────────────────────────────

@test "classify_paths warns and fails open carefully when blocklist is missing" {
  # Simulate a misload (e.g. scripts/classify.sh moved relative to .githooks/).
  rm -f .githooks/blocklist.txt
  run classify_paths docs/backlog/foo.md
  # docs-default-private still fires (path matches the hardcoded meta-rule),
  # so this specific path stays blocked — but the WARNING surfaces the
  # silent-fail-open risk for non-docs paths.
  [[ "$output" == *"WARNING: blocklist.txt not found"* ]]
  # And a non-docs blocked path slips through, confirming the warning is
  # load-bearing.
  rm -f .githooks/blocklist.txt
  run classify_paths notes/my-roadmap.txt
  [ "$status" -eq 0 ]
  [[ "$output" == *"WARNING: blocklist.txt not found"* ]]
}

@test "classify_paths emits self-tamper notice on hook file edits" {
  run classify_paths .githooks/pre-push docs/go-design.md
  [ "$status" -eq 0 ]
  [[ "$output" == *"NOTICE: this commit modifies hook enforcement code"* ]]
}

# ── allowlist.txt sort lint ──────────────────────────────────────────────

@test "classify.bats sort lint — allowlist.txt is canonically ordered" {
  # Canonical order: prefixes (sorted) first, blank line, then exact
  # paths (sorted). macOS awk has no asort, so pipe through sort(1).
  local src="$PROJECT_ROOT/.githooks/allowlist.txt"
  local prefixes exacts expected actual
  # Ignore comments and blank lines; verify prefixes appear before
  # exact-path entries, and each group is sorted (LC_ALL=C).
  prefixes=$(grep -E '^path:' "$src" | LC_ALL=C sort)
  exacts=$(grep -vE '^(#|path:|[[:space:]]*$)' "$src" | LC_ALL=C sort)
  expected=$(printf '%s\n%s' "$prefixes" "$exacts")
  actual=$(grep -vE '^[[:space:]]*(#|$)' "$src")
  [ "$expected" = "$actual" ]
}
