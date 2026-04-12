# Coolant Phase 1: OSS/BSL Foundation + Telemetry Hooks

**Status:** Approved 2026-04-11
**Scope:** Phase 1 of the four-phase Thermal Enterprise roadmap (see Context).

## Context

This is the first of four planned phases that extend Coolant's observability surface beyond what Claude Code's native OTEL exporter emits, toward a full prompt→session→commit→PR→merge→production attribution chain. The strategic goal is to slot into blindspots that GitHub-side SEI vendors (LinearB being the archetype) cannot reach — specifically, the pre-PR runtime surface where intent, attempts, and outcomes actually live.

The four phases, at a glance:

1. **Phase 1 (this spec):** OSS hooks, `coolant-emit` CLI, `pkg/collector` lift, `/commit` session trailer, repo reorganization for OSS/BSL split, licensing landing. Fully free-tier. Community wedge.
2. **Phase 2:** BSL `coolant-daemon` with process-aware build/test/lint detection. Imports OSS `pkg/collector`.
3. **Phase 3:** BSL `coolant-correlator` + GitHub App for SHA→PR→merge/revert attribution. Closes the prompt-to-ship loop.
4. **Phase 4:** Outcome signal integration (Sentry, PagerDuty, Datadog). "This session shipped code that was in an incident."

## Scope

**In scope for this spec:**
- Licensing landing (Apache 2.0 root, BSL 1.1 in `enterprise/` subtree)
- Repo reorganization to the target layout (single repo, two Go modules)
- `pkg/collector` public API lift from `thermal/internal/collector/`
- New `coolant-emit` OTLP+JSONL CLI
- Phase 1 bash hooks (UserPromptSubmit, PreToolUse, PostToolUse, SessionStart, PreCompact, Stop) — new and extended
- `/commit` skill session-id trailer (v1 format)
- Two Phase 1 Grafana dashboards (Session Friction, Prompt-to-Commit Funnel)
- Tests for all of the above (bats + Go)

**Out of scope (explicitly deferred):**
- `coolant-daemon` (Phase 2)
- Build/test/lint detection (Phase 2, daemon-owned)
- `coolant-correlator` + GitHub App (Phase 3)
- Outcome signal integrations (Phase 4)
- SaaS / multi-tenant concerns
- LLC formation / legal entity for BSL copyright holder
- Trademark filings
- Naming disambiguation ("Coolant" vs "Thermal" product hierarchy)

The `enterprise/` subtree is created as a stub only — no Go source yet. It exists to establish the licensing boundary and module layout.

## Licensing landing

### Root (Apache 2.0)

| File | Content |
|---|---|
| `LICENSE` | Apache License 2.0 full text |
| `NOTICE` | Required by Apache 2.0 — `Copyright 2026 Todd Shaffer` and any third-party attributions |
| `CONTRIBUTING.md` | DCO sign-off requirement (`git commit -s`), contribution flow, where enterprise changes go |
| `SECURITY.md` | Vulnerability disclosure policy, threat model for bash hooks running in user shell |
| `CODE_OF_CONDUCT.md` | Contributor Covenant 2.1 boilerplate |

### `enterprise/` subtree (BSL 1.1)

| File | Content |
|---|---|
| `enterprise/LICENSE` | BSL 1.1 template filled with parameters below |
| `enterprise/README.md` | Explains the licensing boundary, points to commercial licensing contact |

**BSL 1.1 parameters:**
- **Licensor:** Todd Shaffer (update when LLC forms)
- **Change Date:** Four years from each release's tag date
- **Change License:** Apache License, Version 2.0
- **Additional Use Grant:** "You may use the Licensed Work for internal business purposes, personal use, evaluation, and development. You may not offer the Licensed Work or a derivative work to third parties as a commercial service that substantially provides the observability, AI attribution, or engineering intelligence functionality of the Licensed Work."

### Source file headers

Every Go source file gets an SPDX identifier on the first line:

- OSS files (root and `pkg/`, `cmd/`, `internal/`): `// SPDX-License-Identifier: Apache-2.0`
- BSL files (`enterprise/`): `// SPDX-License-Identifier: BUSL-1.1`

Bash files get the same in a shell comment: `# SPDX-License-Identifier: Apache-2.0`.

## Repo reorganization (Option A: single repo, two Go modules)

```
coolant/
├── LICENSE / NOTICE / CONTRIBUTING.md / SECURITY.md / CODE_OF_CONDUCT.md
├── README.md / CLAUDE.md
├── .claude-plugin/plugin.json
├── hooks/hooks.json                    hook manifest — path refs updated
├── scripts/                            existing common.sh + existing hook scripts
│   ├── common.sh / toggle.sh / preflight.sh / gate.sh
│   ├── agent-start.sh / agent-stop.sh
│   ├── prompt-submit.sh                NEW
│   ├── tool-post.sh                    NEW
│   ├── compact.sh                      NEW
│   └── session-end.sh                  NEW
├── tests/                              bats suite, new .bats per new hook
├── skills/coolant/                     SKILL.md
├── cmd/
│   ├── thermo/                         ← thermal/cmd/thermal/
│   ├── brailletext/                    ← thermal/cmd/brailletext/
│   ├── swatch/                         ← thermal/cmd/swatch/
│   └── coolant-emit/                   NEW
├── pkg/
│   └── collector/                      ← thermal/internal/collector/, PUBLIC API
├── internal/                           ← thermal/internal/ minus collector
│   ├── anim/ / theme/ / widgets/ / layout/ / config/ / ui/ / model/ / demo/
├── dashboards/                         ← dev/otel/dashboards/, product artifacts
├── dev/otel/                           local Prom+Grafana dev stack (minus dashboards)
├── docs/
│   └── superpowers/specs/              home for this spec and future ones
├── install.sh
├── go.mod                              module: github.com/toddwshaffer/coolant
└── enterprise/
    ├── LICENSE / README.md
    └── go.mod                          separate module, stub only
```

### Decisions embedded in this layout

- **`scripts/` keeps its name.** Renaming to `hooks/` churns for little gain; `hooks/hooks.json` already disambiguates the manifest from the implementations.
- **`dashboards/` moves out of `dev/otel/`.** They're product artifacts, not dev plumbing. Grafana provisioning YAML in `dev/otel/provisioning/` updates to point at the new path.
- **`thermal/` directory is fully removed.** Binary stays named `thermo`.
- **Module path:** `github.com/toddwshaffer/coolant`. Confirm before first tag.
- **Separate Go modules** physically prevent the OSS tree from importing BSL code — Go's module boundary is the enforcement, not a convention to remember. The enterprise module imports OSS via module path; local dev uses a `replace ../` directive.

## `pkg/collector` public API

All existing files lifted verbatim from `thermal/internal/collector/`. The public surface that external consumers (third-party tools, the future daemon) would import:

```go
package collector

type Snapshot struct { ... }
type SystemStats struct { ... }
type ProcessInfo struct { ... }
type Category int

type Options struct {
    FastInterval time.Duration  // default 150ms
    SlowInterval time.Duration  // default 1s
    EventsPath   string          // default $TMPDIR/coolant-$USER.events.jsonl
}

func New(opts Options) *Collector
func (*Collector) Start(ctx context.Context) error
func (*Collector) Snapshot() Snapshot
func (*Collector) Events() <-chan Event
```

### Platform coverage

- `cpu_darwin.go` — existing mach `host_statistics` cgo implementation, unchanged
- `cpu_linux.go` — **NEW stub** returning zero CPU stats with a `TODO` comment, so the library cross-compiles today. Real Linux implementation lands with the Phase 2 daemon.

### Versioning

Tag `v0.1.0` on first release. Pre-1.0, the API may change — this is documented in `pkg/collector/doc.go`.

## `coolant-emit` CLI

### Purpose

A tiny single-invocation Go binary that bash hooks call to emit one OTLP metric and append one line to the shared JSONL event log.

### Usage

```
coolant-emit counter   <metric_name> [key=value ...]
coolant-emit histogram <metric_name> <value> [key=value ...]
coolant-emit gauge     <metric_name> <value> [key=value ...]
```

### Default labels (auto-injected from environment)

- `repo` — from `OTEL_RESOURCE_ATTRIBUTES` (populated by the `cclaude` shell helper)
- `session_id` — from `CLAUDE_SESSION_ID` if Claude Code exports it (confirm env name before landing; if absent, omit)
- `user_email` — from `git config user.email`, silently omitted if unavailable

### Transport

OTLP/HTTP JSON to the endpoint in `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` — same endpoint Claude Code already pushes to. Prometheus 3.x accepts it natively; no collector required.

### JSONL side effect

Every invocation also appends one line to `$COOLANT_EVENTS` (defaults to the existing `$TMPDIR/coolant-$USER.events.jsonl`). Schema matches what `coolant_event` in `common.sh` already produces, so `thermal/bin/thermo` reads it unchanged.

### Error handling

- **JSONL write happens first, always.** If it fails, exit 2. Thermal TUI keeps working even if OTLP endpoint is down.
- **OTLP push has a 500ms timeout.** A slow endpoint never becomes a slow hook, never a slow Claude session.
- **OTLP failures logged only with `--verbose`.** Default exit code is 0 regardless of OTLP result, so hooks don't block Claude Code on telemetry.

### Non-functional budget

- Cold start under 50ms (hooks run often; Go binary startup is the dominant cost)
- Binary size under 10MB
- Zero-config — env and flags only
- Multi-arch builds (darwin-arm64, darwin-amd64, linux-amd64). The current `thermo` release process is manual `GOARCH=...` invocations documented in `CLAUDE.md`; this spec extends that process to also build `coolant-emit` for the same three targets. A proper release-automation pass (goreleaser or equivalent) is a separate followup.

## Phase 1 bash hooks

| Script | Trigger | Metrics emitted |
|---|---|---|
| `prompt-submit.sh` (NEW) | UserPromptSubmit | `coolant_prompt_total` counter; `coolant_prompt_length_chars` histogram |
| `preflight.sh` (EXTEND) | SessionStart | `coolant_session_start_total{branch_state=clean\|dirty}`; caches HEAD SHA to `$TMPDIR/coolant-$USER.session-${SESSION_ID}.head` |
| `gate.sh` (EXTEND) | PreToolUse | `coolant_tool_invocation_total{tool_name}`; existing test cap + build suppression behavior preserved |
| `tool-post.sh` (NEW) | PostToolUse | `coolant_tool_error_total{tool_name,exit_code}` when exit code ≠ 0 |
| `compact.sh` (NEW) | PreCompact | `coolant_context_compaction_total` |
| `session-end.sh` (NEW) | Stop | Diffs HEAD vs cached session-start HEAD → `coolant_session_outcome_total{outcome=committed\|no_commit}`, `coolant_session_commits_total`, `coolant_session_lines_in_commits` histogram. Writes `{session_id, [shas]}` event to JSONL. Cleans up HEAD cache file. |
| `agent-start.sh` / `agent-stop.sh` (EXTEND) | SubagentStart / SubagentStop | Preserves existing counter behavior; adds `coolant_subagent_active_gauge` |

### Error hygiene

Every `coolant-emit` invocation in a hook is wrapped `|| true`. Telemetry never blocks Claude Code. Existing hook behaviors (test capping, build suppression, counter reconciliation) are untouched.

## `/commit` skill session trailer (v1 format)

The `/commit` skill appends a block of Git trailers to every commit message:

```
Coolant-Session-V1: 7c9c485c-345b-4822-b973-b46f391b2696
Coolant-Cost-USD: 0.4231
Coolant-Tokens-Input: 12450
Coolant-Tokens-Output: 3204
Coolant-Tokens-CacheRead: 88300
Coolant-Tokens-CacheCreation: 2100
```

### Rules

- **Schema versioning via `-V1` suffix.** Future breaking changes add `Coolant-Session-V2:`; both remain readable during migration.
- **Missing values → omit the line.** On a Claude Max plan, cost may be absent; never emit an empty value.
- **Only via `/commit` skill.** Not an automatic git hook. Rationale: keeps control with the user, avoids amending (which rewrites SHAs the user may already have seen), keeps Phase 1 scope tight. Non-`/commit` workflows rely on the Phase 3 correlator attributing via the JSONL `{session_id, [shas]}` mapping.

### Data source

The skill queries the Prometheus OTLP endpoint at commit time, aggregating the session's cost and token totals directly:

```
sum(claude_code_cost_usage_USD_total{session_id="<SID>"})
sum by (type) (claude_code_token_usage_tokens_total{session_id="<SID>"})
```

This keeps `coolant-emit` stateless and avoids a second source of truth. If Prometheus is unreachable, the skill omits the cost/token trailer lines but still emits `Coolant-Session-V1:` with the session UUID — attribution survives even when metrics do not.

## Phase 1 Grafana dashboards

Two JSONs land in `dashboards/`, auto-provisioned via `dev/otel/provisioning/dashboards/default.yml`. Both follow the style of existing dashboards: `schemaVersion 39`, `datasource: { type: "prometheus" }` (no uid), descriptions on every panel.

### `claude-friction.json` — Session Friction (Theme C)

| Panel | Metric / PromQL shape |
|---|---|
| Context compaction rate by repo | `sort_desc(sum by (repo) (rate(coolant_context_compaction_total[$__range])))` — bargauge, high = bad |
| Tool error rate by tool × repo | `sort_desc(sum by (tool_name, repo) (rate(coolant_tool_error_total[$__range])) / sum by (tool_name, repo) (rate(coolant_tool_invocation_total[$__range])))` — table w/ gradient |
| Think-to-edit ratio by repo | `sum by (repo) (claude_code_token_usage_tokens_total{type="input"}) / sum by (repo) (claude_code_lines_of_code_count_total{type="added"})` — bargauge |
| Session commit conversion | `sum(rate(coolant_session_outcome_total{outcome="committed"}[$__range])) / sum(rate(coolant_session_outcome_total[$__range]))` — stat + 30d trend |
| Prompt count per session | Histogram from `coolant_prompt_total` aggregated per session_id |
| Subagent concurrency over time | `max_over_time(coolant_subagent_active_gauge[1m])` — timeseries |

### `claude-funnel.json` — Prompt-to-Commit Funnel (Theme A partial)

| Panel | Metric / PromQL shape |
|---|---|
| Funnel: prompts → sessions → commits | Three stat panels with conversion %, computed from `coolant_prompt_total`, `claude_code_session_count_total`, `coolant_session_commits_total` |
| Prompt length distribution | `coolant_prompt_length_chars` histogram — heatmap or histogram chart |
| Session outcome breakdown | `sum by (outcome) (coolant_session_outcome_total)` — piechart |
| Lines committed per session | `coolant_session_lines_in_commits` histogram |
| Repos idle >7 days | `count(count by (repo) (coolant_session_start_total) unless count by (repo) (coolant_session_start_total offset 7d))` — stat, flags dead repos |

## Testing strategy

TDD throughout — red, then green, then refactor. No implementation commits without tests.

| Component | Test type | Coverage |
|---|---|---|
| `pkg/collector` | Go, direct assertions (existing style — no framework) | Existing tests carry over; add `Options` validation and `New` idempotency |
| `cmd/coolant-emit` | Go, table-driven with `*testing.T` | Arg parsing (counter/histogram/gauge variants, key=value label parsing, malformed inputs); default label injection from env; OTLP payload shape via mock HTTP server; JSONL append correctness including parallel-invocation safety; graceful behavior when OTLP endpoint is unreachable |
| New bash hooks | bats, temp-dir isolated via existing `test_helper.bash` | One `.bats` per new hook; one assertion per observable behavior; behavior-describing test names |
| `/commit` trailer | bats | Trailer presence, format compliance, absent-field handling, schema version string |
| Grafana dashboards | Manual verification | Each panel's PromQL queried against live Prometheus to confirm it returns sensible shape |

## Rollout sequence

Seven commits, each independently testable and revertable. All use the `/commit` skill with the full session trailer.

1. **Licensing + repo reorg.** Add LICENSE/NOTICE/CONTRIBUTING/SECURITY/CODE_OF_CONDUCT at root. Create `enterprise/` stub with BSL LICENSE and README. Lift `thermal/` contents to new layout (`cmd/thermo`, `pkg/collector`, `internal/*`). Update `go.mod` module path. Update `install.sh`, `.claude-plugin/plugin.json`, Grafana provisioning paths, `CLAUDE.md`. Verify `go test ./...`, `go build ./...`, `bats tests/` all green. Single commit.
2. **`pkg/collector` lift finalization.** SPDX headers on each file. Add `cpu_linux.go` stub. Update importers (only `cmd/thermo`). Tag `pkg/collector/v0.1.0`. Commit.
3. **`coolant-emit` CLI.** TDD the implementation. Wire into `install.sh`. Add to multi-arch release builds. Commit.
4. **Phase 1 hooks.** TDD each hook script. Wire into `hooks/hooks.json`. Commit.
5. **`/commit` trailer.** Extend the skill. Bats test. Commit.
6. **Phase 1 dashboards.** Land the two JSON files. Verify queries against live Prometheus. Commit.
7. **Docs sweep.** Update `CLAUDE.md` for the new layout, new binaries, and licensing notes. Add a `LICENSING.md` summary. Commit.

## Open questions and deferrals

- **Module path (`github.com/toddwshaffer/coolant`)** — confirm before first `go.mod` lands.
- **Claude Code session_id env var name** — verify live before `session-end.sh` ships. If Claude does not export one, session-end hook falls back to deriving a deterministic ID from process metadata, or we skip session-scoped metrics in v1 and defer until we can tag them reliably.
- **Legal entity** — BSL copyright holder remains Todd Shaffer personally until LLC formation. Noted; not blocking this spec.
- **Contributor agreement** — this spec picks **DCO** (lightweight, enforced via GitHub's native sign-off check). A true CLA can be adopted later if corporate contribution demand justifies the friction.
- **Trademark filings for "Coolant" and "Thermal"** — separate work stream, flagged.
- **Product name hierarchy disambiguation** — "Coolant" (plugin) vs "Thermal" (dashboard) vs "Thermal Enterprise" (paid product) should be resolved before first public marketing push. Not blocking Phase 1 ship.
