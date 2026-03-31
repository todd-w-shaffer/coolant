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

### Agent-first monitor (v3)

The monitor's visual hierarchy is agent-first: the primary signal is how many agents are running and what they're doing, not CPU percentages. Think of it like an ammo gauge — you see the pressure before you read a single number.

- **Agent gauge** — A braille fill bar where each block represents an active agent. Color shifts from green (1-2) to yellow (3-4) to red (5+). At a glance, you know the load.

- **Multi-trace agent chart** — Each agent gets its own color on a shared 5-row braille chart. Thin line traces (not filled area) overlay like btm's per-core CPU view. When agents pile up, the overlapping colored traces create a dense, hot visual. When idle, the chart is nearly invisible — a dark cockpit.

- **Job detection** — Each agent's current activity is extracted from its process tree: `tsc`, `bats`, `node`, `git`, or `idle`. A priority table ensures build tools surface over runtime noise.

- **Pressure badge** — Synthesizes agent count + CPU + memory into a single word: COOL, WARM, HOT, or MELTDOWN.

- **Sparkline history** — CPU and memory trend as filled-area braille sparklines with zone coloring (green/yellow/red thresholds).

```
 COOLANT  PARALLEL                                    22:51:34
 ⣿⣿⣿⣿⣿⣿⠂⠂⠂⠂  6 agents                                HOT

 ┌─ AGENTS ─── 6 active ────────────────────────────────────┐
 │  ⠀⠀⠀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤  │
 │  ⠀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤  │
 │  ⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤  │
 │  ⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤  │
 │  ⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤  │
 └──────────────────────────────────────────────────────────┘
  ● ag1 tsc  ● ag2 bats  ● ag3 node  ● ag4 tsc  ● ag5 idle  ● ag6 tsc

 ┌─ CPU ─── load 4.22 3.81 3.14 (8 cores) ────────── 52% ──┐
 │  ⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤  │
 │  ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿  │
 └──────────────────────────────────────────────────────────┘
 MEM ⣿⣿⣿⣿⣿⣿⣿⣿⣿⠂  91%  14.9G / 16.0G     SWAP ⣿⣿⠂⠂⠂  15%

 PROCESSES ──────────────────────────────────────────────────
   32708   12.3%   340M  claude
     ├─33001    0.2%    45M  bash
     │ └─33045   89.1%   820M  node tsc --noEmit
     ├─33002    0.1%    42M  bash
     │ └─33067   45.2%   650M  node vitest
     └─33003    0.3%    38M  bash
   ──────
   subtree: 147.2% cpu  1935M rss  6 procs

 EVENTS ─────────────────────────────────────────────────────
   21:43:05  parallel mode auto-engaged (threshold: 3)
   21:43:07  agent started (4 active)

 refresh 2s │ q: quit │ coolant v3
```

## Installation

```bash
# From the plugin directory
claude plugin install ./coolant --scope user

# Or symlink for development
claude --plugin-dir /path/to/coolant
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

### Monitor

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

```
coolant/
├── .claude-plugin/
│   └── plugin.json          # plugin manifest
├── hooks/
│   └── hooks.json           # hook definitions (PostToolUse, SubagentStart/Stop)
├── scripts/
│   ├── common.sh            # shared config, paths, log function
│   ├── agents.sh            # agent tracker: slot management, job detection
│   ├── sparkline.sh         # braille chart renderers (filled, line, gauge)
│   ├── monitor.sh           # live TUI dashboard
│   ├── toggle.sh            # manual parallel mode on/off/status
│   ├── parallel-gate.sh     # PostToolUse hook: suppress tsc in parallel mode
│   ├── agent-start.sh       # SubagentStart hook: increment counter
│   └── agent-stop.sh        # SubagentStop hook: decrement counter
├── skills/
│   └── parallel/
│       └── SKILL.md          # /coolant:parallel skill definition
└── tests/
    ├── test_helper.bash      # bats shared setup/teardown
    ├── agents.bats           # agent tracker tests
    ├── sparkline.bats        # renderer tests
    └── *.bats                # per-script test files
```

All pure shell. No runtime dependencies. Works on macOS and Linux.

## Testing

```bash
bats tests/           # run full suite
bats tests/agents.bats  # run a single file
```

## Roadmap

Coolant currently acts as a circuit breaker with agent-aware monitoring. The next steps move toward graduated thermal management:

- **Graduated throttling** — Progressive response as load increases: warn, reduce agent cap, suppress hooks, reap processes
- **Process-level actuators** — `renice` build processes, `SIGSTOP`/`SIGCONT` for pause/resume, targeted kill for runaway processes
- **Column layout** — btm-style side-by-side chart + trace legend when terminal width allows
- **Worktree lifecycle** — Detect and clean orphaned git worktrees from crashed agents
- **Status bar integration** — Compact thermal indicator as a braille cell in the Claude Code status line
- **Cross-session coordination** — Multiple Claude sessions sharing one machine, aware of each other's load

## Compatibility

- Claude Code (CLI, desktop, or IDE extension)
- Any project with PostToolUse hooks that trigger heavy processes
- macOS, Linux, WSL

## License

MIT
