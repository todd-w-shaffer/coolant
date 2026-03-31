# coolant

**Thermal management for Claude Code parallel agents.**

## The Problem

Claude Code can spawn multiple parallel agents, each running in its own worktree. This is powerful — until the build/test phase hits. Every agent independently triggers TypeScript compilation (`tsc --noEmit`), test suites (Vitest), and bundler passes (Wrangler/Webpack/esbuild). Each of these spawns its own Node.js process.

With 6-15 concurrent agents, that's 6-15 simultaneous `tsc` processes, each allocating hundreds of megabytes of memory for the type checker's AST. Add Vitest worker pools and bundler passes on top, and you're looking at dozens of Node processes competing for CPU and RAM simultaneously.

**Real-world damage:** During a routine parallel build session on an M-series MacBook Pro, this cascade consumed **85GB of swap space**, effectively freezing the machine. There's no built-in concurrency limit in Claude Code, no resource awareness, and no coordination between agents. Every agent assumes it has the machine to itself.

This isn't a theoretical problem. It's a `tsc` fork bomb with extra steps.

## How Coolant Fixes It

Coolant is a Claude Code plugin that prevents resource exhaustion through three mechanisms:

### 1. Hook Suppression (the fuse)

Per-edit `tsc --noEmit` hooks are the biggest multiplier. A single agent editing 20 files triggers 20 TypeScript compilations. Multiply by 8 agents and that's 160 `tsc` invocations during the code-writing phase alone — before any intentional build even runs.

Coolant suppresses these hooks when parallel mode is active. Agents write code freely (cheap I/O), and type-checking happens once at the end, not on every keystroke.

### 2. Agent Counting (the thermostat)

`SubagentStart` and `SubagentStop` hooks track how many agents are alive. When the count crosses a configurable threshold (default: 3), Coolant automatically engages parallel mode. When agents finish and the count drops back to zero, it disengages. No manual intervention needed.

### 3. Staggered Validation (the cool-down)

The skill provides a `/coolant:parallel` command that reminds the orchestrating agent to run `check` and `build` sequentially after parallel work completes — not concurrently across all agents. The expensive validation phase gets serialized; the cheap code-writing phase stays parallel.

## Installation

```bash
# From the plugin directory
claude plugin install ./coolant --scope user

# Or symlink for development
claude --plugin-dir /path/to/coolant
```

## Usage

Coolant works automatically once installed. The `SubagentStart`/`SubagentStop` hooks count active agents and engage parallel mode when the threshold is hit.

### Manual override

```
/coolant:parallel on      # force parallel mode on
/coolant:parallel off     # force parallel mode off
/coolant:parallel status  # check current state
```

### What happens in parallel mode

- Per-edit `tsc --noEmit` hooks are suppressed
- A system message reminds agents to defer validation
- When all agents complete, run your build gate manually:

```bash
npm run check    # typecheck + tests, once
npm run build    # bundle, once
```

## Configuration

The agent threshold defaults to 3. Override via the toggle script:

```bash
COOLANT_THRESHOLD=5 /coolant:parallel on
```

## How It Works Internally

```
Normal mode (1-2 agents):
  Edit file -> tsc --noEmit -> immediate feedback -> next edit

Parallel mode (3+ agents):
  Edit file -> [tsc suppressed] -> next edit -> ... -> all agents done
  -> npm run check (once) -> npm run build (once)
```

The trade-off is intentional: you lose per-edit type feedback during parallel work, but you keep your machine alive. Type errors surface in the single validation pass at the end, the same as CI would catch them.

## Compatibility

- Claude Code (CLI, desktop, or IDE extension)
- Any project with PostToolUse hooks that trigger heavy processes (tsc, vitest, webpack, etc.)
- macOS, Linux, WSL

## License

MIT
