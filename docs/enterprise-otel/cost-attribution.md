# Cost Attribution: Data Landscape & Strategy

**Status:** Resolved. Perplexity research complete. Token data is
available today via Claude Code's native OTEL and local session JSONL.
The "blocked on external data source" framing from v1 is obsolete.

Previous version archived at `archive/cost-attribution-v1.md`.

---

## Customer profiles

### Profile A: Claude for Enterprise

Managed org, SSO, admin console, 200+ developers. Centralized billing.

**Data access:** Claude Code OTEL (all token fields, `cost_usd`, model,
user identity), Claude Code Analytics Admin API (daily per-user model
breakdowns with estimated_cost), Anthropic Usage & Cost Admin API
(bucketed org-level data).

**Why they care:** "Engineering spent $47K on Claude this quarter — how
much was Platform vs. Product?"

### Profile B: Stacked Max/Pro plans

Individual seats, no centralized admin. 5-50 person startups.

**Data access:** Claude Code OTEL (identical fields to Enterprise — all
plan types emit the same telemetry), local session JSONL. No Admin API.
No centralized billing surface.

**Why they care:** Same cost visibility desire, zero existing tools.
Anthropic billing page shows nothing useful for Max/Pro — flat rate per
seat, no usage breakdown.

**Key finding (from Perplexity research):** The original concern that
Max/Pro customers might never get token data was wrong. Claude Code OTEL
works for all plan types. The only difference is enforcement: Enterprise
admins push via MDM, Max/Pro developers opt in individually. The data is
identical.

---

## Token data sources (resolved)

Full technical reference:
[Perplexity Data Sources Report](Claude%20Code%20Observability%20%20Token%20%26%20Cost%20Data%20Sources%20(perplexity).md)

### Tier 1: Local, real-time, backend-agnostic (both profiles)

| Source | Fields | Latency | Access |
|--------|--------|---------|--------|
| Claude Code OTEL `api_request` log events | `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`, `model`, `session.id`, `user.account_id` | ~5s | OTLP push |
| Claude Code OTEL `token.usage` metric | Same token types + model | ~60s | OTLP push |
| Local session JSONL (`~/.claude/projects/`) | `usage` object with all four token types | Real-time (tail) | Filesystem |
| Hooks (`Stop` event + transcript parse) | Same as JSONL | End-of-turn | Hook script |

Enable with: `CLAUDE_CODE_ENABLE_TELEMETRY=1` + `OTEL_LOGS_EXPORTER=otlp`

### Tier 2: Cloud-provider-specific (depends on backend)

| Provider | Per-request tokens | Per-developer attribution | Cache tokens |
|----------|-------------------|--------------------------|-------------|
| Bedrock | Yes (response + CloudWatch) | Yes (IAM identity via Invocation Logging) | Yes |
| Vertex AI | Yes (response) | No native metric support | Limited |
| Azure AI Foundry | Yes (response + Azure Monitor) | Requires APIM pattern | No `cache_creation` in Monitor |

**Critical:** Claude Code OTEL emits token data regardless of backend.
Tier 2 sources are for reconciliation, not primary data.

### Tier 3: Admin APIs (Enterprise only)

| API | Granularity | Latency | Covers Bedrock? |
|-----|-------------|---------|-----------------|
| Claude Code Analytics Admin API | Daily per-user, per-model | ~1 hour | No |
| Anthropic Usage & Cost Admin API | 1m/1h/1d buckets, org-level | ~5 min | No |
| Enterprise Analytics API | Daily per-user engagement (no tokens) | 3 days | No |

---

## Thermal's cost strategy

### Authority boundary

Claude Code is authoritative on tokens and cost. Thermal does not
duplicate these metrics. Instead:

- **Claude Code OTEL** provides: `claude_code.token.usage`,
  `claude_code.cost.usage` — per-request, per-model, per-user
- **Thermal OTEL** provides: system context, gate intelligence, agent
  lifecycle — the enrichment layer

Grafana joins them via `session.id`. The combined query tells a story
neither can tell alone: "This team spent $4,200 on Claude this week.
60% happened during HOT/MELTDOWN states. Thermal prevented $800 in
redundant builds."

### What Thermal uniquely contributes

| Metric | What it tells you | Why only Thermal has it |
|--------|-------------------|----------------------|
| `thermal.agents.duration_seconds` | Agent-hours per team/project | Requires OS-level process observation |
| `thermal.gate.time_saved_seconds` | CPU hours prevented by gate suppression | Requires gate hook intelligence |
| `thermal.gate.suppressions` | Count of prevented builds/tests | Unique to gate system |
| `thermal.threat.level` | Machine health during Claude usage | Requires system metric fusion |
| System metrics correlated with cost | "Was the machine healthy when this $ was spent?" | Requires both data streams |

### The sales sequence

1. **"Here's what your team spent."** (Claude Code OTEL — table stakes,
   gets CFO attention)
2. **"Here's why they spent it."** (Thermal enrichment — agent counts,
   threat levels, resource correlation. What Claude Code can't tell you.)
3. **"Here's what Thermal prevented them from spending."** (Gate ROI —
   the unique value, the moat, the closer)

### Gate suppression ROI: the headline metric

"Thermal prevented 847 redundant tsc invocations this week, saving
approximately 12 hours of CPU time."

This is the single most compelling enterprise metric because:
- It's unique to Thermal (nobody else can produce it)
- It's concrete and defensible (count × avg duration)
- It answers "what does Thermal save us?" directly
- Converted to dollars via `cpu_hour_rate`, it justifies the license

### Cache efficiency (enterprise Grafana panel)

`cache_read_tokens / (cache_read_tokens + input_tokens)` per developer.
Derived from Claude Code OTEL, computed in Grafana.

"A team at 90% cache hits is using Claude well. A team at 40% is burning
money on cold starts." This is a coaching metric — the platform team
identifies inefficient Claude usage and helps developers improve.

---

## Cost display in the statusline (free tier)

The statusline reads local session JSONL. No OTEL, no daemon, no
enterprise code. Published rates ship with the binary, updated each
release. Enterprise negotiated rates are a separate config path
(MDM-pushed, not in the statusline's scope).

```
▂▄▆▃▅ ctx  ▃▅▇▅▆ ses  $1.47
```

- Default: token counts (not dollars)
- Dollar display: `cost_display = "dollars"` opt-in
- Redaction: `cost_display = "redacted"` for screen-sharing
- Rate config: `[cost]` block in config.toml with published defaults

---

## Three visualization surfaces

| Surface | Who | Data source | What it shows |
|---------|-----|-------------|---------------|
| **Statusline** | Every dev | Session JSONL (local) | Per-session cost, ambient |
| **Thermo TUI** | Opt-in devs | Daemon (enriched) or standalone | Full machine picture |
| **Grafana / Hosted dashboard** | Platform team | Both OTEL streams | Fleet cost + context + ROI |

"Statusline is the speedometer. Thermo is the engine diagnostic. Grafana
is fleet management."
