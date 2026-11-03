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
coolant queue weights [--json | --toml] [--since <RFC3339>] [--until <RFC3339>]
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
weighs 1 core / 512 MB. The default weights are a starting point, not a
measurement of any machine: `coolant queue weights` sets them from your own
records. Weights, the budget and new classes live in
`~/.config/coolant/config.toml` (or `$COOLANT_CONFIG`):

```toml
[queue]
mode = "observe"      # "enforce" once `coolant queue weights` has set the numbers
cores = 8             # core budget (default: hw.ncpu)
memory_mb = 8192      # memory ceiling (default: half of physical RAM)

[queue.classes.mutation]
cores = 4
memory_mb = 3000

[queue.classes.check]  # a class table sets only the keys it names;
cores = 6              # a shipped whole class needs `whole = false`
whole = false          # before its cores count
```

## Environment

| Variable | Set by | Meaning |
|---|---|---|
| `COOLANT_LANE=<name>` | a launcher, for its sessions | The caller is a lane, not interactive. An unmarked caller is interactive and goes to the front of the waiting line. Subagents and child processes inherit it. |
| `COOLANT_TICKET=<seq>:<pid>` | the door, for its child | The held job's ticket. A `coolant run` inside it passes straight through: it runs at once, takes no ticket, and is recorded `nested`. It passes only while that ticket's holder is alive and is an ancestor of the caller. A stale or leaked ticket is refused with a warning, and the caller takes its own turn. Callers never set this. |
| `COOLANT_GRANTED_CORES=<n>` | the door, in enforce mode only | The cores this job was admitted with: an admission budget, not a worker count. A caller may read it or ignore it. Unset in observe mode. |
| `COOLANT_QUEUE_MODE` | a person, to try enforce in one shell | `observe` or `enforce`. Overrides config. |
| `COOLANT_THERMO` | tests, unusual installs | The exact thermo binary to use. |
| `COOLANT_QUEUE_DIR`, `COOLANT_RUNS`, `COOLANT_CONFIG` | tests | Override the paths below. |

**The grant is an admission budget.** It decides when a job starts, not
how wide it runs. A caller may ignore it and let its tools pick their own
width; a vitest suite left at its default runs `availableParallelism() − 1`
workers in either mode. A job that ignores its grant uses what it really
uses, so its class's weight must cover that, or enforce admits jobs that
then contend. Narrowing a job to its grant is the caller's choice and can
cost more than the workers' share: one vitest check ran 32% longer at a
6-worker grant than at its default 7.

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

## `coolant queue weights`

Reads `runs.jsonl` and prints, per class, what the jobs cost, the weight
config gives the class, and a weight suggested from the records. It is
read-only: it never writes config, the records or the queue. Copying a
suggestion into `config.toml`, and switching to enforce, are the user's
own edits.

The figures come from top-level observe-mode records. Left out, and
counted in the header: `nested` records, whose cost is already inside
their parent's record; `degraded` records, which held no ticket; and
`enforce` records, which ran at their grant, so feeding them back would
measure the weight with itself.

| Column | Meaning |
|---|---|
| jobs | records of the class |
| wall p50, p90 | `wall_s` |
| cores used p50, p90 | `cpu_s / wall_s`, over jobs longer than 1 s |
| peak MB p50, p90, max | `peak_mem_mb` |
| would wait | share of jobs recorded `would_wait` |
| configured | the weight config gives the class now |
| suggested | the rule below applied to the class |

Percentiles are nearest-rank.

**The rule.** cores = the p90 of cores used, rounded to a whole core, kept
within 1 and the core budget. memory_mb = the p90 peak memory, rounded up
to 256 MB. Never whole. A class with fewer than 5 jobs longer than 1 s gets
no suggestion. Cores used is an average over the whole run, so it
understates a job whose phases differ in width: a check that typechecks on
2 cores and then runs a suite on every core averages about 3.5.

**`check` and `suite` are weighted by policy, not by the rule.** Give each
more than half the core budget, so two cannot run at once. Measured cores
used is no evidence that they can share: on one 8-core machine a check
measured about 3.6 cores, yet two checks at once each ran about twice as
long (median 249 s alone, 491 s beside another job). Either `cpu_s`
undercounts, for instance workers that are killed and never reaped add
nothing to their parent's rusage, or the bottleneck is not CPU. The rule's
4 cores there would let two overlap. Their `suggested` column still shows
the rule; override it.

**Waits.** Three tables follow, of jobs that would have waited: as
recorded (under the weights configured when each job ran), and replayed
against the same overlaps under the configured weights and under the
suggested ones. Each gives the job's own median wall time and bounds on
the wait: it lies between when the first and when the last of the jobs
running at its start ended. These read the observed timeline, where
nothing waited, so they are rough. A recorded wait with no overlapping
record (the job ahead left none, e.g. it was killed) counts as
`unbounded`. To try candidate weights against your history before using
them, point `COOLANT_CONFIG` at a candidate file: the configured replay
uses it.

`--toml` prints the suggested `[queue.classes.<name>]` tables, and nothing
else. `--since` and `--until` keep records whose start is in that window.
`--json` is the same report:

```json
{
  "budget": { "cores": 8, "memory_mb": 8192 },
  "from": "…", "to": "…",
  "records": 730, "jobs": 394, "nested": 332, "enforce": 4, "degraded": 0,
  "rule": "…",
  "classes": [
    { "class": "check", "jobs": 40,
      "wall_s": { "p50": 290.89, "p90": 521.53, "max": 706.47 },
      "cores_used": { "n": 40, "p50": 3.31, "p90": 3.86, "max": 4.12 },
      "peak_mem_mb": { "p50": 3585.4, "p90": 3943.65, "max": 4052.1 },
      "would_wait": { "jobs": 12, "share": 0.3, "own_wall_p50_s": 484.32,
        "first_end_s": { "p50": 133, "p90": 308, "max": 421 },
        "last_end_s": { "p50": 133, "p90": 308, "max": 421 } },
      "configured": { "cores": 6, "memory_mb": 4096, "whole": false },
      "suggested": { "cores": 4, "memory_mb": 4096, "whole": false },
      "configured_would_wait": { "…": "…" },
      "suggested_would_wait": { "…": "…" } }
  ]
}
```

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
