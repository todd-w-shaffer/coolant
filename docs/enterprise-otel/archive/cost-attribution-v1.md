# Cost Attribution: What We Have, What We Need

**Status:** Active research — Perplexity deep-dive pending on API/billing
surfaces across all Claude deployment models.

---

## Customer profiles

Enterprise customers fall into two distinct billing topologies. Thermal
must serve both. The token/cost data available to each may differ
fundamentally.

### Profile A: Claude for Enterprise

Managed org, SSO, admin console, usage policies. Fortune 500 platform
teams, 200+ developers. Centralized billing with (potentially) admin APIs
that expose per-user or per-workspace usage breakdowns.

**Why they care:** Cost allocation across teams/projects. "Engineering
spent $47K on Claude this quarter — how much was Platform vs. Product?"

**Likely data access:** Org-admin APIs, possibly richer telemetry than
individual plans. May have access to per-request token counts via admin
console or programmatic API.

### Profile B: Stacked Max/Pro plans

Individual seats, no centralized admin. 5-50 person startups where each
developer has their own subscription.

**Why they care:** Same cost visibility desire, no centralized billing
surface. Each developer is a billing island.

**Likely data access:** Individual plan usage (if exposed at all).
"Unlimited" plans may deliberately obscure token counts — no billing
incentive to surface them. But the team still wants relative consumption
ranking.

**Key tension:** Profile B may never get absolute token counts from
Anthropic. Thermal's proxy metrics (agent-hours, relative ranking) may be
the *only* fleet-level cost signal these customers ever get. That makes
the proxy metrics a product, not a stopgap.

---

## What we can ship today (no external dependencies)

These metrics derive entirely from data Thermal already collects via the
JSONL event bus and OS-level observation.

### Agent-hours

`SubagentStop.timestamp - SubagentStart.timestamp`, per agent, summed and
attributed by session, machine, team (from config labels).

- **Enterprise value:** "Team A burned 340 agent-hours this week, Team B
  burned 40." Rankable. Actionable without knowing token prices.
- **Grafana panel:** Bar chart by team, stacked by project, week-over-week.
- **Metric:** `thermal.agents.duration_seconds` (Float64Counter, summed)
  with `team`, `project` attributes from config labels.

### Gate suppression ROI

Already tracked as monotonic counters. The missing piece is translating
count into time/cost saved.

- **Approach:** Configurable average-duration-per-tool in config. Default
  estimates shipped (e.g., tsc ~8s, eslint ~3s, vitest ~12s). Customer
  overrides for their actual build times.
- **Enterprise value:** "Thermal prevented 847 redundant tsc invocations
  this week, saving approximately 12 hours of CPU time." This is machine
  cost, not API cost, but it's concrete and defensible.
- **Grafana panel:** Stat panel (hero number), time series by tool type.
- **Metrics:**
  - `thermal.gate.time_saved_seconds` (Float64Counter) with `tool` attribute
  - Derived from existing `thermal.gate.suppressions` × configured duration

### Relative consumption ranking

Cross-machine comparison without absolute dollar amounts.

- **Signals:** Agent-hours, agent spawn count, gate suppression count,
  average threat level, peak memory utilization, time-in-MELTDOWN.
- **Enterprise value:** "Which teams are burning the most?" is answerable
  from relative ranking alone. The CTO doesn't need exact dollars to
  reallocate headcount or adjust workflows.
- **Grafana panel:** Table with per-team/per-developer composite score,
  sortable by any column.

### Resource cost correlation

Agent activity × system resource impact = infrastructure cost signal.

- **Approach:** Correlate agent-hours with CPU/MEM utilization during those
  hours. High agent-hours + high resource utilization = high infrastructure
  impact.
- **Enterprise value:** "This team's Claude usage correlates with 94%
  memory utilization across their machines."
- **Metric:** No new metric needed — correlation computed in Grafana from
  existing `thermal.agents.duration_seconds` and `thermal.cpu.utilization`.

---

## What's blocked on external data sources

These require token counts that Thermal cannot observe today.

### Absolute cost attribution

Per-agent, per-session, per-team dollar amounts. Requires:
- Input/output/cache token counts per API request
- Token pricing (varies by model, changes over time)

### Cache efficiency

Cache hit ratio as a team-health metric. Requires:
- `cache_read_input_tokens` vs. `input_tokens` per request
- "A team at 90% cache hits is using Claude well; 40% is burning money
  on cold starts" (from sales engineer findings)

### Reserved metric names (forward-compatible)

These names are reserved in the metric catalog. Dashboards can reference
them now and light up when the data source exists.

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.tokens.input` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.output` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.cache_read` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.cache_creation` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.cost.usd` | Float64Counter | dollars | `agent_id`, `session_id` |

---

## Resolution paths (pending Perplexity research)

Investigating all possible token data sources across deployment surfaces:

1. **Claude API response fields** — token counts in response body/headers
2. **Claude Code local telemetry** — session logs, usage files on disk
3. **Claude Code hooks** — do any hook types receive usage data on stdin?
4. **Claude for Enterprise admin API** — programmatic usage breakdowns
5. **Bedrock CloudWatch / Vertex Cloud Monitoring** — per-invocation metrics
6. **Anthropic Foundry** — OTEL or Prometheus telemetry

Key question for Profile B (Max/Pro plans): does "unlimited" mean token
counts are simply not exposed? If so, Thermal's proxy metrics are the
*only* cost signal these customers get — permanently, not temporarily.

---

## Strategic framing

The spec should present cost attribution as two tiers:

**Cost signal (ships with OTEL v1):** Agent-hours, gate ROI, relative
ranking, resource correlation. Derived from data we own. Valuable to both
customer profiles regardless of whether token data ever materializes.

**Cost attribution (ships when data source exists):** Absolute token
counts and dollar amounts. Dependent on external data sources. May only
be available to Profile A (Enterprise) customers. Reserved metric names
ensure forward compatibility.

The gate suppression ROI number ("Thermal saved your org 12 hours of CPU
time this week") may be the single most compelling enterprise metric
regardless of token availability. It's concrete, defensible, and unique
to Thermal. It should be the headline, not a footnote.
