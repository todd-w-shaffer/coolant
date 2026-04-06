# coolant

A resource management layer for Claude Code -- prevents machines from melting when parallel agents run unthrottled.

![thermal dashboard](assets/thermal-demo.gif)

## What it does

Run five Claude Code agents in parallel and your machine will try to compile, test, and lint everything at once. CPU pins, memory fills, the compressor spikes, and your whole system locks up. Coolant stops that.

The **dashboard** gives you a real-time read on system pressure so you know when to back off before things get ugly. The **hooks** do the throttling automatically -- capping concurrent test runners, suppressing build tools during parallel work, and tracking agent lifecycles. You get the throughput of parallel agents without the meltdown.

## Quick start

```bash
# Add the coolant marketplace (one time)
claude plugin marketplace add todd-w-shaffer/coolant

# Install the plugin
claude plugin install coolant

# Install the dashboard (macOS only)
curl -fsSL https://github.com/todd-w-shaffer/coolant/releases/latest/download/thermal-darwin-$(uname -m) -o /usr/local/bin/thermal && chmod +x /usr/local/bin/thermal

# See it in action
thermal --demo
```

## The dashboard

A single bottom-strip panel designed for a tmux split alongside your Claude Code session. Glance down, know the state.

- **Am I about to lock up?** -- CPU, MEM, and SWAP sparklines with severity coloring. Green is fine, yellow means watch it, red means act now.
- **Which session is the problem?** -- Each diamond is a Claude Code session. Color tracks the compilation dance: idle (gray), language spinning up (yellow), build tools running (orange), shell explosion (red, 30+ processes).
- **How many agents are running?** -- Breathing hexagons on the headline, one per active subagent. Pulsing means alive.
- **What's eating resources?** -- Live process counts by category (`build:003 shell:087`). Runtime labels (`node`, `go`, `rust`) appear when those processes are active and disappear when they're not.
- **System vitals** -- CPU/MEM/SWAP/GPU gauges, spawn/death rates, network connectivity.

Keyboard shortcuts: `h` help overlay, `q` quit.

## Hooks

| Hook | Script | What it does |
|------|--------|--------------|
| SessionStart | `preflight.sh` | Warns about missing worktree exclusions |
| PreToolUse | `gate.sh` | Caps test runners by agent count, suppresses build tools in parallel mode |
| SubagentStart | `agent-start.sh` | Increments agent counter, emits JSONL event |
| SubagentStop | `agent-stop.sh` | Decrements counter, auto-disengages parallel mode when agents finish |

The gate applies adaptive concurrency: `cap = floor((cores - 2) / agents)`, minimum 1. One agent gets generous parallelism; five agents each get a fair share. Test runners (vitest, jest, cargo test, go test, pytest) are always capped. Build tools, type checkers, and linters (tsc, eslint, cargo build, go vet, mypy, etc.) are suppressed entirely during parallel mode. Commands are matched regardless of wrappers (`npx tsc`, `env vitest`) or path prefixes.

## Skills

`/coolant:parallel` -- Toggle parallel mode to suppress per-edit typecheck hooks. Commands: `on`, `off`, `status`.

## Requirements

- **macOS** -- the dashboard uses native system APIs (mach kernel, vm_stat, sysctl). Prebuilt binaries for Apple Silicon and Intel.
- **bash 3.2+** -- hooks only, ships with macOS
- **tmux** -- optional, for running the dashboard in a bottom split pane

Hooks work on any platform with bash. The thermal dashboard is macOS-only.

## Project structure

```
hooks/          # bash hook definitions
scripts/        # hook implementations + shared config
thermal/        # Go thermal dashboard (bubbletea)
skills/         # /coolant:parallel skill
tests/          # bats test suite
```

## License

MIT
