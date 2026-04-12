# Phase 1c — Presentation: Dashboards & Docs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) or superpowers:subagent-driven-development (two parallel subagents, one per dashboard). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Ship two Phase 1 Grafana dashboards surfacing the metrics now being emitted by hooks, update `CLAUDE.md` for the new repo layout, and add a concise `LICENSING.md` that explains the OSS/BSL split.

**Architecture:** Both dashboards are JSON files in `dashboards/`, auto-provisioned by the existing Grafana dev stack. All PromQL is verified against a live Prometheus before committing — no fiction.

**Tech Stack:** Grafana 11 JSON schema (v39), PromQL, Markdown.

---

## Parallel guide

Tasks C1 and C2 touch independent JSON files and can run as parallel subagents. Task C3 (docs) can start in parallel with either.

| Task | Writes | Reads |
|---|---|---|
| C1 | `dashboards/claude-friction.json` | Prometheus at `http://localhost:9090` |
| C2 | `dashboards/claude-funnel.json` | Prometheus at `http://localhost:9090` |
| C3 | `CLAUDE.md`, `LICENSING.md` | — |

---

## Task C1 — Session Friction dashboard

**Files:**
- Create: `dashboards/claude-friction.json`

Surfaces the metrics that tell you *when Claude is struggling*: context compaction rate, tool error rate, think-to-edit ratio, session commit conversion, prompt count distribution, subagent concurrency.

### Step 1: Verify all source metrics exist in Prometheus

Run each check (after Phase 1b hooks have been emitting for at least one session):
```bash
for m in coolant_context_compaction_total coolant_tool_error_total coolant_tool_invocation_total coolant_prompt_total coolant_session_outcome_total coolant_subagent_active_gauge; do
  echo "=== $m ==="
  curl -s "http://localhost:9090/api/v1/query?query=$m" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d.get("data",{}).get("result",[])), "series")'
done
```
Expected: each prints `N series` with N ≥ 1. If any shows `0 series`, the corresponding hook has not yet emitted — run a few Claude Code sessions and re-check before proceeding.

Also verify the Claude-native metrics used in the think-to-edit ratio panel:
```bash
curl -s 'http://localhost:9090/api/v1/query?query=claude_code_token_usage_tokens_total' | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["data"]["result"]), "series")'
curl -s 'http://localhost:9090/api/v1/query?query=claude_code_lines_of_code_count_total' | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["data"]["result"]), "series")'
```

### Step 2: Create `dashboards/claude-friction.json`

The full JSON follows the existing dashboard style (schemaVersion 39, `datasource: {"type": "prometheus"}` with no uid, descriptions on every panel). Match the structure of `dashboards/claude-techdebt.json` as a template — copy that file and transform panel by panel.

Specifically, the dashboard must contain six panels with these PromQL queries (verified against live Prometheus in Step 3 before commit):

| Panel | Type | PromQL |
|---|---|---|
| Context compaction rate by repo (high = bad) | bargauge | `sort_desc(sum by (repo) (increase(coolant_context_compaction_total[$__range])))` |
| Tool error rate by tool × repo | table | `sort_desc(sum by (tool_name, repo) (increase(coolant_tool_error_total[$__range])) / sum by (tool_name, repo) (increase(coolant_tool_invocation_total[$__range])))` |
| Think-to-edit ratio (input tokens per line added) | bargauge | `sort_desc(sum by (repo) (claude_code_token_usage_tokens_total{type="input"}) / sum by (repo) (claude_code_lines_of_code_count_total{type="added"}))` |
| Session commit conversion | stat + 30d timeseries | `sum(increase(coolant_session_outcome_total{outcome="committed"}[$__range])) / sum(increase(coolant_session_outcome_total[$__range]))` |
| Prompt count per session | histogram / heatmap | `histogram_quantile(0.95, sum by (le) (rate(coolant_prompt_length_chars_bucket[$__range])))` (and a complementary `sum by (session_id)(coolant_prompt_total)` histogram) |
| Subagent concurrency over time | timeseries | `max_over_time(coolant_subagent_active_gauge[1m])` |

Each panel's `description` field must give a one-line explanation of what a high/low value means. Example for compaction rate: "Rate of context-window compaction events per repo. Sessions that compact are sessions that ran out of window — a friction signal. Lower is better."

Use unit types appropriately: `percentunit` for ratios, `short` for counts, `none` for ratios that aren't percentages. Use `continuous-RdYlGr` or `continuous-GrYlRd` color modes as matched to whether high is good or bad.

Dashboard metadata:
```json
{
  "title": "Claude Code — Session Friction",
  "uid": "coolant-friction",
  "tags": ["claude-code", "coolant", "friction"],
  "schemaVersion": 39,
  "refresh": "1m",
  "time": { "from": "now-7d", "to": "now" },
  "timezone": "browser"
}
```

### Step 3: Verify every panel's PromQL returns valid data

For each panel's query, substitute `$__range` with `7d` and hit Prometheus:
```bash
QUERY='sort_desc(sum by (repo) (increase(coolant_context_compaction_total[7d])))'
curl -s "http://localhost:9090/api/v1/query" --data-urlencode "query=$QUERY" | python3 -m json.tool | head -20
```
Expected: each returns `"status":"success"` and a non-empty `result` array (or at minimum, a success status and zero-length array for queries where no data has accumulated yet — that's OK for something like `session_outcome` if no sessions have ended with commits yet).

**If any query returns `"status":"error"`, fix the query before committing.** Common issues: referencing a label that doesn't exist, using `rate()` on a non-counter, forgetting `sum by ()`.

### Step 4: Reload Grafana and visually inspect

Run:
```bash
# Grafana auto-reloads dashboards every 10s by default — wait a few seconds, then:
open http://localhost:3000/d/coolant-friction
```
Expected: dashboard loads without errors. Panels may show "No data" for metrics that haven't accumulated enough samples yet — that's fine. Errors (red text, "parse error") are not fine; fix before commit.

### Step 5: Commit via `/commit` skill

---

## Task C2 — Prompt-to-Commit Funnel dashboard

**Files:**
- Create: `dashboards/claude-funnel.json`

Shows the conversion funnel from user prompts → sessions → commits, plus per-stage distributions. Complements C1: C1 is friction diagnostics, C2 is output throughput.

### Step 1: Verify source metrics

Same check as C1 Step 1. Additional metric needed:
```bash
curl -s "http://localhost:9090/api/v1/query?query=claude_code_session_count_total" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["data"]["result"]), "series")'
```

### Step 2: Create `dashboards/claude-funnel.json`

Six panels:

| Panel | Type | PromQL |
|---|---|---|
| Prompts (selected range) | stat | `sum(increase(coolant_prompt_total[$__range]))` |
| Sessions (selected range) | stat | `sum(increase(claude_code_session_count_total[$__range]))` |
| Sessions with commits | stat | `sum(increase(coolant_session_outcome_total{outcome="committed"}[$__range]))` |
| Prompt length distribution | histogram | `sum by (le) (increase(coolant_prompt_length_chars_bucket[$__range]))` |
| Session outcome breakdown | piechart | `sum by (outcome) (increase(coolant_session_outcome_total[$__range]))` |
| Lines committed per session | histogram | `sum by (le) (increase(coolant_session_lines_in_commits_bucket[$__range]))` |
| Repos idle ≥7 days | table (bonus) | `count by (repo) (coolant_session_start_total) unless count by (repo) (coolant_session_start_total offset 7d)` |

The three funnel stat panels should use `background_solid` color mode with `continuous-GrYlRd` so that, visually, the drop-off from prompts → sessions → commits is immediately obvious. Arrange them horizontally at the top.

Dashboard metadata:
```json
{
  "title": "Claude Code — Prompt-to-Commit Funnel",
  "uid": "coolant-funnel",
  "tags": ["claude-code", "coolant", "funnel"],
  "schemaVersion": 39,
  "refresh": "1m",
  "time": { "from": "now-7d", "to": "now" },
  "timezone": "browser"
}
```

### Step 3: Verify every PromQL query live

Same pattern as C1 Step 3. Fix errors before commit.

### Step 4: Reload Grafana, visual inspect

```bash
open http://localhost:3000/d/coolant-funnel
```

### Step 5: Commit via `/commit` skill

---

## Task C3 — Docs sweep

**Files:**
- Modify: `CLAUDE.md`
- Create: `LICENSING.md`

### Step 1: Update `CLAUDE.md`

Open `CLAUDE.md`. Make these specific changes:

**A. Quick reference block** — ensure it reads:
```bash
go build -o bin/thermo ./cmd/thermo/            # build thermal dashboard
go build -o bin/coolant-emit ./cmd/coolant-emit/  # build OTLP/JSONL emitter
go test ./...                                    # Go tests (pkg/collector + cmd/*)
bats tests/                                      # bash hook tests
./bin/thermo --demo                              # thermal dashboard, synthetic data
./bin/thermo                                     # thermal dashboard, live system data
```

**B. Project structure tree** — replace the `thermal/` subtree with the new layout:
```
cmd/
  thermo/          # thermal TUI main (was thermal/cmd/thermal/)
  coolant-emit/    # stateless OTLP+JSONL emitter called by bash hooks
  brailletext/     # braille font debug tool
  swatch/          # theme palette preview
pkg/
  collector/       # process/system observation library, PUBLIC API (v0.1.0)
internal/
  anim/ theme/ widgets/ layout/ config/ ui/ model/ demo/
enterprise/        # BSL 1.1 subtree — daemon and correlator land here in Phase 2/3
dashboards/        # Grafana product dashboards (auto-provisioned)
```

**C. Add a new section right after the existing "Project structure" section:**

```markdown
## Licensing

- **Root tree** (Apache 2.0) — `cmd/`, `pkg/`, `internal/`, `scripts/`, `skills/`, `hooks/`, `tests/`, `dashboards/`, `dev/`, `docs/`. Public, contributions welcome under DCO.
- **`enterprise/` subtree** (BSL 1.1, converting to Apache 2.0 four years after each release) — daemon and correlator code. Use is permitted for internal business, personal, evaluation, and development purposes; offering the code as a competing commercial service requires a separate license. See `enterprise/LICENSE` and `LICENSING.md`.

The two trees live under separate Go modules (root `go.mod` vs `enterprise/go.mod`). OSS code cannot accidentally import BSL code — Go's module boundary is the enforcement.
```

**D. Add a new telemetry section after the existing JSONL event bus section:**

```markdown
### Phase 1 telemetry hooks

Claude Code's native OTLP push (cost, tokens, LOC) is complemented by runtime metrics emitted by bash hooks via the `coolant-emit` CLI:

- `scripts/prompt-submit.sh` — UserPromptSubmit: `coolant_prompt_total`, `coolant_prompt_length_chars`
- `scripts/preflight.sh` — SessionStart: `coolant_session_start_total{branch_state}`; caches HEAD SHA for session-end diff
- `scripts/gate.sh` — PreToolUse: `coolant_tool_invocation_total{tool_name}` (plus existing gate behavior)
- `scripts/tool-post.sh` — PostToolUse: `coolant_tool_error_total{tool_name,exit_code}` on non-zero exit
- `scripts/compact.sh` — PreCompact: `coolant_context_compaction_total`
- `scripts/session-end.sh` — Stop: `coolant_session_outcome_total{outcome}`, `coolant_session_commits_total`, `coolant_session_lines_in_commits`; writes session→SHA mapping to JSONL
- `scripts/agent-start.sh` / `scripts/agent-stop.sh` — SubagentStart/Stop: `coolant_subagent_active_gauge` (plus existing counter reconcile)

All `coolant-emit` invocations are wrapped `|| true` — telemetry never blocks Claude Code.

### Commit trailer protocol

The `/commit` skill appends a versioned trailer to every commit message it generates:

```
Coolant-Session-V1: <uuid>
Coolant-Cost-USD: <n>
Coolant-Tokens-Input: <n>
Coolant-Tokens-Output: <n>
Coolant-Tokens-CacheRead: <n>
Coolant-Tokens-CacheCreation: <n>
```

Values are aggregated at commit time by querying Prometheus for the current session's cost and token totals. Missing values are omitted (not emitted as zero). The `-V1` suffix versions the schema — future breaking changes append `Coolant-Session-V2:` without breaking existing parsers.
```

### Step 2: Write `LICENSING.md`

Create `LICENSING.md` at repo root:

```markdown
# Licensing

This repository uses a two-tier licensing model:

## Apache License 2.0 — root tree

Everything at the repository root (outside the `enterprise/` subtree) is licensed under Apache License 2.0. Full text in `LICENSE`. In practice:

- `cmd/`, `pkg/`, `internal/`, `scripts/`, `skills/`, `hooks/`, `tests/`, `dashboards/`, `dev/`, `docs/`
- The `coolant-emit` CLI, `thermo` TUI, `pkg/collector` library, all bash hooks, all Grafana dashboards, all Claude Code skills

You can use, modify, fork, redistribute, and sublicense this code for any purpose, commercial or non-commercial. Contributions are welcome and covered by the Developer Certificate of Origin (see `CONTRIBUTING.md`).

## Business Source License 1.1 — `enterprise/` subtree

Everything under `enterprise/` is licensed under the Business Source License 1.1. Full text in `enterprise/LICENSE`. The key terms:

- **Change Date:** Four years after each release, the code converts to Apache License 2.0.
- **Change License:** Apache License, Version 2.0.
- **Additional Use Grant:** You may use the Licensed Work for internal business purposes, personal use, evaluation, and development. You may not offer the Licensed Work or a derivative work to third parties as a commercial service that substantially provides the observability, AI attribution, or engineering intelligence functionality of the Licensed Work.

In plain terms: run it inside your company for whatever you want. Fork it, study it, modify it for your own use. Don't turn it into a competing SaaS without talking to us first.

If your intended use falls outside the Additional Use Grant, contact `licensing@coolant.dev` (TODO: set up forwarder) to discuss a commercial license.

## Why two tiers?

The root tree is the community tool. It's useful, valuable, and free — forever. The `enterprise/` tree is what pays for continued development of the whole thing. The BSL's 4-year conversion clock means even the commercial code becomes fully open-source eventually; it's time-delayed openness, not perpetual restriction.

## Source identification

Every source file carries an SPDX license identifier on the first line (Go) or second line (bash, after the shebang):

- Apache 2.0 files: `// SPDX-License-Identifier: Apache-2.0` or `# SPDX-License-Identifier: Apache-2.0`
- BSL 1.1 files: `// SPDX-License-Identifier: BUSL-1.1`

The identifiers make license-scanning tools work correctly on the repo.

## Third-party dependencies

All runtime dependencies are permissively licensed (MIT, BSD, Apache 2.0). No GPL or AGPL dependencies are acceptable. Audit via `go mod licenses` (or similar tooling) before adding new deps.
```

### Step 3: Verify renders

Run:
```bash
grep -c 'thermal/' CLAUDE.md
```
Expected: `0` — all `thermal/` references should now be root paths or `cmd/thermo/`.

```bash
head -20 LICENSING.md
```
Expected: shows the "Licensing" heading and first section.

### Step 4: Commit via `/commit` skill

---

## Exit criteria for Plan 1c (and Phase 1 overall)

- [ ] Both dashboards (`claude-friction.json`, `claude-funnel.json`) render in Grafana without errors
- [ ] Every panel's PromQL has been live-verified against Prometheus
- [ ] `CLAUDE.md` reflects the new repo layout, new binaries, licensing summary, and telemetry hook catalog
- [ ] `LICENSING.md` present at repo root
- [ ] No `thermal/` path references remain in docs
- [ ] `go test ./...` and `bats tests/` all green
- [ ] `bin/thermo` and `bin/coolant-emit` both build via `install.sh`
- [ ] A live Claude Code session via `cclaude` produces metrics visible on both new dashboards within one refresh cycle
- [ ] The last commit of this plan has a `Coolant-Session-V1:` trailer

When all exit criteria pass, Phase 1 is complete. Tag the release: `git tag v0.1.0` and `git tag pkg/collector/v0.1.0` (the collector tag was prepared in Plan 1a). Do not push tags yet — coordinate with the user before any remote push.
