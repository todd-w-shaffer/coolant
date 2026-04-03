# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## ABSOLUTE RULES

**NEVER delete files or directories without explicit permission.** No exceptions. No "cleanup." No "safe to delete." ASK FIRST. Always. Even if the file looks stale, orphaned, or unnecessary. Even if you created it. Even if it's untracked. The user runs with permissive settings — that trust must not be abused for destructive operations.

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
thermal/                     # Go thermal dashboard binary (see below)
skills/parallel/SKILL.md     # /coolant:parallel skill definition
tests/test_helper.bash       # bats shared setup/teardown (temp dir isolation)
tests/*.bats                 # bats test files, one per script
```

### thermal/ (Go thermal dashboard)

Thermal dashboard rendered via bubbletea. Runs as a bottom tmux strip or standalone.

```
thermal/
├── cmd/thermal/main.go      # bubbletea app, flag parsing
├── internal/
│   ├── collector/
│   │   ├── types.go          # Snapshot, SystemStats, ProcessInfo, Category
│   │   ├── cpu_darwin.go     # cgo mach host_statistics for CPU tick deltas
│   │   ├── system.go         # MEM/SWAP/decompressions via sysctl/vm_stat
│   │   ├── procs.go          # Claude process discovery + descendant trees
│   │   ├── network.go        # API connectivity check
│   │   └── collector.go      # orchestrates collection, sends Snapshots
│   ├── model/
│   │   ├── state.go          # AppState: rolling history, smoothed counts
│   │   ├── threat.go         # ThreatLevel: COOL/WARM/HOT/MELTDOWN
│   │   ├── projection.go     # memory weight classes, headroom estimation
│   │   ├── personality.go    # idle messages, threat quips (loaded from CSV)
│   │   └── data/
│   │       └── messages.csv  # embedded status bar messages per threat level
│   ├── widgets/
│   │   ├── widget.go         # Widget interface
│   │   ├── sparkline.go      # braille severity bars (⡀ green/⡄ yellow/⡆ red)
│   │   ├── headline.go       # thermal bar: overall temp + 5 category boxes
│   │   ├── gauges.go         # CPU/MEM/compressor braille sparklines
│   │   ├── rates.go          # system stats + spawn/death/net + [h] help
│   │   └── alerts.go         # scrolling alert log
│   ├── layout/
│   │   └── horizontal.go     # bottom-strip layout compositor
│   ├── ui/
│   │   └── colors.go         # type colors, category colors, thresholds
│   └── demo/
│       └── demov2.go         # synthetic Snapshots with system stats
├── go.mod
└── go.sum
```

**Dependencies:** Go 1.26+, cgo (for mach CPU ticks), `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`.

**Build:** `cd thermal && go build -o ../bin/thermal ./cmd/thermal/`

**Run:**
- `./bin/thermal --demo` (thermal dashboard, synthetic data)
- `./bin/thermal` (thermal dashboard, live system data)

## Key conventions

### Bash (hooks, plumbing, collector)

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_THRESHOLD`).
- All hook scripts log events via `coolant_log "message"` from common.sh.
- State lives in `/tmp/coolant-$USER.*` files — lockfile, counter, event log. No databases, no config files at runtime.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao` for sensors. No third-party tools.

### Keyboard shortcuts

- `h` — toggle help overlay (replaces sparklines with plain-language explainers)
- `c` — collapse/expand the notification bar (top row with [i] install, etc.)
- `q` / `ctrl+c` — quit

### UI layers

- **Notification bar** (top, collapsible via `c`) — transient chrome: install CTA, future version alerts
- **Rates bar** (bottom, always visible) — system stats with colored dots + spawn/death/net + permanent `[h] help`
- **Status messages** — loaded from `thermal/internal/model/data/messages.csv` via `go:embed`, prefixed with `:: `

### Go (visualization)

- bubbletea Elm architecture: `Init` → `Update(msg)` → `View()`. No manual cursor management.
- Each widget is its own struct in `internal/widgets/` with `SetSize()`, `Update()`, and `View()` methods.
- Collector runs in a goroutine, sends `snapshotMsg` into bubbletea's program.
- Type colors defined once in `internal/ui/colors.go`, shared across all widgets.
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
