# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## Project structure

Two layers: **bash** for hooks, plumbing, and data collection; **Go** for visualization.

```
.claude-plugin/plugin.json   # plugin manifest
hooks/hooks.json             # hook definitions (PostToolUse, SubagentStart/Stop)
scripts/common.sh            # shared config, paths, log function
scripts/monitor.sh           # live TUI dashboard (run in separate terminal)
scripts/toggle.sh            # manual parallel mode on/off/status
scripts/parallel-gate.sh     # PostToolUse hook: suppress tsc in parallel mode
scripts/agent-start.sh       # SubagentStart hook: increment counter
scripts/agent-stop.sh        # SubagentStop hook: decrement counter
cc-viz/collector.sh          # JSONL snapshot collector (bash, ps + jq)
cc-viz/cc-viz.sh             # tmux launcher (starts collector + Go binary)
cc-viz/common.sh             # shared colors, thresholds, JSONL parser (bash)
cc-viz-go/                   # Go visualization binary (see below)
skills/parallel/SKILL.md     # /coolant:parallel skill definition
tests/test_helper.bash       # bats shared setup/teardown (temp dir isolation)
tests/*.bats                 # bats test files, one per script
```

### cc-viz-go (Go visualization)

```
cc-viz-go/
├── cmd/cc-viz/main.go       # bubbletea app entry point
├── internal/
│   ├── jsonl/jsonl.go        # JSONL parser + file tailer
│   ├── demo/demo.go          # synthetic data generator (--demo mode)
│   ├── ui/
│   │   ├── colors.go         # type colors, thresholds
│   │   └── layout.go         # 2x2 grid layout renderer
│   └── panes/
│       ├── heatmap.go        # heatmap spectrogram (type × time)
│       ├── waveform.go       # braille spawn/death waveform
│       ├── waterfall.go      # per-process lifetime bars
│       ├── breakdown.go      # type breakdown bar chart
│       ├── phasering.go      # system state trajectory dots
│       └── alertlog.go       # scrolling event log
├── go.mod
└── go.sum
```

**Dependencies:** Go 1.26+, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`.

**Build:** `cd cc-viz-go && go build ./cmd/cc-viz/`

**Run:** `./cc-viz-go/cc-viz --demo` (synthetic data) or `./cc-viz-go/cc-viz --data /tmp/cc-procs.jsonl` (live)

## Key conventions

### Bash (hooks, plumbing, collector)

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_THRESHOLD`).
- All hook scripts log events via `coolant_log "message"` from common.sh.
- State lives in `/tmp/coolant-$USER.*` files — lockfile, counter, event log. No databases, no config files at runtime.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao` for sensors. No third-party tools.

### Go (visualization)

- bubbletea Elm architecture: `Init` → `Update(msg)` → `View()`. No manual cursor management.
- Each pane is its own struct in `internal/panes/` with `SetSize()`, `Update()`, and `View()` methods.
- JSONL tailer runs in a goroutine, sends `tickMsg` into bubbletea's program.
- Type colors defined once in `internal/ui/colors.go`, shared across all panes.
- Braille rendering done natively in Go (no awk, no subshells).

## TDD Workflow

Strict red-green-refactor. One feature per cycle.

1. **Red** — Write a failing `.bats` test in `tests/`. Do NOT write any implementation yet. One assertion per test, behavior-describing names (`agent-start auto-engages at threshold`).
2. **Green** — Implement the minimum code to pass. Nothing more.
3. **Refactor** — Improve code quality while keeping tests green. Do not skip this step.

## Testing

Uses [bats-core](https://github.com/bats-core/bats-core) (`brew install bats-core`). Tests are a dev dependency — they don't ship with coolant.

```bash
# Run full suite
bats tests/

# Run a single test file
bats tests/toggle.bats

# Run tests matching a name pattern
bats tests/ -f "auto-engage"
```

### Test conventions

- Each script gets a corresponding `tests/<name>.bats` file.
- `tests/test_helper.bash` provides `setup`/`teardown` — isolates all state to a temp directory so tests never touch real `/tmp/coolant-*` files.
- Tests set env vars (`COOLANT_LOCKFILE`, etc.) to point at the temp dir. Scripts respect these via the defaults in `common.sh`.
- New scripts must have tests before merge. New behavior on existing scripts must have a failing test first (red-green-refactor).

### Smoke tests (monitor only)

The TUI monitor can't be unit-tested with bats. Verify manually:

```bash
echo "q" | bash scripts/monitor.sh --refresh 1
```

## Commit style

Subject line in imperative mood. Body includes a `Recipe:` block (a distilled prompt that would reproduce the change in one shot) and a `Changes:` block (per-file narrative). No Co-Authored-By lines.
