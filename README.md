# coolant

**A resource management layer for Claude Code.**

Claude Code has no awareness of the physical machine it's running on. When it spawns parallel agents, each one assumes it owns the system — launching independent `tsc`, `vitest`, and bundler processes that compete for CPU and RAM simultaneously. On a 16GB MacBook Pro with 6-15 concurrent agents, this cascade can consume 85GB of swap and freeze the machine for minutes.

Coolant sits between Claude Code and your hardware. It senses system state, tracks agent processes, and throttles work to keep your machine responsive.

## What it does

### Thermal management (v1)

- **Hook suppression** — Per-edit `tsc --noEmit` hooks are the biggest multiplier. One agent editing 20 files triggers 20 compilations. Multiply by 8 agents and that's 160 `tsc` invocations during the writing phase alone. Coolant suppresses these hooks when parallel mode is active; type-checking happens once at the end.

- **Agent counting** — `SubagentStart`/`SubagentStop` hooks track how many agents are alive. When the count crosses a configurable threshold (default: 3), parallel mode auto-engages. When agents finish, it disengages. No manual intervention.

- **Staggered validation** — The `/coolant:parallel` skill reminds the orchestrating agent to run `check` and `build` sequentially after all agents complete, not concurrently across agents.

### System monitoring (v2)

- **Live dashboard** — A terminal TUI (`monitor.sh`) that runs alongside your Claude session. Designed for a tmux side pane, built with braille-character rendering.

- **Process-tree awareness** — Not just "how many node processes exist" but "which specific processes descended from this Claude session and how much CPU/memory are they using." Uses `ps` process ancestry to trace the exact subtree.

- **System sensors** — CPU load, memory usage, swap pressure — collected from macOS system calls (`sysctl`, `vm_stat`), no dependencies.

- **Event logging** — Every mode change, agent start/stop, and hook suppression is logged with timestamps, visible in the monitor's event panel.

### Subprocess visualizer (cc-viz)

A fullscreen terminal dashboard that monitors the process tree spawned by Claude Code in real time. Built in Go with [bubbletea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss) — a single binary renders all six panes with no tmux required. A bash collector (`cc-viz/collector.sh`) writes per-second JSONL snapshots; the Go binary tails the file and renders.

- **Heatmap spectrogram** — Rows are process types, columns are time ticks. Cell color intensity encodes count (black → gray → orange → red). Reveals which type is hot *when* — synchronized bands mean correlated activity, staggered bands mean cascading spawns.

- **Braille waveform** — Overlaid spawn and death traces at braille resolution (4 dots per character row). Two waveforms on one canvas so crossovers, divergence, and convergence are visible as shapes — the moment deaths overtake spawns is a crossing of two traces, not a number to compute mentally.

- **Process waterfall** — Each row is one alive process, bar length proportional to age, colored by type with age-based intensity (bright for new spawns, dim for old workers). The only view that tracks individual processes from birth to death. Many short bars = churn. Few long bars = stable workers.

- **Phase ring** — Classifies each tick into CALM/RAMPING/EXPLODING/COOLING based on multi-signal analysis, then displays the trajectory as a rolling sequence of colored dots. The 30,000-foot view — one glance tells you the story of the last minute.

- **Type breakdown** — Horizontal bar chart of currently alive processes grouped by type. Bars proportional to count, colored by type, with threshold-colored totals.

- **Alert log** — Scrolling event log of threshold crossings, burst detections, and mode changes with timestamps.

## Prerequisites

- **macOS or Linux** (bash 3.2+)
- **Go 1.26+** — for building the cc-viz visualizer (`brew install go`)
- **jq** — used by the JSONL collector (`brew install jq`)
- **tmux** — optional, if you want to run cc-viz alongside your Claude session in a split (`brew install tmux`)
- **bats-core** — optional, for running tests (`brew install bats-core`)

## Installation

```bash
# 1. Install the Claude Code plugin
claude plugin install ./coolant --scope user

# Or symlink for development
claude --plugin-dir /path/to/coolant

# 2. Build the cc-viz visualizer
cd cc-viz-go
go build ./cmd/cc-viz/
```

## Usage

### Automatic mode

Coolant works automatically once installed. Hooks count active agents and engage parallel mode at the threshold.

### Manual override

```
/coolant:parallel on      # force parallel mode on
/coolant:parallel off     # force parallel mode off
/coolant:parallel status  # check current state
```

### cc-viz (subprocess visualizer)

```bash
# Demo mode — synthetic data, no live Claude session needed
./cc-viz-go/cc-viz --demo

# Live mode — auto-detects Claude Code process
bash cc-viz/cc-viz.sh

# Live mode — specific PID
bash cc-viz/cc-viz.sh --pid 12345
```

The launcher (`cc-viz.sh`) starts the bash collector and the Go binary together. Or run them separately:

```bash
# Terminal 1: start collector
bash cc-viz/collector.sh --pid 12345

# Terminal 2: start visualizer
./cc-viz-go/cc-viz --data /tmp/cc-procs.jsonl
```

Press `q` to quit.

### Monitor (system-level)

Open a second terminal tab or tmux pane:

```bash
bash scripts/monitor.sh
```

Options:

```bash
bash scripts/monitor.sh --pid 12345    # monitor specific Claude PID
bash scripts/monitor.sh --refresh 5    # custom refresh interval (default: 2s)
COOLANT_REFRESH=5 bash scripts/monitor.sh  # same via env var
```

Press `q` to quit.

### What happens in parallel mode

- Per-edit `tsc --noEmit` hooks are suppressed
- A system message reminds agents to defer validation
- When all agents complete, run your build gate:

```bash
npm run check    # typecheck + tests, once
npm run build    # bundle, once
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `COOLANT_THRESHOLD` | `3` | Agent count that triggers parallel mode |
| `COOLANT_REFRESH` | `2` | Monitor refresh interval (seconds) |
| `COOLANT_LOCKFILE` | `/tmp/coolant-$USER.lock` | Parallel mode lockfile path |
| `COOLANT_COUNTER` | `/tmp/coolant-agents-$USER.count` | Agent counter path |
| `COOLANT_LOG` | `/tmp/coolant-$USER.log` | Event log path |

## How it works

```
Normal mode (1-2 agents):
  Edit file -> tsc --noEmit -> immediate feedback -> next edit

Parallel mode (3+ agents):
  Edit file -> [tsc suppressed] -> next edit -> ... -> all agents done
  -> npm run check (once) -> npm run build (once)
```

The trade-off is intentional: you lose per-edit type feedback during parallel work, but you keep your machine alive. Type errors surface in the single validation pass at the end — the same as CI would catch them.

## Architecture

Two layers: **bash** for hooks, plumbing, and data collection; **Go** for visualization.

```
coolant/
├── .claude-plugin/
│   └── plugin.json          # plugin manifest
├── hooks/
│   └── hooks.json           # hook definitions (PostToolUse, SubagentStart/Stop)
├── scripts/                 # bash — hooks, plumbing, system monitor
│   ├── common.sh            # shared config, paths, log function
│   ├── agents.sh            # agent tracker: slot management, job detection
│   ├── sparkline.sh         # braille chart renderers (monitor only)
│   ├── monitor.sh           # live TUI dashboard (system-level)
│   ├── toggle.sh            # manual parallel mode on/off/status
│   ├── parallel-gate.sh     # PostToolUse hook: suppress tsc in parallel mode
│   ├── agent-start.sh       # SubagentStart hook: increment counter
│   └── agent-stop.sh        # SubagentStop hook: decrement counter
├── cc-viz/                  # bash — collector + launcher
│   ├── cc-viz.sh            # launcher (starts collector + Go binary)
│   ├── collector.sh         # JSONL snapshot collector (ps + jq → /tmp/cc-procs.jsonl)
│   └── common.sh            # shared colors, thresholds, JSONL parser
├── cc-viz-go/               # Go — visualization binary
│   ├── cmd/cc-viz/main.go   # bubbletea app entry point
│   ├── internal/
│   │   ├── jsonl/            # JSONL parser + file tailer
│   │   ├── demo/             # synthetic data generator (--demo)
│   │   ├── ui/               # colors, thresholds, grid layout
│   │   └── panes/            # heatmap, waveform, waterfall, breakdown,
│   │                         # phase ring, alert log
│   ├── go.mod
│   └── go.sum
├── skills/
│   └── parallel/
│       └── SKILL.md          # /coolant:parallel skill definition
└── tests/
    ├── test_helper.bash      # bats shared setup/teardown
    ├── agents.bats           # agent tracker tests
    ├── sparkline.bats        # renderer tests
    └── *.bats                # per-script test files
```

The bash collector writes JSONL snapshots to `/tmp/cc-procs.jsonl`. The Go binary tails that file and renders all six panes in a single fullscreen terminal UI. No tmux required for the visualizer itself.

## Testing

```bash
# Bash tests (hooks, plumbing)
bats tests/
bats tests/agents.bats

# Go tests (visualization)
cd cc-viz-go && go test ./...
```

## Roadmap

- **Graduated throttling** — Progressive response as load increases: warn, reduce agent cap, suppress hooks, reap processes
- **Process-level actuators** — `renice` build processes, `SIGSTOP`/`SIGCONT` for pause/resume, targeted kill for runaway processes
- **Worktree lifecycle** — Detect and clean orphaned git worktrees from crashed agents
- **Status bar integration** — Compact thermal indicator as a braille cell in the Claude Code status line
- **Cross-session coordination** — Multiple Claude sessions sharing one machine, aware of each other's load

## Compatibility

- Claude Code (CLI, desktop, or IDE extension)
- Any project with PostToolUse hooks that trigger heavy processes
- macOS, Linux, WSL

## License

MIT
