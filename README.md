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

### Thermal dashboard

A terminal dashboard that monitors Claude Code's process tree in real time. Built in Go with [bubbletea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss) — a single binary collects system data directly and renders a thermal strip.

- **Thermal headline** — Overall threat level with personality quip + five category boxes (test/build/run/search/shell) that glow from invisible to red based on per-category danger thresholds.

- **Braille sparklines** — CPU%, MEM%, and SWAP history as severity-colored braille bars (green/yellow/red), each dot colored by its own value.

- **Rates + stats** — Spawn/death/net rates and current system metrics (CPU, MEM, SWAP) in fixed-width format.

- **Offline detection** — Detects API connectivity loss and switches to a distinct visual mode with rainbow sparklines.

## Prerequisites

- **macOS or Linux** (bash 3.2+)
- **Go 1.26+** — for building the thermal dashboard (`brew install go`)
- **tmux** — optional, if you want to run the thermal dashboard alongside your Claude session in a split (`brew install tmux`)
- **bats-core** — optional, for running tests (`brew install bats-core`)

## Installation

```bash
# 1. Install the Claude Code plugin
claude plugin install ./coolant --scope user

# Or symlink for development
claude --plugin-dir /path/to/coolant

# 2. Build the thermal dashboard
cd thermal && go build -o ../bin/thermal ./cmd/thermal/
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

### Thermal dashboard

```bash
# Standalone — Go binary collects system data directly
./bin/thermal

# Demo mode — synthetic data, no live Claude session needed
./bin/thermal --demo
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
├── thermal/                 # Go — thermal dashboard binary
│   ├── cmd/thermal/main.go  # bubbletea app entry point
│   ├── internal/
│   │   ├── collector/        # system stats + process tree scanning
│   │   ├── model/            # AppState, threat classification, personality
│   │   ├── widgets/          # headline, gauges, rates, alerts, sparklines
│   │   ├── layout/           # horizontal strip compositor
│   │   ├── demo/             # synthetic data generator (--demo)
│   │   └── ui/               # colors, thresholds
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

The Go binary collects system data directly via `sysctl`, `vm_stat`, and `ps` — no external dependencies.

## Testing

```bash
# Bash tests (hooks, plumbing)
bats tests/
bats tests/agents.bats

# Go tests (thermal dashboard)
cd thermal && go test ./...
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
