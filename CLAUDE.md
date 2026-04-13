# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## ABSOLUTE RULES

**NEVER delete files or directories without explicit permission.** No exceptions. No "cleanup." No "safe to delete." ASK FIRST. Always. Even if the file looks stale, orphaned, or unnecessary. Even if you created it. Even if it's untracked. The user runs with permissive settings — that trust must not be abused for destructive operations.

## Quick reference

```bash
cd thermal && go test ./...              # Go tests
bats tests/                              # bash tests
cd thermal && go build -o ../bin/thermo ./cmd/thermal/  # build
./bin/thermo --demo                      # run with synthetic data
./bin/thermo                             # run with live data
```

## Workflow rules

### Commit style

Subject line in imperative mood. Body includes a `Recipe:` block (a distilled prompt that would reproduce the change in one shot) and a `Changes:` block (per-file narrative). No Co-Authored-By lines.

### TDD (all functional code)

Strict red-green-refactor for **any** code that can break — bash, Go, or otherwise. No exceptions. One feature per cycle.

1. **Red** — Write a failing test first. Do NOT write any implementation yet.
2. **Green** — Implement the minimum code to pass. Nothing more.
3. **Refactor** — Improve code quality while keeping tests green. Do not skip this step.

Bug fixes follow the same cycle: write a test that reproduces the bug (red), fix it (green), clean up (refactor).

#### Bash tests

`.bats` files in `tests/`. One assertion per test, behavior-describing names (`agent-start auto-engages at threshold`). Uses [bats-core](https://github.com/bats-core/bats-core).

#### Go tests

Table-driven tests in `*_test.go` files next to the code they test. Use `t.Helper()` on test helpers, direct assertions with `t.Errorf`/`t.Fatalf` — no test framework. Run: `cd thermal && go test ./...`

## Project structure

Two layers: **bash** for hooks, plumbing, and data collection; **Go** for visualization.

```
.claude-plugin/plugin.json   # plugin manifest
.github/workflows/           # notify-marketplace.yml (triggers submodule update on push)
claude-statusline/           # braille statusline for Claude Code (context/session/weekly bars)
install.sh                   # interactive installer (binary + statusline + settings.json patching)
hooks/hooks.json             # hook definitions (SessionStart, PreToolUse, SubagentStart/Stop)
scripts/common.sh            # shared config, paths, log + JSONL event functions + _reconcile_counter
scripts/toggle.sh            # manual parallel mode on/off/status (reconciled)
scripts/preflight.sh         # SessionStart hook: warn about missing worktree exclusions
scripts/gate.sh              # PreToolUse hook: cap test runners (reconciled), suppress build tools
scripts/agent-start.sh       # SubagentStart hook: increment counter, warn at threshold
scripts/agent-stop.sh        # SubagentStop hook: decrement counter, auto-disengage at zero
thermal/                     # Go thermal dashboard binary (see below)
docs/theming/                # theme system planning: color audit, schema, palettes, migration mapping
skills/coolant/SKILL.md      # /coolant skill (opt-in build suppression)
tests/test_helper.bash       # bats shared setup/teardown (temp dir isolation)
tests/*.bats                 # bats test files, one per script
assets/                      # VHS tape files, demo GIFs, marketing screenshots
dev/otel/                    # Local Prometheus+Grafana stack for OTEL dogfooding (see below)
```

### Archive folders (gitignored — do not read, reference, or treat as current)

Historical artifacts kept for nostalgia. May be stale or broken.

- `docs/archive/` — superseded specs and explorations
- `scripts/archive/` — old bash TUI (monitor.sh, sparkline.sh, agents.sh), replaced by Go thermal
- `tests/archive/` — tests for archived scripts
- `thermal/cmd/archive/` — one-off Go experiments (breathe, comets, sparkdebug)
- `bin/archive/` — compiled binaries from archived experiments

### dev/otel/ (Local OTEL dogfood stack)

Local Prometheus + Grafana prototype for validating the Thermal Cloud
data model and dashboard queries before touching hosted infra. Claude
Code pushes OTLP metrics directly to Prometheus (no collector needed —
Prometheus 3.x native OTLP receiver). Grafana auto-provisions the
datasource and five dashboards via file-based config.

- `start.sh` — launches both processes with prefixed logs, Ctrl-C kills both
- `env.sh` — source before launching Claude Code; `cclaude` alias in ~/.zshrc does this automatically
- `dashboards/` — claude-spend, claude-insights, claude-models, claude-cfo, claude-vpeng
- `data/` — gitignored Prometheus TSDB and Grafana state

Verified Claude Code metric names (live-checked, not guessed):
`claude_code_cost_usage_USD_total`, `claude_code_token_usage_tokens_total`,
`claude_code_lines_of_code_count_total`, `claude_code_active_time_seconds_total`,
`claude_code_session_count_total`, `claude_code_code_edit_tool_decision_total`.

### thermal/ (Go thermal dashboard)

Thermal dashboard rendered via bubbletea. Runs as a bottom tmux strip or standalone.

```
thermal/
├── cmd/thermal/
│   ├── main.go              # bubbletea app, flag parsing
│   ├── parent_darwin.go     # kqueue EVFILT_PROC watcher — exits when parent dies
│   └── parent_other.go      # no-op stub for non-darwin platforms
├── cmd/brailletext/main.go  # standalone braille font debug tool
├── cmd/swatch/main.go       # theme palette preview tool (static or --animate bubbletea loop)
├── internal/
│   ├── collector/
│   │   ├── types.go          # Snapshot, SystemStats, ProcessInfo, Category
│   │   ├── cpu_darwin.go     # cgo mach host_statistics for CPU tick deltas (cached host port)
│   │   ├── system.go         # MEM/SWAP/decompressions via sysctl/vm_stat
│   │   ├── procs.go          # Claude process discovery + descendant trees
│   │   ├── network.go        # API connectivity check (TCP to api.anthropic.com)
│   │   ├── collector.go      # decoupled fast (150ms) + slow (1s network/swap/GPU) loops
│   │   └── events.go         # JSONL event tailer (polls $TMPDIR/coolant-$USER.events.jsonl)
│   ├── model/
│   │   ├── state.go          # AppState: rolling history, smoothed counts
│   │   ├── threat.go         # ThreatLevel: COOL/WARM/HOT/MELTDOWN
│   │   ├── projection.go     # memory weight classes, headroom estimation
│   │   ├── personality.go    # idle messages, threat quips (loaded from CSV)
│   │   ├── ring.go           # generic RingBuffer[T] — O(1) push, used by history/alerts/rates
│   │   ├── temperature.go    # OverallTemperature formula + severity→brightness curves for the LCD readout
│   │   └── data/
│   │       └── messages.csv  # embedded status bar messages per threat level
│   ├── anim/
│   │   ├── profile.go        # Profile struct (all animation tunables)
│   │   ├── default.go        # Default profile (reads from config/tuning.go)
│   │   ├── calm.go           # Calm profile (slower rates, wider brightness, softer)
│   │   ├── intense.go        # Intense profile (faster rates, tighter gaussian, snappier)
│   │   └── registry.go       # animation registry: Get(), Names(), --animation flag lookup
│   ├── theme/
│   │   ├── theme.go          # Theme struct, SeverityColor, SparkThresholds, Init (LUT pre-compute)
│   │   ├── classic.go        # Classic palette (backward-compat traffic-light)
│   │   ├── iron.go           # Iron palette (FLIR blackbody: purple→magenta→amber)
│   │   ├── mono.go           # Mono palette (single amber hue, brightness-only)
│   │   ├── frappe.go         # Frappe palette (native catppuccin frappe hex colors)
│   │   └── registry.go       # theme registry: Get(), Names(), --theme flag lookup
│   ├── widgets/
│   │   ├── sparkline.go      # double-res braille sparklines (2 samples/char), themed severity color
│   │   ├── headline.go       # 2-row thermal bar: quip + runtimes over right-anchored sessions/agents/build-shell/LCD cluster
│   │   ├── segmentreadout.go # LCD-style temperature readout (spring-smoothed value, per-digit styled spans, meltdown pulse)
│   │   ├── segmentfont.go    # 7-segment bitmap font for segmentreadout digits and degree glyph
│   │   ├── gauges.go         # CPU/MEM/compressor gauges + spring animations
│   │   ├── rates.go          # system stats (CPU/MEM/SWAP/GPU) + spawn/death/net + [h] help
│   │   ├── alerts.go         # scrolling alert log
│   │   ├── breathedots.go    # agent indicators: tidal wave (active), KITT scanner (stale or highscore), 3-state glyphs (⬡⏣⬢)
│   │   ├── heatbloom.go      # thermographic accent behind headline left zone: HCL-blended bloom driven by composite heat
│   │   ├── rail.go           # directional heat rails (dotted underlines above/below build/shell counts) with ember decay
│   │   ├── braillefont.go    # 4×8 bitmap font for gauge labels (CPU/MEM/SWAP)
│   │   ├── thermal.go        # category heat-level threshold logic (returns gradient index)
│   │   ├── golden_test.go    # golden capture/match tests for render regression detection
│   │   └── testdata/*.golden # frozen render output for Classic theme backward-compat
│   ├── layout/
│   │   └── horizontal.go     # bottom-strip layout compositor
│   ├── config/
│   │   └── tuning.go         # named constants: timing, thresholds, EMA, animation defaults
│   ├── ui/
│   │   └── colors.go         # type colors, category colors, agent glyphs, DimText/ColorText helpers
│   └── demo/
│       └── demov2.go         # synthetic Snapshots with system stats
├── go.mod
└── go.sum
```

**Dependencies:** Go 1.25+, cgo (for mach CPU ticks), `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/harmonica`, `github.com/lucasb-eyer/go-colorful`.

**Note:** The source directory is `thermal/` but the binary is named `thermo` (to avoid colliding with macOS `/usr/bin/thermal`).

**Build:** `cd thermal && go build -o ../bin/thermo ./cmd/thermal/`

**Run:**
- `./bin/thermo --demo` (thermal dashboard, synthetic data)
- `./bin/thermo` (thermal dashboard, live system data)

## Distribution

Plugin and dashboard ship separately. The plugin installs via Claude Code's marketplace system; the dashboard is a prebuilt binary on GitHub Releases.

- **Marketplace:** `todd-w-shaffer/marketplace` repo hosts the plugin manifest. Coolant is a git submodule under `plugins/coolant`. A GitHub Action (`.github/workflows/notify-marketplace.yml`) fires `repository_dispatch` on every push to main, triggering the marketplace repo to auto-update the submodule.
- **Binaries:** `thermo-darwin-arm64` and `thermo-darwin-amd64` attached to GitHub Releases. Build both with: `GOARCH=arm64 go build -o bin/thermo-darwin-arm64 ./cmd/thermal/ && GOARCH=amd64 go build -o bin/thermo-darwin-amd64 ./cmd/thermal/` (from `thermal/`).
- **Install script:** `install.sh` downloads the binary for the user's arch, asks where to put it, and optionally installs the braille statusline to `~/.claude/` with settings.json patching.

## Conventions

### JSONL event bus (the bash↔Go seam)

Bash hooks write to `$TMPDIR/coolant-$USER.events.jsonl`. Go's event tailer (`collector/events.go`) polls this file at 500ms. This is the *only* runtime data path between the two layers — there are zero direct calls. The JSONL schema is defined by `coolant_event` in `common.sh` and parsed by `TailEvents` in `events.go`.

### Bash (hooks, plumbing)

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_EVENTS`, `COOLANT_THRESHOLD`).
- Hook scripts log human-readable events via `coolant_log "message"` and structured JSONL via `coolant_event '"key":"value"'`.
- JSON field extraction from hook stdin uses `_json_field` (top-level) and `_nested_command` (tool_input.command) — no jq dependency.
- Values interpolated into JSONL must pass through `_json_escape` to handle backslashes and quotes.
- State lives in `$TMPDIR/coolant-$USER.*` files — lockfile, counter, event log. No databases, no config files at runtime. `$TMPDIR` is per-user on macOS (`/var/folders/.../T/`), avoiding `/tmp` symlink attacks.
- **Gate system**: `gate.sh` is a PreToolUse hook on Bash with two behaviors. **Test runners** (vitest, jest, cargo test, go test, pytest) are always capped with agent-count-adaptive concurrency limits: `cap = floor((cores - 2) / agents)`, min 1. **Build tools, linters, and type checkers** (tsc, eslint, cargo build, go vet, etc.) are suppressed when user opts in via `/coolant`. Agent count is reconciled against JSONL event log (`_reconcile_counter` in common.sh) to prevent stale counters from orphaned agents. Handles wrappers (`npx`, `env`, `command`, path prefixes). See `docs/gate-system-report.md`.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao`, `ioreg` for sensors. No third-party tools.

### Go (API gotchas)

- **bubbletea v2** Elm architecture: `Init` → `Update(msg)` → `View() tea.View`. View returns a struct with `Content`, `AltScreen`, `MouseMode` fields. Uses `tea.KeyPressMsg` (not v1's `tea.KeyMsg`). Mode 2026 synchronized output is automatic.
- **lipgloss v2** (`charm.land/lipgloss/v2`): `lipgloss.Color()` returns `color.Color` (stdlib), not a type. Map types use `color.Color` with `image/color` import.
- Each widget is its own struct in `internal/widgets/` with `SetSize()`, `Update()`, and `View() string` methods (only top-level model returns `tea.View`).
- **Collector** runs three loops: fast (150ms) for CPU/procs via cgo, slow (1s) for network + swap/vm_stat/GPU concurrently via subprocesses, event tailer (500ms) for JSONL. GPU utilization via `ioreg -r -d 1 -c AGXAccelerator` piped through grep. Shared online state protected by mutex.
- **Theme system** (`internal/theme/`): All colors flow through a `*theme.Theme` struct passed to every widget constructor. `Theme.Init()` pre-computes HCL blend LUTs (101 entries each for severity gradients). `Theme.SeverityColor()` replaces the old package-level `severityColor()`. Built-in themes registered in `theme.Registry`; resolved via `--theme` flag > `COOLANT_THEME` env > `"classic"` default. To add a theme: copy `classic.go`, change colors, register in `registry.go`.
- **Animation system** (`internal/anim/`): All motion parameters flow through an `*anim.Profile` struct passed alongside `*theme.Theme` to widget constructors. Theme controls color, profile controls motion — orthogonal axes. `Default()` reads from `config/tuning.go` (single source of truth). Built-in profiles: `default`, `calm` (slower/softer), `intense` (faster/sharper). Resolved via `--animation` flag > `COOLANT_ANIMATION` env > `"default"`. To add a profile: copy `default.go`, change values, register in `registry.go`.
- **Agent animations**: Active agents use a tidal wave (slow sine swell, `⬡→⏣→⬢` three-state glyph sweep). Stale/ghost agents use a KITT scanner (gaussian brightness peak bouncing left-to-right). `--kitt-highscore` flag (or `COOLANT_KITT_HIGHSCORE=1`) repurposes the scanner to display completed agent count instead — stale dots dim-breathe, completed dots earn the KITT scan. Animation patterns are universal across themes and profiles; theme controls accent color, profile controls speed/brightness/spring physics.
- `GaugeDotColor.ANSIOverride` lets Classic use 16-color ANSI escapes while other themes use truecolor. If set, `Init()` uses the override; otherwise derives truecolor from `Color`.
- Process type and category colors live in `internal/ui/colors.go` as semantic defaults. Agent glyphs (`⬡⏣⬢`) and `DimText`/`ColorText` helpers also in `ui/colors.go`. Timing, thresholds, and EMA alphas live in `internal/config/tuning.go`; animation defaults also live there but are consumed via `anim.Default()` rather than directly.
- Braille rendering done natively in Go (no awk, no subshells).
- For sparkline, gauge, and render architecture internals see `docs/go-design.md`.

## Testing

Uses [bats-core](https://github.com/bats-core/bats-core) (`brew install bats-core`). Tests are a dev dependency — they don't ship with coolant.

```bash
bats tests/                        # full suite
bats tests/toggle.bats             # single file
bats tests/ -f "reconcile"         # name pattern
```

- Each script gets a corresponding `tests/<name>.bats` file.
- `tests/test_helper.bash` provides `setup`/`teardown` — isolates all state to a temp directory so tests never touch real `/tmp/coolant-*` files.
- Tests set env vars (`COOLANT_LOCKFILE`, etc.) to point at the temp dir. Scripts respect these via the defaults in `common.sh`.
- New scripts must have tests before merge. New behavior on existing scripts must have a failing test first (red-green-refactor).

## graphify

Knowledge graph at `graphify-out/`. Post-commit hook keeps it current automatically.
Read `graphify-out/GRAPH_REPORT.md` for god nodes and community structure before deep codebase questions.
