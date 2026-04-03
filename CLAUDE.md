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
│   │   ├── widget.go         # Widget interface
│   │   ├── sparkline.go      # double-res braille sparklines (2 samples/char)
│   │   ├── headline.go       # thermal bar: overall temp + 5 category boxes
│   │   ├── gauges.go         # CPU/MEM/compressor gauges + spring animations
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

**Dependencies:** Go 1.26+, cgo (for mach CPU ticks), `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/harmonica`, `github.com/lucasb-eyer/go-colorful`.

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

- **bubbletea v2** Elm architecture: `Init` → `Update(msg)` → `View() tea.View`. View returns a struct with `Content`, `AltScreen`, `MouseMode` fields. Uses `tea.KeyPressMsg` (not v1's `tea.KeyMsg`). Mode 2026 synchronized output is automatic.
- **lipgloss v2** (`charm.land/lipgloss/v2`): `lipgloss.Color()` returns `color.Color` (stdlib), not a type. Map types use `color.Color` with `image/color` import.
- Each widget is its own struct in `internal/widgets/` with `SetSize()`, `Update()`, and `View() string` methods (only top-level model returns `tea.View`).
- **Collector** runs two decoupled loops: fast (150ms) for CPU/MEM/procs driving sparklines, slow (1s) for network reachability. Shared online state protected by mutex.
- **Sparklines** use double-resolution braille: both columns of each character pack two time samples (left=N, right=N+1), doubling visible history. Raw data is linearly interpolated (midpoint insertion) before rendering to turn step-function transitions into visible ramps. Shared helpers `prepareSparkData()`/`prepareSparkMask()` handle zero-padding, interpolation, and visible window slicing. Height is proportional (auto-scaled to visible window peak), color is severity gradient via go-colorful `BlendHcl()` (green→yellow→red, 24-bit truecolor). Values below 2% of peak render as invisible (noise floor). **Edge fades** dim the outermost 3 characters on both left and right edges (35%→60%→82% brightness ramp) so data fades in on entry and fades out on exit. Uses `dimmedFg()` which blends severity color toward black via Lab space.
- **Gauges** use harmonica spring physics (critically damped, 30fps) for smooth numeric readout easing. **Sparklines scroll at animation rate (30fps)**, not collector rate: each `AnimTick` pushes the spring-interpolated value into a per-gauge `renderHistory` buffer, and sparklines render from this buffer. This decouples scroll speed from data collection. Peak per gauge tracks the visible render-history window only with fast decay (0.982/tick at 30fps, ~1.3s half-life) — spikes that scroll off screen release the scale within ~1s.
- **Render architecture**: collector samples at 150ms, animation tick runs at 30fps (~33ms). Springs interpolate between data arrivals (~4.5 frames per sample). `snapshotMsg` updates spring targets; `animTickMsg` advances springs AND pushes interpolated values into render history, driving sparkline scroll. The two rates are fully decoupled via separate bubbletea messages.
- **CPU sampling** caches the mach host port (avoids port leak) and holds the last computed CPU% when tick deltas are zero (avoids false 0% gaps at 150ms).
- Type colors defined once in `internal/ui/colors.go`, shared across all widgets. `ui.ThresholdColor(val, warn, crit float64)` is the single threshold color function.
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
