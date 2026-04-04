# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## ABSOLUTE RULES

**NEVER delete files or directories without explicit permission.** No exceptions. No "cleanup." No "safe to delete." ASK FIRST. Always. Even if the file looks stale, orphaned, or unnecessary. Even if you created it. Even if it's untracked. The user runs with permissive settings — that trust must not be abused for destructive operations.

## Workflow rules

### Commit style

Subject line in imperative mood. Body includes a `Recipe:` block (a distilled prompt that would reproduce the change in one shot) and a `Changes:` block (per-file narrative). No Co-Authored-By lines.

### TDD (bash scripts only)

Strict red-green-refactor. One feature per cycle.

1. **Red** — Write a failing `.bats` test in `tests/`. Do NOT write any implementation yet. One assertion per test, behavior-describing names (`agent-start auto-engages at threshold`).
2. **Green** — Implement the minimum code to pass. Nothing more.
3. **Refactor** — Improve code quality while keeping tests green. Do not skip this step.

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
├── cmd/breathe/main.go      # braille dot fade test (color-as-opacity proof of concept)
├── internal/
│   ├── collector/
│   │   ├── types.go          # Snapshot, SystemStats, ProcessInfo, Category
│   │   ├── cpu_darwin.go     # cgo mach host_statistics for CPU tick deltas (cached host port)
│   │   ├── system.go         # MEM/SWAP/decompressions via sysctl/vm_stat
│   │   ├── procs.go          # Claude process discovery + descendant trees
│   │   ├── network.go        # API connectivity check (TCP to api.anthropic.com)
│   │   └── collector.go      # decoupled fast (150ms) + slow (1s network) loops
│   ├── model/
│   │   ├── state.go          # AppState: rolling history, smoothed counts
│   │   ├── threat.go         # ThreatLevel: COOL/WARM/HOT/MELTDOWN
│   │   ├── projection.go     # memory weight classes, headroom estimation
│   │   ├── personality.go    # idle messages, threat quips (loaded from CSV)
│   │   └── data/
│   │       └── messages.csv  # embedded status bar messages per threat level
│   ├── widgets/
│   │   ├── sparkline.go      # double-res braille sparklines (2 samples/char)
│   │   ├── headline.go       # thermal bar: overall temp + 5 category boxes
│   │   ├── gauges.go         # CPU/MEM/compressor gauges + spring animations
│   │   ├── rates.go          # system stats (CPU/MEM/SWAP/GPU) + spawn/death/net + [h] help
│   │   └── alerts.go         # scrolling alert log
│   ├── layout/
│   │   └── horizontal.go     # bottom-strip layout compositor
│   ├── config/
│   │   └── tuning.go         # named constants: timing, thresholds, EMA, animation
│   ├── ui/
│   │   └── colors.go         # type colors, category colors, ThreatColor, GaugeDots
│   └── demo/
│       └── demov2.go         # synthetic Snapshots with system stats
├── go.mod
└── go.sum
```

**Dependencies:** Go 1.26+, cgo (for mach CPU ticks), `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/harmonica`, `github.com/lucasb-eyer/go-colorful`.

**Build:** `cd thermal && go build -o ../bin/thermal ./cmd/thermal/`

**Run:**
- `./bin/thermal --demo` (thermal dashboard, synthetic data)
- `./bin/thermal` (thermal dashboard, live system data)

## Conventions

### Bash (hooks, plumbing)

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_THRESHOLD`).
- All hook scripts log events via `coolant_log "message"` from common.sh.
- State lives in `/tmp/coolant-$USER.*` files — lockfile, counter, event log. No databases, no config files at runtime.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao`, `ioreg` for sensors. No third-party tools.

### Go (API gotchas)

- **bubbletea v2** Elm architecture: `Init` → `Update(msg)` → `View() tea.View`. View returns a struct with `Content`, `AltScreen`, `MouseMode` fields. Uses `tea.KeyPressMsg` (not v1's `tea.KeyMsg`). Mode 2026 synchronized output is automatic.
- **lipgloss v2** (`charm.land/lipgloss/v2`): `lipgloss.Color()` returns `color.Color` (stdlib), not a type. Map types use `color.Color` with `image/color` import.
- Each widget is its own struct in `internal/widgets/` with `SetSize()`, `Update()`, and `View() string` methods (only top-level model returns `tea.View`).
- **Collector** runs two decoupled loops: fast (150ms) for CPU/MEM/GPU/procs, slow (1s) for network. GPU utilization via `ioreg -r -d 1 -c AGXAccelerator` piped through grep. Shared online state protected by mutex.
- Type colors, `ThreatColor`, and `GaugeDots` defined once in `internal/ui/colors.go`, shared across all widgets. Severity gradient coloring uses `severityColor()` in `sparkline.go` (green→yellow→red via go-colorful HCL blending). All magic numbers (timing, thresholds, EMA alphas, animation params) live in `internal/config/tuning.go` as named constants.
- Braille rendering done natively in Go (no awk, no subshells).
- For sparkline, gauge, and render architecture internals see `docs/go-design.md`.

## Testing

Uses [bats-core](https://github.com/bats-core/bats-core) (`brew install bats-core`). Tests are a dev dependency — they don't ship with coolant.

```bash
bats tests/                        # full suite
bats tests/toggle.bats             # single file
bats tests/ -f "auto-engage"       # name pattern
```

- Each script gets a corresponding `tests/<name>.bats` file.
- `tests/test_helper.bash` provides `setup`/`teardown` — isolates all state to a temp directory so tests never touch real `/tmp/coolant-*` files.
- Tests set env vars (`COOLANT_LOCKFILE`, etc.) to point at the temp dir. Scripts respect these via the defaults in `common.sh`.
- New scripts must have tests before merge. New behavior on existing scripts must have a failing test first (red-green-refactor).
- Smoke test for TUI monitor: `echo "q" | bash scripts/monitor.sh --refresh 1`
