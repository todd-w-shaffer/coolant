# Spec: Self-Documenting Config File Scaffold

## Goal

When `~/.config/coolant/config.toml` doesn't exist, `Load()` creates one pre-filled with all defaults and plain-English comments explaining each section. The file IS the documentation — users learn what to change by reading the file they're editing.

Wire into `install.sh` so new installs get the commented file from the start.

## Behavior

### Load() change

1. `Load()` resolves the config path (env override or `~/.config/coolant/config.toml`).
2. If the file **does not exist**: call `scaffold(path)` to write the commented default, then return (defaults are already loaded).
3. If the file **exists**: parse it as today (no change).
4. `scaffold()` creates parent directories (`mkdir -p`) and writes the full commented TOML.

### install.sh change

After step 3 (statusline), add a step that calls `thermo --scaffold-config` (or simply documents that the config is auto-created on first run). Preferred: just mention it in the final output — "config lives at ~/.config/coolant/config.toml, created on first run with commented defaults."

## Scaffolded file content

The file below is the exact TOML that `scaffold()` writes. Every value shown is the compiled default. Comments use `#` and are written to teach, not just label.

```toml
# ─── coolant config ────────────────────────────────────────────
#
# This file was created with defaults on first run. Every value
# shown is what coolant uses out of the box — change only what
# you need. Delete a line to fall back to the compiled default.
#
# Syntax is lenient: 65, 65.0, "65", or 65% all work for
# percentage fields. Gigabyte fields accept 4, 4.0, 4GB, "4 GB".
#
# Location: ~/.config/coolant/config.toml
# Override:  COOLANT_CONFIG=/path/to/file.toml thermo


# ─── memory ────────────────────────────────────────────────────
# RAM usage thresholds as percent of total physical memory.
# These drive the threat score — cross a line and the dashboard
# shifts color. On a 16GB machine, 65% ≈ 10.4GB used.
#
# Most Claude Code sessions sit around 50-60% with a few agents.
# If you run other heavy apps alongside (Docker, Xcode, Chrome),
# consider lowering warm_pct to get earlier warnings.

[memory]
warm_pct = 65     # start watching — pressure building
hot_pct  = 80     # getting uncomfortable — swap likely soon
crit_pct = 90     # swap is happening — time to shed load


# ─── cpu ───────────────────────────────────────────────────────
# CPU utilization thresholds (percent, all cores averaged).
# Short spikes during compilation are normal. Sustained high CPU
# means multiple agents are compiling or running tests at once —
# that's exactly what coolant's gate system throttles.

[cpu]
warm_pct = 75     # elevated but fine
crit_pct = 90     # sustained ceiling — things are hot


# ─── swap ──────────────────────────────────────────────────────
# Swap usage thresholds in gigabytes.
# macOS uses swap proactively — seeing 1-2GB of swap while you
# still have free RAM is normal and not a problem. Don't panic
# until it climbs well past warm. On an 8GB machine, lower these
# to roughly half (1 / 4 / 10).

[swap]
warm_gb = 2       # baseline noise — macOS being macOS
hot_gb  = 8       # real memory pressure — performance degrading
crit_gb = 20      # meltdown territory — machine is thrashing


# ─── headroom ─────────────────────────────────────────────────
# Free memory headroom after subtracting estimated process weight.
# "Headroom" = available RAM minus what running Claude agents are
# likely to consume. This catches the case where RAM looks fine
# now but a running build is about to eat the rest.
#
# warn fires first (more room left), crit fires when it's tight.
# Set both to 0 to disable headroom warnings entirely.
# On an 8GB machine, try warn = 2, crit = 1.

[headroom]
warn_gb = 4       # heads-up — room is getting tight
crit_gb = 2       # critical — next big allocation may swap


# ─── spawn ─────────────────────────────────────────────────────
# Process spawning thresholds. Claude agents launch bursts of
# child processes (npm install, cargo build, etc). These settings
# control when a burst triggers an alert.
#
# burst_threshold: how many new processes in a single 150ms tick
# before an alert fires. 8 is conservative — a cargo build can
# spawn 20+ in one tick.
#
# rate_escalation: the smoothed (EMA) spawn rate that bumps the
# threat score. Higher = more tolerant of sustained spawning.

[spawn]
burst_threshold = 8       # procs/tick to trigger burst alert
rate_escalation = 10.0    # smoothed spawn rate that escalates threat


# ─── score ─────────────────────────────────────────────────────
# Threat score → level mapping. The classifier sums individual
# signals (memory, CPU, swap, spawn rate) into a composite score.
# These boundaries decide when the dashboard changes severity.
#
# Example: memory crosses warm (+1) and CPU crosses warm (+1)
# gives score=2, which is WARM. Add swap crossing hot (+2) and
# you're at 4 = HOT. One more signal hits meltdown.
#
# Raise these if you want a calmer dashboard that doesn't
# escalate until things are truly bad.

[score]
warm     = 1      # score ≥ 1 → WARM (yellow)
hot      = 3      # score ≥ 3 → HOT (orange/red)
meltdown = 5      # score ≥ 5 → MELTDOWN (full red, alerts firing)


# ─── sparklines ────────────────────────────────────────────────
# Color breakpoints for the gauge sparklines. When a value
# exceeds warn it turns yellow; past crit it turns red.
# These are independent of the threat score — they only affect
# the visual color of individual sparkline charts.

[sparklines]
cpu_warn  = 70        # CPU % — yellow above this
cpu_crit  = 90        # CPU % — red above this
mem_warn  = 60        # memory % — yellow above this
mem_crit  = 80        # memory % — red above this

# Decompressions are macOS memory compressor operations per tick.
# High values mean the system is actively compressing/decompressing
# pages — a sign of real memory pressure even if "used %" looks OK.
decomp_warn = 5000    # decompressions/tick — yellow
decomp_crit = 20000   # decompressions/tick — red

# Swap sparkline uses GB, not percent. Aligns with [swap] above.
swap_warn_gb = 2.0    # swap GB — yellow
swap_crit_gb = 8.0    # swap GB — red

# GPU utilization (Apple Silicon only, via ioreg).
# If you don't use GPU-heavy workflows, these won't matter.
gpu_warn = 60         # GPU % — yellow
gpu_crit = 85         # GPU % — red


# ─── categories ────────────────────────────────────────────────
# Per-category process count thresholds: [warm, hot].
# When a category's process count crosses warm, its row turns
# amber in the headline bar. Past hot, it turns red.
#
# These are tuned for typical Claude Code usage. Node processes
# eat ~500MB-1.5GB each, so even one is worth watching on a
# constrained machine. Shell processes are ephemeral and cheap —
# dozens are normal during a build.
#
# Add any category name from the headline bar. Unknown categories
# fall back to `default`.

[categories]
node    = [1, 8]      # node: ~1GB each — warm on first appearance
go      = [1, 4]      # go: heavy per-process, especially during build
python  = [1, 6]      # python: variable weight, watch for runaway
rust    = [1, 4]      # rust: heavy compilation + linking
swift   = [1, 4]      # swift: heavy compilation + linking
build   = [1, 3]      # build: few procs but each spawns heavy trees
shell   = [15, 40]    # shell: ephemeral and cheap — lots are normal

default = [10, 25]    # fallback for categories not listed above

# Shell explosion threshold — when shell count exceeds this,
# coolant enters "shell explosion" mode. This usually means an
# agent kicked off a big build (language → build → shell cascade).
shell_explosion = 30
```

## Implementation notes

- The scaffold string should be a Go `const` or embedded file in `internal/config/`, not generated from struct reflection. Hand-written comments are the whole point.
- `scaffold()` must not overwrite an existing file.
- `scaffold()` should create parent dirs with `os.MkdirAll(dir, 0755)`.
- Write atomically: write to a temp file in the same directory, then rename. This prevents partial files if the process is interrupted.
- On write error (permissions, disk full), log a warning and continue — the dashboard should still run with compiled defaults.
- Add a test: `Load()` with a non-existent path should create the file, and the created file should parse back cleanly with `Load()` returning all defaults.

## install.sh change

Replace the final output block (after "✓ coolant installed") entirely. The current output leads with `--demo`, which is a dev tool. New users want to know what they can customize, not how to run a fake mode.

Current:
```
    ✓ coolant installed

    thermo --demo      see the dashboard
    thermo             monitor your system
```

New:
```
    ✓ coolant installed

    thermo                 start monitoring
    --theme                classic · iron · mono · frappe
    --animation            default · calm · intense

    config created at ~/.config/coolant/config.toml
    defaults are tuned for your machine — no changes needed.
    tweak thresholds anytime; press [h] in thermo for details.
```

No separate install step for the config — it self-creates on first `thermo` run.

## Help overlay change

The `[h]` help overlay in `layout/horizontal.go` (`helpView()`) currently shows sparkline legends, session phases, agent glyphs, and keybindings. It does NOT mention the config file. The install message promises "press [h] in thermo for details" — we need to deliver on that.

Add a config line to the help overlay, in the keybinding row or as a new row:

```
~/.config/coolant/config.toml — thresholds, sparkline breakpoints, categories
```

Keep it short — this is a pointer, not documentation. The file itself is the documentation.

## Implementation notes (clarifications)

- The scaffold string should be a Go `const` or embedded file in `internal/config/`, not generated from struct reflection. Hand-written comments are the whole point.
- `scaffold()` must not overwrite an existing file.
- `scaffold()` should create parent dirs with `os.MkdirAll(dir, 0755)`.
- Write atomically: write to a temp file in the same directory, then rename. This prevents partial files if the process is interrupted.
- On write error (permissions, disk full), log a warning and continue — the dashboard should still run with compiled defaults.
- **Path resolution stays in `main.go`.** `Load()` takes a path argument; `main.go` already resolves `COOLANT_CONFIG` env → `~/.config/coolant/config.toml` fallback (lines 167-173). Scaffold uses the same path passed to `Load()` — no new resolution logic.
- **Drift guard:** `TestScaffoldRoundTrip` is the safety net against the hand-written TOML drifting from `Defaults()`. If someone changes a default in `tuning.go` but forgets to update the scaffold string, this test fails. The spec calls this out explicitly so future maintainers know why that test exists.

## Test plan

1. **Red**: `TestScaffoldCreatesFile` — call `Load()` with a path in a temp dir that doesn't exist. Assert the file was created and is non-empty.
2. **Red**: `TestScaffoldRoundTrip` — scaffold a file, then `Load()` it again. Assert all values equal `Defaults()`. **This is the drift guard** — if the scaffold TOML has stale values that don't match compiled defaults, this test catches it.
3. **Red**: `TestScaffoldNoOverwrite` — write a custom config, call `Load()`. Assert the file content is unchanged (not overwritten with defaults).
4. **Red**: `TestScaffoldParentDirs` — use a path with multiple non-existent parent dirs. Assert they're created.
5. **Green/Refactor**: implement `scaffold()` and wire into `Load()`.
6. **Separate cycle**: add config path to `helpView()` in `layout/horizontal.go`.
