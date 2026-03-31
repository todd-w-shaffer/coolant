# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## Project structure

Pure shell plugin. No build step, no package manager, no runtime dependencies.

```
.claude-plugin/plugin.json   # plugin manifest
hooks/hooks.json             # hook definitions (PostToolUse, SubagentStart/Stop)
scripts/common.sh            # shared config, paths, log function
scripts/monitor.sh           # live TUI dashboard (run in separate terminal)
scripts/toggle.sh            # manual parallel mode on/off/status
scripts/parallel-gate.sh     # PostToolUse hook: suppress tsc in parallel mode
scripts/agent-start.sh       # SubagentStart hook: increment counter
scripts/agent-stop.sh        # SubagentStop hook: decrement counter
skills/parallel/SKILL.md     # /coolant:parallel skill definition
```

## Key conventions

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_THRESHOLD`).
- All hook scripts log events via `coolant_log "message"` from common.sh.
- State lives in `/tmp/coolant-$USER.*` files — lockfile, counter, event log. No databases, no config files at runtime.
- Monitor uses braille characters (`⣿`, `⠂`) for progress bars, matching the user's existing status bar aesthetic.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao` for sensors. No third-party tools.

## TDD Workflow

Strict red-green-refactor. One feature per cycle.

1. **Red** — Write a failing test. Do NOT write any implementation yet. Use AAA (Arrange-Act-Assert), one assertion per test, behavior-describing names (`should_return_empty_when_no_items`).
2. **Green** — Implement the minimum code to pass. Nothing more.
3. **Refactor** — Improve code quality while keeping tests green. Do not skip this step.

## Testing

No test suite. To verify:

```bash
# Smoke test the monitor (renders one frame, then quit)
echo "q" | bash scripts/monitor.sh --refresh 1

# Test toggle
bash scripts/toggle.sh on
bash scripts/toggle.sh status
bash scripts/toggle.sh off

# Verify hooks parse
cat hooks/hooks.json | python3 -m json.tool
```

## Commit style

Subject line in imperative mood. Body includes a `Recipe:` block (a distilled prompt that would reproduce the change in one shot) and a `Changes:` block (per-file narrative). No Co-Authored-By lines.
