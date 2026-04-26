# Coolant

A resource management layer for Claude Code — prevents machines from melting when parallel agents run unthrottled.

## ABSOLUTE RULES

**NEVER delete files or directories without explicit permission.** No exceptions. No "cleanup." No "safe to delete." ASK FIRST. Always. Even if the file looks stale, orphaned, or unnecessary. Even if you created it. Even if it's untracked. The user runs with permissive settings — that trust must not be abused for destructive operations.

## Subtree CLAUDE.md (lazy-loaded)

Subtree CLAUDE.md files load only when files inside them are accessed. Read these proactively when answering questions in their domain:

- **`thermal/CLAUDE.md`** — Go thermal dashboard: deps, internal package groups, full Go API gotchas (bubbletea/lipgloss/theme/anim/widgets/stats/bubblezone), terminal-rendering gotchas, preview tools.
- **`scripts/CLAUDE.md`** — Bash conventions, common.sh helpers, lock semantics, state files, gate system + terminology mapping.
- **`dev/otel/CLAUDE.md`** — Local Prometheus+Grafana dogfood stack and verified CC metric names.

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

## Surfaces

Coolant has two distinct surfaces with non-overlapping scopes:

- **`thermal/` (thermo)** — the **system-wide** knowledge surface. Every machine-level signal (battery, GPU, swap, network, overall thermal, agent activity) lives here. Bottom tmux strip; one instance per machine.
- **`claude-statusline/`** — the **per-session** surface. Token usage, model name, update glyph. Rendered in every session chrome.

System-wide signals belong only in thermo — do not propose adding them to statusline even when a cell would technically fit. Statusline crowding duplicates information thermo already shows and makes the per-session surface noisy.

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

A second hook pair (`.claude/hooks/{audit-review-agents,enforce-spec-to-ship-reviews}.sh`) gates `git commit` on whether `/simplify` and `/observations` ran. Substantive commits (>`$COOLANT_GATE_THRESHOLD_LINES` lines, default 200) require ≥3 distinct simplify kinds + ≥2 distinct observations kinds in the per-session audit log. Override with `[skip-review]` trailer in the commit body. Tunables in the hook headers.

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

Bug fixes follow the same cycle: write a test that reproduces the bug (red), fix it (green), clean up (refactor).

## Project structure

Two layers: **bash** for hooks, plumbing, and data collection; **Go** for visualization. The bash↔Go seam is a JSONL event log (see "JSONL event bus" below).

**Repo-level layout:**

- `VERSION` — root-level semver stamp, updated by auto-release workflow.
- `hooks/`, `scripts/` — bash plumbing (see `scripts/CLAUDE.md`).
- `thermal/` — Go thermal dashboard (see `thermal/CLAUDE.md`).
- `.claude-plugin/`, `install.sh`, `claude-statusline/` — plugin manifest, installer, and the braille statusline.
- `docs/` — design docs, theming plans, backlog specs.
- `dev/otel/` — local Prometheus+Grafana dogfood stack (see `dev/otel/CLAUDE.md`).
- `tests/` — bats tests for the bash layer.
- `skills/coolant/SKILL.md` — the `/coolant` skill entry.
- `assets/` — VHS tapes + demo GIFs.
- `.githooks/` — pre-push commit-classification hook + blocklist/allowlist data files (activated by `scripts/install-hooks.sh`).
- `.claude/hooks/classify-staged.sh` — companion Claude PreToolUse hook.

For the full package inventory, call graph, and community structure, read `graphify-out/GRAPH_REPORT.md` — regenerated by a post-commit hook, always current.

### Archive folders (gitignored — do not read, reference, or treat as current)

Historical artifacts kept for nostalgia. May be stale or broken.

- `docs/archive/` — superseded specs and explorations.
- `scripts/archive/` — old bash TUI (monitor.sh, sparkline.sh, agents.sh), replaced by Go thermal.
- `tests/archive/` — tests for archived scripts.
- `thermal/cmd/archive/` — one-off Go experiments (breathe, comets, sparkdebug).
- `bin/archive/` — compiled binaries from archived experiments.

## JSONL event bus (the bash↔Go seam)

Bash hooks write to `$TMPDIR/coolant-$USER.events.jsonl`. Go's event tailer (`collector/events.go`) polls this file at 500ms. This is the *only* runtime data path between the two layers — there are zero direct calls. The JSONL schema is defined by `coolant_event` in `common.sh` and parsed by `TailEvents` in `events.go`.

Every line carries `"schema":1` (envelope shape contract — bumps on rename or removal, additive optional fields stay at 1). Writes are serialized via the non-reentrant `coolant_lock` mkdir-mutex so parallel hooks emitting >PIPE_BUF payloads can't splice. Lock-acquisition timeout falls through to an unsynchronized write and bumps `$COOLANT_DEGRADED_COUNT` (out-of-band counter, truncated each SessionStart). The Go schema gate at `internal/stats` filters pre-versioning events at parse time — old and new envelopes coexist in the same JSONL ("virtual chop").

## Distribution

Plugin and dashboard ship separately. The plugin installs via Claude Code's marketplace system; the dashboard is a prebuilt binary on GitHub Releases.

- **Marketplace:** `todd-w-shaffer/marketplace` repo hosts the plugin manifest. Coolant is a git submodule under `plugins/coolant`. A GitHub Action (`.github/workflows/notify-marketplace.yml`) fires `repository_dispatch` on every push to main, triggering the marketplace repo to auto-update the submodule.
- **Binaries:** `thermo-darwin-arm64` and `thermo-darwin-amd64` attached to GitHub Releases. Cut automatically by `.github/workflows/auto-release.yml` on every push to `main` that touches `thermal/**`, `scripts/**`, or `hooks/**` — bumps minor semver (seeded at v0.4.0), cross-compiles both arches with `CGO_ENABLED=1` on macos-latest, and runs `gh release create --target $SHA`. Bash-only commits cut releases with binaries identical to the previous release; that's a noisy version bump but architecturally honest, since coolant's runtime spans both layers. Manual major bumps: push a `vX.Y.Z` tag to trigger `release.yml`. Local build: `CGO_ENABLED=1 GOARCH=arm64 go build -o bin/thermo-darwin-arm64 ./cmd/thermal/ && CGO_ENABLED=1 GOARCH=amd64 go build -o bin/thermo-darwin-amd64 ./cmd/thermal/` (from `thermal/`).
- **Install script:** `install.sh` downloads the binary for the user's arch, asks where to put it, and optionally installs the braille statusline to `~/.claude/` with settings.json patching. `install.sh --upgrade` delegates to `scripts/upgrade.sh` for silent re-fetch of both artifacts.
- **Staleness detection:** Daily TTL check against `VERSION` on main, cached at `$TMPDIR/coolant-$USER.latest-version` (shared between Go updater and bash statusline). Thermo shows "update available" in the notification banner; statusline appends a dim yellow ⬆ glyph. `[updates]` TOML section controls TTL and opt-out.

## Testing

Tests are a dev dependency — they don't ship with coolant. New scripts must have tests before merge; new behavior on existing scripts must have a failing test first (red-green-refactor).

- Bash (bats-core): see `scripts/CLAUDE.md`.
- Go: see `thermal/CLAUDE.md`.

## graphify

Knowledge graph at `graphify-out/`. Post-commit hook keeps it current automatically. Read `graphify-out/GRAPH_REPORT.md` for god nodes and community structure before deep codebase questions.
