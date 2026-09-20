#!/usr/bin/env bats

load test_helper

# The auto-release workflow stamps the version into several files. Nothing
# verified that the stamps landed, so .claude-plugin/plugin.json sat at 0.1.0
# while VERSION reached 0.32.0 — and because Claude Code caches plugins under
# cache/<owner>/coolant/<manifest-version>/, the updater compared 0.1.0 against
# 0.1.0 and never refreshed any install.

# ── Version agreement ──────────────────────────────────────

@test "plugin manifest version matches VERSION" {
  local want got
  want=$(cat "$PROJECT_ROOT/VERSION")
  got=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
        "$PROJECT_ROOT/.claude-plugin/plugin.json" | head -1)
  [ "$got" = "$want" ]
}

@test "statusline version stamp matches VERSION" {
  local want got
  want=$(cat "$PROJECT_ROOT/VERSION")
  got=$(sed -n 's/^# VERSION:[[:space:]]*//p' \
        "$PROJECT_ROOT/claude-statusline/statusline.sh" | head -1)
  [ "$got" = "$want" ]
}

# ── Stamp liveness ─────────────────────────────────────────

# A sed whose anchor matches nothing rewrites nothing and exits 0, so a stamp
# can rot into a no-op without the release ever failing. Assert every anchor
# in the workflow still finds its target.
@test "every auto-release sed anchor matches a line in its target file" {
  local wf="$PROJECT_ROOT/.github/workflows/auto-release.yml"
  local dead="" line pat file
  while IFS= read -r line; do
    pat=$(printf '%s\n' "$line" | sed -n 's|.*sed -i '"''"' "s/\(.*\)|\1|p' | sed 's|/.*||')
    file=$(printf '%s\n' "$line" | awk '{print $NF}')
    [ -n "$pat" ] || continue
    if ! grep -qE "$pat" "$PROJECT_ROOT/$file" 2>/dev/null; then
      dead="${dead}${file}: ${pat}"$'\n'
    fi
  done < <(grep "sed -i ''" "$wf")
  [ -z "$dead" ] || { echo "dead stamps:"; echo "$dead"; false; }
}
