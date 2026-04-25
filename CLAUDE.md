# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## ABSOLUTE RULES

**NEVER delete files or directories without explicit permission.** No exceptions. No "cleanup." No "safe to delete." ASK FIRST. Always. Even if the file looks stale, orphaned, or unnecessary. Even if you created it. Even if it's untracked. The user runs with permissive settings — that trust must not be abused for destructive operations.

## Cross-repo spec access

TUI specs live in the private companion repo at
`~/Desktop/apps/thermal-enterprise/docs/backlog/tui/`. Two gitignored
paths in this repo coordinate with that one:

- `docs/.tui-specs` — read-side symlink. Points at enterprise's
  `docs/backlog/tui/`. Use for reading specs during `/spec-to-ship`
  or any coolant-side work that needs to consult a spec.
- `docs/_drafts/` — write-side scratch dir. Gitignored so drafts
  never stage or commit. Write new enterprise-bound specs here when
  they emerge mid-flow from a coolant session.

### Rules

- **Reading enterprise material from coolant CWD.** Always OK via
  `docs/.tui-specs/` symlink or absolute path.
- **Drafting enterprise material from coolant CWD.** Allowed into
  `docs/_drafts/` only. Gitignored → classifier never sees it →
  zero commit risk. No CWD switch required.
- **Promoting drafts and committing to enterprise.** Requires a CWD
  switch to thermal-enterprise. A sweep prompt from an enterprise
  session reads coolant's `docs/_drafts/`, moves ready files
  (frontmatter `status: ready`) into `docs/backlog/tui/`, and commits
  from enterprise CWD.
- **Editing or amending existing enterprise specs.** Requires a CWD
  switch. A tracked enterprise file is one `git add -f` away from a
  real leak — don't edit live enterprise specs from coolant CWD.

### Readiness signal

Drafts carry frontmatter `status: draft` while in flight;
`status: ready` when the coolant session considers them ready for
the enterprise sweep.

### Spec lifecycle

1. **Seed / draft** — coolant CWD, write to `docs/_drafts/`
   (`status: draft`); or thermal-enterprise CWD, write directly.
2. **Promote** — thermal-enterprise CWD, sweep reads coolant's
   `_drafts/` (`status: ready`), moves to `backlog/tui/`, commits.
3. **Audit / expand** — thermal-enterprise CWD (tracked specs only).
4. **Implement** — coolant CWD, read spec via `docs/.tui-specs/`.
5. **Archive** — thermal-enterprise CWD, `git mv` shipped spec to
   `backlog/tui/archive/`.

## Quick reference

```bash
cd thermal && go test ./...              # Go tests
bats tests/                              # bash tests
cd thermal && go build -o ../bin/thermo ./cmd/thermal/  # build
./bin/thermo --demo                      # run with synthetic data
./bin/thermo                             # run with live data
```

## Contributor setup

After cloning, run once:

```bash
./scripts/install-hooks.sh
```

This sets `core.hooksPath=.githooks`, which activates the
commit-classification pre-push hook (blocks private/strategy content from
leaking into this public repo). A companion Claude Code PreToolUse hook at
`.claude/hooks/classify-staged.sh` runs the same checks on `git commit`
calls. Rules live in `.githooks/{blocklist,allowlist}.txt`; see
`.githooks/README.md` for the short version.

Any existing hooks in `.git/hooks/` (e.g., graphify's `post-commit`)
are automatically copied into `.githooks/` so they keep running under
the new `core.hooksPath`. Copied hooks are gitignored — they stay
local-only.

**Running tests cold:** `tests/test_helper.bash::setup_git_tmprepo`
chmods the hook scripts executable on demand, so `bats tests/` works in
a fresh clone even before `install-hooks.sh` has been run.

## Commit style

Subject line in imperative mood. Body includes a `Recipe:` block (a distilled prompt that would reproduce the change in one shot) and a `Changes:` block (per-file narrative). No Co-Authored-By lines.

## TDD

Strict red-green-refactor for **any** code that can break — bash, Go, or otherwise. No exceptions. One feature per cycle.

1. **Red** — write a failing test first, do NOT write implementation yet
2. **Green** — implement the minimum code to pass, nothing more
3. **Refactor** — improve quality while keeping tests green, do not skip

Bug fixes follow the same cycle: write a test that reproduces the bug (red), fix it (green), clean up (refactor). See **Testing** below for per-language conventions and commands.

## Project structure

Two layers: **bash** for hooks, plumbing, and data collection; **Go** for
visualization. The bash↔Go seam is a JSONL event log (see the "JSONL
event bus" convention below).

**Repo-level layout:**
- `VERSION` — root-level semver stamp, updated by auto-release workflow
- `hooks/`, `scripts/` — bash plumbing (SessionStart / PreToolUse /
  Subagent hooks, reconciled counters, JSONL event emission, `upgrade.sh`)
- `thermal/` — Go thermal dashboard (packages below)
- `.claude-plugin/`, `install.sh`, `claude-statusline/` — plugin
  manifest, installer, and the braille statusline
- `docs/` — design docs, theming plans, backlog specs
- `dev/otel/` — local Prometheus+Grafana dogfood stack (see below)
- `tests/` — bats tests for the bash layer
- `skills/coolant/SKILL.md` — the `/coolant` skill entry
- `assets/` — VHS tapes + demo GIFs
- `.githooks/` — pre-push commit-classification hook + blocklist/allowlist
  data files (activated by `scripts/install-hooks.sh`)
- `.claude/hooks/classify-staged.sh` — companion Claude PreToolUse hook

**Thermal dashboard (`thermal/internal/`) — conceptual groups:**
- **Sampling** — `collector/` gathers system metrics (CPU/MEM/procs/GPU/battery/network,
  cgo mach ticks, JSONL event tailer); `model/` holds AppState, threat level
  formula, ring buffers, idle personality
- **Rendering** — `theme/` (color palettes, HCL blend LUTs), `anim/` (motion
  profiles, orthogonal to theme), `widgets/` (sparklines, headline, LCD, gauges,
  agent dots, heat bloom/rails, alerts, battery), `layout/` (bottom-strip compositor
  with `overlayContent` for `h` help and `i` intel overlays, category filter dim
  feedback via bubblezone click regions), `ui/` (semantic colors, glyphs, helpers,
  `CatZoneID`), `keys/` (shared keybinding registry)
- **Plumbing** — `config/` (timing, thresholds, EMA, animation defaults),
  `demo/` (scripted 6-phase narrative for `--demo`), `version/` (ldflags-injected
  build version), `updater/` (daily-TTL staleness check against `VERSION` on main)
- **Entry points** — `cmd/thermal/` (bubbletea app), `cmd/swatch/` and
  `cmd/brailletext/` (preview tools)

For the full package inventory, call graph, and community structure,
read `graphify-out/GRAPH_REPORT.md` — regenerated by a post-commit
hook, always current.

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

Thermal dashboard rendered via bubbletea; runs as a bottom tmux strip or standalone. Build/run commands in **Quick reference** at the top of this file.

**Dependencies:** Go 1.25+, cgo (for mach CPU ticks), `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/harmonica`, `github.com/lucasb-eyer/go-colorful`.

**Naming note:** source directory is `thermal/`, binary is `thermo` (avoids colliding with macOS `/usr/bin/thermal`).

## Distribution

Plugin and dashboard ship separately. The plugin installs via Claude Code's marketplace system; the dashboard is a prebuilt binary on GitHub Releases.

- **Marketplace:** `todd-w-shaffer/marketplace` repo hosts the plugin manifest. Coolant is a git submodule under `plugins/coolant`. A GitHub Action (`.github/workflows/notify-marketplace.yml`) fires `repository_dispatch` on every push to main, triggering the marketplace repo to auto-update the submodule.
- **Binaries:** `thermo-darwin-arm64` and `thermo-darwin-amd64` attached to GitHub Releases. Cut automatically by `.github/workflows/auto-release.yml` on every push to `main` that touches `thermal/**`, `scripts/**`, or `hooks/**` — bumps minor semver (seeded at v0.4.0), cross-compiles both arches with `CGO_ENABLED=1` on macos-latest, and runs `gh release create --target $SHA`. Bash-only commits cut releases with binaries identical to the previous release; that's a noisy version bump but architecturally honest, since coolant's runtime spans both layers. Manual major bumps: push a `vX.Y.Z` tag to trigger `release.yml`. Local build: `CGO_ENABLED=1 GOARCH=arm64 go build -o bin/thermo-darwin-arm64 ./cmd/thermal/ && CGO_ENABLED=1 GOARCH=amd64 go build -o bin/thermo-darwin-amd64 ./cmd/thermal/` (from `thermal/`).
- **Install script:** `install.sh` downloads the binary for the user's arch, asks where to put it, and optionally installs the braille statusline to `~/.claude/` with settings.json patching. `install.sh --upgrade` delegates to `scripts/upgrade.sh` for silent re-fetch of both artifacts.
- **Staleness detection:** Daily TTL check against `VERSION` on main, cached at `$TMPDIR/coolant-$USER.latest-version` (shared between Go updater and bash statusline). Thermo shows "update available" in the notification banner; statusline appends a dim yellow ⬆ glyph. `[updates]` TOML section controls TTL and opt-out.

## Conventions

### JSONL event bus (the bash↔Go seam)

Bash hooks write to `$TMPDIR/coolant-$USER.events.jsonl`. Go's event tailer (`collector/events.go`) polls this file at 500ms. This is the *only* runtime data path between the two layers — there are zero direct calls. The JSONL schema is defined by `coolant_event` in `common.sh` and parsed by `TailEvents` in `events.go`.

### Bash (hooks, plumbing)

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_EVENTS`, `COOLANT_AGENT_STARTS`, `COOLANT_DEGRADED_COUNT`, `COOLANT_THRESHOLD`).
- Hook scripts log human-readable events via `coolant_log "message"` and structured JSONL via `coolant_event '"key":"value"'`. `coolant_event` injects `"schema":1` into every envelope and serializes `>>` appends behind `coolant_lock` (mkdir-mutex). The lock is **NOT reentrant** — callers already holding `coolant_lock` MUST release it before invoking `coolant_event`, or they deadlock for ~1s and fall through to an unsynchronized write that bumps `$COOLANT_DEGRADED_COUNT`.
- JSON field extraction from hook stdin uses `_json_field` (top-level), `_nested_command` (tool_input.command), and `_extract_escaped` (extract + escape for re-emission into JSONL) — no jq dependency.
- Values interpolated into JSONL must pass through `_json_escape` to handle backslashes and quotes. Agent metadata extraction uses `_extract_agent_fields`, which calls `_extract_escaped` for each field.
- State lives in `$TMPDIR/coolant-$USER.*` files — lockfile, counter, event log, agent-starts tsv (agent_id<TAB>epoch_s for duration_s computation on stop; 24h entries self-prune), version cache. No databases, no config files at runtime. `$TMPDIR` is per-user on macOS (`/var/folders/.../T/`), avoiding `/tmp` symlink attacks.
- **Gate system**: `gate.sh` is a PreToolUse hook on Bash with two behaviors. **Test runners** (vitest, jest, cargo test, go test, pytest) are always throttled with agent-count-adaptive concurrency limits: `cap = floor((cores - 2) / agents)`, min 1. **Build tools, linters, and type checkers** (tsc, eslint, cargo build, go vet, etc.) are blocked when user opts in via `/coolant`. Alert labels use "throttled" (test runners, automatic) and "blocked" (build tools, opt-in) — JSONL event names remain `gate.cap`/`gate.suppress` internally. Agent count is reconciled against JSONL event log (`_reconcile_counter` in common.sh) to prevent stale counters from orphaned agents. Handles wrappers (`npx`, `env`, `command`, path prefixes). See `docs/gate-system-report.md`.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao`, `ioreg` for sensors. No third-party tools.

### Go (API gotchas)

- **bubbletea v2** Elm architecture: `Init` → `Update(msg)` → `View() tea.View`. View returns a struct with `Content`, `AltScreen`, `MouseMode` fields. Uses `tea.KeyPressMsg` (not v1's `tea.KeyMsg`). Mode 2026 synchronized output is automatic.
- **lipgloss v2** (`charm.land/lipgloss/v2`): `lipgloss.Color()` returns `color.Color` (stdlib), not a type. Map types use `color.Color` with `image/color` import.
- Each widget is its own struct in `internal/widgets/` with `SetSize()`, `Update()`, and `View() string` methods (only top-level model returns `tea.View`).
- **Collector** runs three loops: fast (150ms) for CPU/procs via cgo, slow (1s) for network + swap/vm_stat/GPU/battery concurrently via subprocesses, event tailer (500ms) for JSONL. GPU utilization via `ioreg -r -d 1 -c AGXAccelerator` piped through grep; battery via `pmset -g batt`. Shared online state protected by mutex.
- **Theme system** (`internal/theme/`): All colors flow through a `*theme.Theme` struct passed to every widget constructor. `Theme.Init()` pre-computes HCL blend LUTs (101 entries each for severity gradients). `Theme.SeverityColor()` replaces the old package-level `severityColor()`. Built-in themes registered in `theme.Registry`; resolved via `--theme` flag > `COOLANT_THEME` env > `"classic"` default. To add a theme: copy `classic.go`, change colors, register in `registry.go`.
- **Animation system** (`internal/anim/`): All motion parameters flow through an `*anim.Profile` struct passed alongside `*theme.Theme` to widget constructors. Theme controls color, profile controls motion — orthogonal axes. `Default()` reads from `config/tuning.go` (single source of truth). Built-in profiles: `default`, `calm` (slower/softer), `intense` (faster/sharper). Resolved via `--animation` flag > `COOLANT_ANIMATION` env > `"default"`. To add a profile: copy `default.go`, change values, register in `registry.go`.
- **Agent animations**: Active agents use a tidal wave (slow sine swell, `⬡→⏣→⬢` three-state glyph sweep). Stale/ghost agents use a KITT scanner (gaussian brightness peak bouncing left-to-right). `--kitt-highscore` flag (default on) makes the KITT scanner display completed agent count — stale dots dim-breathe, completed dots earn the KITT scan. Disable with `--kitt-highscore=false` or `COOLANT_KITT_HIGHSCORE=0` to revert to ghost mode. Animation patterns are universal across themes and profiles; theme controls accent color, profile controls speed/brightness/spring physics.
- `GaugeDotColor.ANSIOverride` lets Classic use 16-color ANSI escapes while other themes use truecolor. If set, `Init()` uses the override; otherwise derives truecolor from `Color`.
- Process type and category colors live in `internal/ui/colors.go` as semantic defaults. Agent glyphs (`⬡⏣⬢`) and `DimText`/`ColorText` helpers also in `ui/colors.go`. Timing, thresholds, and EMA alphas live in `internal/config/tuning.go`; animation defaults also live there but are consumed via `anim.Default()` rather than directly.
- Braille rendering done natively in Go (no awk, no subshells).
- For sparkline, gauge, and render architecture internals see `docs/go-design.md`.
- **bubblezone v2 is incompatible with `lipgloss.Canvas` / the v2 compositor.** We register clickable zones via `github.com/lrstanley/bubblezone/v2`. The maintainer warns that bubblezone may not work when using the lipgloss v2 canvas/compositor — the marker-scan model assumes a flat string. Coolant composes layouts via string concatenation in `layout/horizontal.go`; do not introduce `lipgloss.Canvas` without re-evaluating the zone story.
- **Zone ID naming convention:** `<widget>:<entity-id>` — e.g. `cat:build`, `agent:abc123`, `stat:cpu`. Avoids cross-widget collisions without `zone.NewPrefix()` overhead.
- **Width math near marks:** bubblezone markers are zero-width to `lipgloss.Width` but inflate `len()`. Wrap only the final styled output in `zone.Mark`; never compute width on a Mark-wrapped string. Prefer `lipgloss.Width` over `len` anywhere a string may be marked.

## Testing

Tests are a dev dependency — they don't ship with coolant. New scripts must have tests before merge; new behavior on existing scripts must have a failing test first (red-green-refactor).

**Bash (bats-core):** `.bats` files in `tests/`, one per script. One assertion per test, behavior-describing names (`agent-start auto-engages at threshold`). Install via `brew install bats-core`.

```bash
bats tests/                        # full suite
bats tests/toggle.bats             # single file
bats tests/ -f "reconcile"         # name pattern
```

`tests/test_helper.bash` provides `setup`/`teardown` that isolates all state to a temp directory — tests never touch real `/tmp/coolant-*` files. Tests set env vars (`COOLANT_LOCKFILE`, etc.) to point at the temp dir; scripts respect these via the defaults in `common.sh`.

**Go:** table-driven tests in `*_test.go` next to the code they test. Use `t.Helper()` on test helpers; direct assertions via `t.Errorf`/`t.Fatalf` — no test framework.

```bash
cd thermal && go test ./...        # full suite
```

## graphify

Knowledge graph at `graphify-out/`. Post-commit hook keeps it current automatically.
Read `graphify-out/GRAPH_REPORT.md` for god nodes and community structure before deep codebase questions.
