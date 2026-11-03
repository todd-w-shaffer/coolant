# The work queue: interface

One queue for every CPU- or memory-heavy job on the machine, across every
repo and session. A job enters through the door, `coolant run`, which
runs the command and answers its exit status. This page is the contract a
caller such as a repo's npm scripts, git hooks or vitest globalSetup
builds against.

## The door

```bash
coolant run --class <class> -- [NAME=value ...] <command> [args...]
coolant queue [--json]
```

Leading `NAME=value` words after `--` set the command's environment, as
env(1) does (`coolant run --class suite -- CI=1 vitest run`).

`coolant` is `scripts/coolant` in this repo (and in the installed
plugin). Put it on `PATH` by linking it into a directory every shell
sees, including non-interactive ones (for zsh, one exported from
`~/.zshenv`), e.g. `ln -s <checkout>/scripts/coolant ~/.local/bin/coolant`. The shim finds a
`thermo` that has the door (`thermo --door-version` prints `1`) and execs
`thermo run` / `thermo queue`. It looks in this order:

1. `$COOLANT_THERMO`. When it is set, only it is tried.
2. `bin/thermo` in the checkout the shim lives in, found through symlinks.
3. `thermo` on `PATH`.

**Calling it from a non-interactive shell** (a git hook, an npm script, a
launcher) uses the same line. Do not depend on `PATH`. Resolve it once:

```bash
COOLANT="${COOLANT:-$(command -v coolant)}"   # or set COOLANT to the shim's path
if [ -n "$COOLANT" ] && [ -x "$COOLANT" ]; then
  "$COOLANT" run --class check -- npm run check:inner
else
  npm run check:inner   # no door on this machine: run as before
fi
```

The door never fails a job because the queue is missing. If no door-capable
thermo is found, or the queue directory cannot be used, it prints one
`coolant:` line to stderr and runs the command unqueued.

## Classes

| Class | Meant for | Default weight |
|---|---|---|
| `suite` | a whole test suite | the whole core budget, 3584 MB |
| `check` | a full check (typecheck + lint + suite …) | the whole core budget, 4608 MB |
| `files` | a few test files | 2 cores, 1536 MB |
| `typecheck` | `tsc --noEmit` and the like | 1 core, 1536 MB |
| `lint` | type-aware lint | 1 core, 2048 MB |
| `build` | a small build (esbuild, Tailwind) | 1 core, 512 MB |

A class that takes the whole budget runs alone. An unknown class warns and
weighs 1 core / 512 MB. Weights, the budget and new classes live in
`~/.config/coolant/config.toml` (or `$COOLANT_CONFIG`):

```toml
[queue]
mode = "observe"      # "enforce" once a day of records sets the numbers
cores = 8             # core budget (default: hw.ncpu)
memory_mb = 8192      # memory ceiling (default: half of physical RAM)

[queue.classes.mutation]
cores = 4
memory_mb = 3000
```

## Environment

| Variable | Set by | Meaning |
|---|---|---|
| `COOLANT_LANE=<name>` | a launcher, for its sessions | The caller is a lane, not interactive. An unmarked caller is interactive and goes to the front of the waiting line. Subagents and child processes inherit it. |
| `COOLANT_TICKET=<seq>:<pid>` | the door, for its child | The held job's ticket. A `coolant run` inside it passes straight through: it runs at once, takes no ticket, and is recorded `nested`. It passes only while that ticket's holder is alive and is an ancestor of the caller. A stale or leaked ticket is refused with a warning, and the caller takes its own turn. Callers never set this. |
| `COOLANT_GRANTED_CORES=<n>` | the door, in enforce mode only | The cores this job was admitted with. A repo's vitest config reads it for `maxWorkers`, and so on. Unset in observe mode, so observed runs show unthrottled behaviour. |
| `COOLANT_QUEUE_MODE` | a person, to try enforce in one shell | `observe` or `enforce`. Overrides config. |
| `COOLANT_THERMO` | tests, unusual installs | The exact thermo binary to use. |
| `COOLANT_QUEUE_DIR`, `COOLANT_RUNS`, `COOLANT_CONFIG` | tests | Override the paths below. |

A vitest config that honours the grant:

```js
const granted = Number(process.env.COOLANT_GRANTED_CORES);
export default defineConfig({
  test: { maxWorkers: Number.isInteger(granted) && granted > 0 ? granted : undefined },
});
```

## Exit status

The command's own status, always, including when the door degrades. A
command killed by signal N answers 128+N. The door's own statuses are:

| Status | Meaning |
|---|---|
| 64 | malformed call: no `--class`, nothing after `--`, unknown flag |
| 126 | the command was found but could not start |
| 127 | the command was not found |
| 128+N | the wait was abandoned by signal N (Ctrl-C while queued is 130) |

## Waiting

Observe mode (the shipped default) never waits.

In enforce mode, a job is admitted while the running total plus its own
weight fits both the core budget and the memory ceiling. A job is also
admitted when nothing is running, so an oversized job runs alone rather
than never. Waiting jobs are served interactive first, then first come,
first served, and the head of the line is never passed by a smaller job
behind it. A running job is never stopped.

A waiting job prints one line to stderr naming what is ahead of it, and
prints again each time that changes:

```
coolant: waiting, 2 ahead: pid 51234 (nightly, suite) in ~/Desktop/apps/myapp [running]; pid 51300 (interactive, typecheck) in ~/Desktop/apps/coolant [waiting]
```

No wait has a clock. A wait ends when the work ahead ends: when its
process exits or is killed, since the kernel drops the holder's lock. A
long wait inside a Claude Code Bash call can pass that tool's own time
limit, so a session should run whole suites in the background.

## Where things live

| Path | What |
|---|---|
| `~/.coolant/queue/` | Tickets: `<seq>.json`, plus `queue.lock` and `next`. Fixed and outside every repo, so every repo, worktree and session shares one queue. Safe to inspect. Do not edit while jobs run. |
| `~/.coolant/runs.jsonl` | One record per job, appended under flock. |
| `~/.config/coolant/config.toml` | `[queue]` mode, budget and class weights. |

## Records (`runs.jsonl`, `schema: 1`)

Each line is a JSON object:

| Field | Meaning |
|---|---|
| `ts`, `end` | UTC start (after any wait) and end of the command |
| `mode` | `observe` or `enforce` |
| `class`, `lane`, `interactive` | how the caller entered |
| `repo`, `worktree`, `cwd` | main checkout (shared by its worktrees), the worktree, and the working directory |
| `cmd` | the command, shell-quoted |
| `exit` | the status the door answered |
| `wall_s` | wall time of the command, waiting excluded |
| `cpu_s` | user+sys CPU of the command's process tree |
| `peak_mem_mb`, `mem_samples`, `mem_sample_ms` | peak memory of the whole process tree; see below |
| `load_start`, `load_end` | 1-minute load average at start and end |
| `wait_s` | seconds spent waiting (always 0 in observe) |
| `granted_cores` | the grant (enforce only) |
| `would_wait` | whether enforce would have held this job when it started (observe only) |
| `others_running`, `others_cores`, `others_mem_mb` | what else was running, by class weight, when it started |
| `nested` | ran inside a held job's ticket; its resources are also inside that job's record |
| `degraded` | ran unqueued because the queue could not be used |
| `seq`, `pid` | ticket number and the door's pid |

**How memory is measured.** Every 200 ms the door walks the live process
tree under the command (`proc_listchildpids`) and sums each process's
`phys_footprint` from `proc_pid_rusage`. That is the figure Activity
Monitor's Memory column shows, and unlike RSS it does not count shared
library pages once per process. The peak is the largest sum seen, max'd
with the command's own `ru_maxrss`. A spike shorter than 200 ms can be
missed. A process that daemonizes out of the tree stops being counted —
notably a Gradle or Maven build daemon, whose work happens outside the
door's tree, so those records show only the thin client.

**How CPU is measured.** `cpu_s` is user+sys from the command's `wait4`
rusage. macOS folds each reaped descendant's usage into its parent at
exit, so this covers every process in the tree that was waited for.

## `coolant queue --json`

```json
{
  "mode": "enforce",
  "budget": { "cores": 8, "memory_mb": 8192 },
  "running": [
    { "seq": 41, "pid": 51234, "lane": "nightly", "priority": 1, "class": "suite",
      "cores": 8, "mem_mb": 3584, "whole": true, "state": "running",
      "cwd": "…", "repo": "…", "worktree": "…", "cmd": "npm run check",
      "queued_at": "…", "started_at": "…",
      "elapsed_s": 62, "estimate_s": 240, "remaining_s": 178, "known": true }
  ],
  "waiting": [
    { "seq": 42, "pid": 51300, "class": "typecheck", "…": "…",
      "start_in_s": 178, "estimate_s": 31, "known": true }
  ]
}
```

`waiting` is in service order. An estimate is the median `wall_s` of the
last 20 records of the same class in the same repo, else the same class
anywhere. `known` is false when there are no records for the job, or when
it waits behind a job with no records.
