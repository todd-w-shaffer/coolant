# Thermal Enterprise v2: Sales Engineer Findings

**Status:** Independent review — input to revised spec, not the spec itself.
**Reviewer:** Enterprise Sales Engineer panel, second pass.
**Date:** April 2026

---

## 1. The Ground Has Shifted

The original spec was written when we believed Thermal would be the sole
OTEL emitter on a developer's machine, and that token/cost data was a
future aspiration blocked on external data sources. Both assumptions are
now wrong.

**Claude Code already emits per-request token counts, cost in USD, model
attribution, session correlation, and user identity** via standard OTEL
env vars. The `claude_code.api_request` log event carries `input_tokens`,
`output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`,
`model`, and `duration_ms` at 5-second latency. The `claude_code.token.usage`
metric and `claude_code.cost.usage` counter provide the same data on a
60-second cadence. Local JSONL session files carry identical token data,
readable by any process with filesystem access.

This changes the product story fundamentally. Thermal is not the token/cost
authority. Claude Code is. Thermal's enterprise value is what it adds on
top: system state, gate intelligence, agent lifecycle, threat semantics,
and the fusion of "what Claude is doing" with "what it's doing to your
machine."

The spec acknowledges this ("Thermal is an enricher, not the authority"),
but the implications haven't been fully worked through. This document does
that.

---

## 2. The Revised Demo Flow

The old demo was a five-beat sequence ending with "the fleet export is
what you'd license." That close assumed the buyer had no fleet-level Claude
visibility yet. In 2026, many enterprise platform teams have already
enabled `CLAUDE_CODE_ENABLE_TELEMETRY=1` via MDM and are piping
`claude_code.api_request` events into their existing observability stack.
They already see token spend per developer. The old close lands flat with
these buyers.

### New demo flow: "You have the data. You're missing the context."

**Beat 1: Peripheral vision (0:00-5:00)**
Unchanged. Thermal runs in tmux. Todd advises on Claude adoption. The
braille strip glows. Nobody mentions it.

**Beat 2: The question (5:00-5:30)**
Unchanged. "What's that at the bottom?" Brief answer. Move on.

**Beat 3: The casual demo (8:00-10:00)**
Spawn three parallel agents. Dashboard lights up. Point at the statusline
cost number. "That's my session cost, real-time. I negotiated 7% off with
Anthropic, so that's my actual rate, not list price." Point at gate
suppression count. "It just killed four redundant tsc invocations. Those
would have burned CPU for nothing."

**Beat 4: The pivot — acknowledge what they already have (10:00-12:00)**
"You're probably already collecting Claude Code OTEL. Most of my customers
are. So you can see token spend per developer in Grafana. Great. But
answer me this: when a developer burns $200 in a morning, do you know
*why*? Was it cold cache thrashing? Runaway agents spawning more agents?
A build tool loop that should have been suppressed? You can see the cost.
You can't see the cause."

**Beat 5: The enrichment demo (12:00-15:00)**
Pull up Grafana. Show two panels side by side:
- Left: Claude Code's own `claude_code.cost.usage` metric. Flat numbers.
  Developer X spent $47 today.
- Right: Thermal's enrichment layer. Same developer. 14 agents spawned,
  8 concurrent at peak, 340 gate suppressions, threat level hit MELTDOWN
  twice, memory headroom dropped to 2GB. Agent-hours: 6.2. Gate time
  saved: 45 minutes of CPU.

"The left panel tells you what they spent. The right panel tells you why
they spent it, and what Thermal prevented them from spending."

**Beat 6: The close (15:00-16:00)**
"The dashboard is free. Open source. Statusline cost is free. The
enrichment layer that tells your platform team *why* costs are what they
are — that's the enterprise license. Your developers install it today.
Your platform team sees the value next week."

### Why this works better

The old demo assumed ignorance ("you have no idea which teams burn API
credits"). The new demo assumes competence ("you already collect token
data — here's what you're missing"). This is more respectful and more
accurate for 2026 enterprise buyers. It also makes the sale harder to
dismiss as "we can build that ourselves" because the answer is: you
already built the token collection part. The system-state enrichment is
the part you can't build without instrumenting every developer's machine
at the OS level.

---

## 3. Cost Consciousness in the Statusline

### Recommendation: Free tier. No question.

This is the single most important PLG decision in the entire product. The
statusline cost number must be free.

**The argument for free:**
- Every developer who installs Thermal sees their spend. This creates an
  ambient awareness that doesn't exist today. Claude Code has `/cost` but
  nobody runs it mid-session. The statusline number is always visible.
- Developers who see their spend talk about their spend. "I burned $12 on
  that refactor" becomes water-cooler conversation. This is viral.
- When the platform team hears developers talking about cost, they ask
  "how do you know that?" The answer is Thermal. This is the champion
  creation moment.
- Making it enterprise-only kills the viral loop. The developer sees a
  blank statusline. They don't talk about cost. The platform team never
  hears about Thermal. The PLG engine dies.

**The argument against free (and why it's wrong):**
- "Cost is an enterprise feature." No. Cost *attribution across a fleet*
  is an enterprise feature. Individual cost visibility is a developer
  productivity feature. Every developer wants to know what they're
  spending, even on a personal Max plan.
- "We're giving away enterprise value." The statusline shows one number:
  your session cost. The enterprise tier shows that number across 200
  developers, broken down by team, correlated with system state, with
  alerting rules. The gap is enormous.

**Implementation for free tier:**
The statusline reads from Claude Code's local JSONL session files. No OTEL
required. No daemon required. No configuration beyond `cost = true` in
config.toml (or `COOLANT_COST=1` env var). It parses `usage` fields from
the running session's transcript JSONL at `~/.claude/projects/`. This is
Rung 1 — zero infrastructure, immediate value.

**Default rates vs. enterprise rates:**
- Free tier uses Anthropic's published list prices, updated with each
  Thermal release. Good enough for individual awareness.
- Enterprise tier supports a `[cost]` config block with negotiated rates
  per model. "Your company negotiated 7% off Opus. Here's what you're
  *actually* spending." This is a small but psychologically powerful
  feature — it makes the discount tangible to every developer, which
  reinforces the value of the enterprise relationship with Anthropic.

---

## 4. Enterprise Discounts as a Feature

### Verdict: Compelling, but not a selling point. A retention point.

No CTO signs a PO because "we can show developers their negotiated rate."
But once deployed, the ambient visibility of "you're paying $0.93 instead
of $1.00 per unit" reinforces three things simultaneously:

1. The value of the enterprise Anthropic contract ("our discount is real
   and visible").
2. The value of the platform team that negotiated it ("they saved us money").
3. The value of Thermal that surfaces it ("without this, the discount is
   invisible").

This is a retention and expansion feature, not an acquisition feature.
Include it in the enterprise tier, mention it in demos when relevant, but
don't lead with it.

**Config design:**

```toml
[cost]
# Override list prices with negotiated rates (per 1M tokens)
[cost.rates.claude-opus-4-6]
input = 13.95       # list: $15.00
output = 69.75      # list: $75.00
cache_read = 1.67   # list: $1.875 (Anthropic's actual list price has changed)
cache_creation = 17.44  # list: $18.75

# Or a blanket discount
# discount_pct = 7
```

---

## 5. The Three-Surface Model

### Statusline -> Thermo TUI -> Grafana

**Verdict: Coherent and sellable, but only if each surface has a clear
"why this one" answer.**

The risk is that a buyer sees three things and thinks "which one do I
actually need?" The answer must be immediately obvious:

| Surface | Who | When | Value |
|---------|-----|------|-------|
| Statusline | Every developer | Always (ambient) | "What am I spending right now?" |
| Thermo TUI | Developer who wants to look deeper | On-demand (opt-in) | "What is my machine doing right now?" |
| Grafana | Platform team | Fleet review (scheduled) | "What is every machine doing over time?" |

**The sales narrative:**
"Statusline is the speedometer. Thermo is the engine diagnostic. Grafana
is fleet management. You don't pick one. You use the right one for the
right moment. The statusline ships free with the plugin. Thermo is a
binary your developers can install if they want the full dashboard.
Grafana is where your platform team lives."

**Important: the buyer is the platform team, not the developer.** The
platform team doesn't care about the statusline or the TUI. They care
about Grafana. The statusline and TUI are what get developers to install
Thermal, which is what generates the data that flows to Grafana. This is
the flywheel. Explain the flywheel, not the surfaces.

---

## 6. Pricing Evolution

### Recommendation: Per-seat annual, unchanged. But add a site license tier.

The daemon changes the deployment model but should not change the pricing
model. Per-seat is what procurement teams know. Usage-based pricing for a
monitoring tool is a non-starter — it creates perverse incentives (less
monitoring when you need it most).

**Revised tiers:**

| Tier | Price | What you get |
|------|-------|-------------|
| Free | $0 | Dashboard, statusline, gate system, cost visibility (list rates) |
| Team | $8/dev/month (annual) | OTEL export, fleet labels, daemon mode, enterprise rates, Grafana dashboards |
| Enterprise | $12/dev/month (annual) | Team tier + compliance docs, alerting templates, mTLS, endpoint allowlisting, MDM config validation |
| Site license | Custom (negotiated) | Enterprise tier, unlimited seats, named account support |

**Why three paid tiers, not one:**
- The Team tier at $8 is the "startup-friendly" entry. 20 developers,
  $160/month. Falls under most managers' discretionary budget. No
  procurement involved.
- The Enterprise tier at $12 includes the compliance artifacts that
  Fortune 500 infosec teams require. This is the "we need the PIA template
  and the SIG Lite answers" tier.
- The Site license is for the 1000+ developer orgs where per-seat
  counting is a friction point.

**Per-machine pricing was considered and rejected:**
The daemon runs on every developer machine. Per-machine pricing would be
natural but creates a problem: developers with multiple machines
(desktop + laptop, or a fleet of CI runners) get double-counted. Per-seat
is cleaner.

---

## 7. Max/Pro Startup Path

### The realistic deployment for a 20-person startup

This is the segment most likely to self-serve and least likely to tolerate
friction. Here's the honest path:

**Week 1: Organic adoption (Rung 0-1)**
- 2-3 developers install Thermal because they saw it on Todd's stream or
  in a demo. Plugin install via Claude Code marketplace. Dashboard binary
  via `install.sh`.
- They enable `cost = true` in config.toml. Now they see session cost in
  the statusline.
- No coordination required. No admin. Pure individual adoption.

**Week 2-3: Word of mouth (still Rung 1)**
- "Have you seen Thermal? It shows you what you're spending." More
  developers install it. The engineering manager notices.

**Week 4: The engineering manager asks (Rung 2)**
- "Can I see what the whole team is spending?" This is the upgrade moment.
- Reality check: there's no MDM, no managed settings, no admin console.
  The startup path is:
  1. Add a shared `.envrc` to the monorepo with
     `CLAUDE_CODE_ENABLE_TELEMETRY=1` and collector endpoint.
  2. Stand up a collector (docker-compose with OTEL Collector + Grafana,
     or use Grafana Cloud's free tier).
  3. Import Thermal's Grafana dashboard JSONs.
- This takes a competent developer about 2 hours. It should take 30
  minutes. **Recommendation: ship a `docker-compose.yml` in the enterprise
  repo that stands up Collector + Prometheus + Grafana with Thermal
  dashboards pre-loaded.** This is the "startup kit."

**Week 5+: Paid conversion (Rung 3-4)**
- The engineering manager sees fleet data. Wants gate ROI, enterprise
  rates, daemon mode for reliability.
- Self-serve purchase via website. Credit card. No sales call required.
  Annual billing optional but not required (monthly OK for startups).

**Key insight for this segment:** The startup path must be entirely
self-serve. If a 20-person startup needs to talk to a salesperson, you've
already lost them. The Team tier at $8/dev/month must be purchasable with
a credit card in under 5 minutes.

**The Max/Pro token data problem is solved.** The original spec flagged
Max/Pro customers as potentially unable to access token data. This is no
longer true. `CLAUDE_CODE_ENABLE_TELEMETRY=1` works for all plan types.
The `claude_code.api_request` event includes all four token types and
`cost_usd` regardless of whether the account is Enterprise, Max, or Pro.
The only difference is enforcement: Enterprise admins can push the env
vars via MDM; Max/Pro developers must opt in individually.

The Anthropic Admin API (`/v1/organizations/usage_report/claude_code`)
is not available to Max/Pro individual accounts, but this is irrelevant
for Thermal's use case because the OTEL telemetry from Claude Code itself
is sufficient. Thermal doesn't need server-side API access when it can
read the client-side telemetry directly.

---

## 8. Gate Suppression ROI vs. Cost Attribution

### "Thermal saved your org 12 hours of CPU time" vs. "Your Claude spend was $47K"

**Verdict: Lead with cost. Close with gate ROI.**

The original spec positioned gate suppression ROI as the headline
enterprise metric. This was correct when Thermal couldn't see token costs.
Now that Claude Code's OTEL provides `cost_usd` per request, the cost
attribution story is immediately available and far more compelling to
buyers.

**Why cost wins as the headline:**
- CFOs and CTOs think in dollars, not CPU-hours.
- "$47K this quarter on Claude, here's the breakdown by team" is a
  sentence that triggers a procurement conversation.
- "12 hours of CPU time saved" requires translation ("what's that worth?")
  and sounds like a nice-to-have, not a must-have.

**Why gate ROI wins as the closer:**
- Cost attribution is available from Claude Code's own OTEL. A
  sufficiently motivated platform team could build a Grafana dashboard
  from `claude_code.cost.usage` without Thermal.
- Gate suppression ROI is *unique to Thermal*. Nobody else can tell you
  "we prevented 847 redundant build tool invocations." This is the moat.
- The ROI number answers "what does Thermal save us?" directly. The cost
  number answers "what are we spending?" which is useful but not
  Thermal-specific.

**The revised pitch sequence:**
1. "Here's what your team spent this quarter." (Cost attribution — gets
   attention, establishes relevance.)
2. "Here's why they spent it." (System state enrichment — agent counts,
   threat levels, resource correlation. This is what Claude Code's OTEL
   can't tell you.)
3. "Here's what Thermal prevented them from spending." (Gate ROI — the
   unique value, the moat, the closer.)

**The combined metric that matters most:**
`thermal.gate.time_saved_seconds` converted to dollars using the
customer's blended compute rate. "Thermal saved your organization $X in
prevented CPU waste this week." This requires a configurable
dollar-per-CPU-hour rate in the enterprise config, which should ship as
a simple config line:

```toml
[cost]
cpu_hour_rate = 0.50  # $/hour for developer machine CPU time
```

Most enterprises can estimate this from their hardware amortization or
cloud equivalent. The default should be conservative ($0.25/hr) so the
number is credible even without configuration.

---

## 9. Progressive Value Mapped to the Sales Cycle

### The ladder as a sales funnel

| Rung | What happens | Who cares | Sales stage |
|------|-------------|-----------|-------------|
| **0: Free install** | Developer installs plugin + dashboard. Machine doesn't melt. | Individual developer | **Awareness.** No sales involvement. PLG only. |
| **1: Cost statusline** | Developer sees session cost. Talks about it. | Individual developer, peers | **Champion creation.** The developer who says "you should see what I'm spending" becomes the internal champion. |
| **2: Claude Code OTEL** | Platform team enables OTEL fleet-wide. Token spend visible in Grafana. | Engineering manager, platform team | **Discovery.** The platform team realizes they can see Claude usage. They're getting value from Claude Code's own telemetry. Thermal is not involved yet at the fleet level. |
| **3: Daemon deployment** | Platform team deploys Thermal daemon. System state enrichment flows to Grafana alongside token data. | Platform team lead | **Evaluation.** "Now I can see not just what they're spending, but why. And I can see what Thermal is preventing." This is the trial period. |
| **4: Enterprise rates + labels** | Configures negotiated rates, team labels, cost center attribution. | Finance, engineering VP | **Business case.** The numbers are real, attributed, and at negotiated rates. "This is what Team X actually costs us." |
| **5: Full integration** | Compliance docs, alerting rules, production Grafana dashboards. | CISO, CTO, procurement | **Close.** The compliance package satisfies infosec. The alerting rules satisfy SRE. The PO gets signed. |

### Which rung gets the champion excited?

**Rung 1.** The moment a developer sees "$4.20 this session" in their
statusline, they're hooked. This is the "oh cool" moment. It costs
nothing, requires no infrastructure, and creates an emotional connection
to cost that doesn't exist when cost is invisible.

### Which rung gets the PO signed?

**Rung 4.** When the engineering VP can see "Platform team: $12K/month,
Product team: $8K/month, ML team: $27K/month" with per-developer
breakdowns and gate ROI numbers, the business case writes itself. The PO
isn't for Thermal — it's for "AI cost observability." Thermal is just the
best (only) tool that provides it with system-state context.

### Which rung is the "oh shit, we need enterprise" moment?

**Rung 3.** When the daemon starts reporting system state alongside the
token data they already have from Claude Code OTEL, the platform team
sees something they can't get any other way: the *cause* behind the cost.
Agent count spikes correlated with cost spikes. Threat level transitions
correlated with memory pressure. Gate suppressions showing what was
prevented. This is the moment the free tier stops being sufficient.

### Can you close a deal at Rung 2?

**No, and you shouldn't try.** At Rung 2, the customer is getting value
from Claude Code's own OTEL. Thermal hasn't entered the fleet picture yet.
If you try to sell here, the objection is "we already have token data in
Grafana, what do we need you for?" Let them sit at Rung 2 and discover
the gap themselves. The gap is: they can see cost but not cause. When they
ask "why did Developer X burn $200 yesterday?", they can't answer it from
token data alone. That's when they're ready for Rung 3.

### Natural expansion motion from Rung N to N+1

| Transition | Trigger | Friction |
|-----------|---------|----------|
| 0 -> 1 | Developer curiosity about cost | One config line. Zero friction. |
| 1 -> 2 | Engineering manager asks "can I see the whole team?" | Two env vars + collector. Low friction for competent team. |
| 2 -> 3 | Platform team asks "why is this developer so expensive?" | Daemon deployment. Medium friction (binary distribution, MDM for enterprise). |
| 3 -> 4 | Finance asks "what are we actually paying vs. list price?" | Config file update. Zero friction. |
| 4 -> 5 | Infosec asks "what data does this emit?" | Compliance package. Zero friction (we ship it). |

The critical friction point is 2 -> 3. This is where a startup needs the
docker-compose kit and an enterprise needs the MDM deployment guide. Both
should be one-page documents that a platform engineer can execute in under
an hour.

---

## 10. The Daemon Changes Everything (and Nothing)

### What "Invisible Thermal" means for sales

The daemon (headless mode) is a fundamentally different product surface
than the TUI. The TUI is a demo tool. The daemon is an enterprise
deployment artifact. This duality is a strength, not a confusion, as long
as the narrative is clean.

**For developers:** "Thermal is the dashboard that keeps your machine
from melting."

**For platform teams:** "Thermal is the daemon that tells you what 200
Claude agents are doing to your infrastructure."

**Same binary, different mode.** The daemon is `thermo --daemon` (or
launched via launchd/systemd). It collects and exports without rendering.
When a developer also wants the TUI, it connects to the running daemon
for data instead of collecting independently. This means the daemon is
the single data authority on the machine, and the TUI is just a view.

**Sales implication:** You sell the daemon to the platform team. You give
the TUI to the developer. The platform team never sees the TUI. The
developer may never know the daemon is running. Both are happy.

**Deployment artifacts for enterprise:**
- `thermo-darwin-arm64` and `thermo-darwin-amd64` (same binary, daemon
  mode via flag or launchd plist)
- `com.coolant.thermal.plist` — launchd plist template for macOS
- `config.toml` — MDM-distributable config with endpoint, labels, rates
- One-page deployment guide: "push these three files via Jamf/Intune"

### Demo flow adjustment for daemon

The TUI is still the hook. You don't demo a daemon. You demo the
dashboard, then say "this same binary runs headless on every machine
in your fleet. Your developers don't need to see it. Your Grafana does."

---

## 11. Spec Updates Required

Based on this review, the following changes should be made to the
consensus spec:

### Tier split table (Section 1)

Add to Free tier:
- Cost visibility in statusline (list rates, from local JSONL)

Add to Enterprise tier:
- Cost visibility with negotiated rates
- Daemon/headless mode
- Docker-compose startup kit (for startup segment)
- Compliance documentation package

### Cost signal section (Section 3)

The "blocked on external data source" framing is obsolete. Claude Code's
OTEL provides per-request `cost_usd` with `model` and `user.account_id`
attribution. Thermal should consume this data (via OTEL collector or
local JSONL) and enrich it with system state. The reserved metric names
(`thermal.tokens.input`, etc.) should be promoted to "ships with v1" and
sourced from Claude Code's telemetry.

**New architecture for cost data:**
```
Claude Code OTEL (api_request events)
    ├── Direct to customer's collector (existing path, no Thermal involvement)
    └── Thermal daemon reads from same collector (or local JSONL)
            └── Enriches with system state, gate events, agent lifecycle
                    └── Emits fused metrics to customer's Grafana
```

Thermal doesn't duplicate the token data. It joins it with system context
and emits the enriched view.

### Configuration section (Section 4)

Add `[cost]` config block:
```toml
[cost]
enabled = true           # show cost in statusline (free tier)
cpu_hour_rate = 0.50     # $/hr for gate ROI calculation (enterprise)
# discount_pct = 7       # blanket discount (enterprise)
# [cost.rates.claude-opus-4-6]  # per-model overrides (enterprise)
```

### Demo section (Section 8)

Replace with the revised demo flow from Section 2 of this document.

### Implementation sequence (Section 10)

Add Phase 0 before current Phase 1:
- **Phase 0: Cost statusline (free tier, no OTEL)**
  1. Parse local JSONL session files for `usage` fields
  2. Compute session cost using published model rates
  3. Display in braille statusline
  4. Support `[cost]` config for enabling/disabling and rate overrides

This ships before any enterprise OTEL work and generates the PLG flywheel
immediately.

### New section: Startup Kit

Add a section describing the docker-compose deployment for startups:
- OTEL Collector configured for `claude_code.api_request` and
  `thermal.*` metrics
- Prometheus with appropriate retention
- Grafana with Thermal dashboards pre-imported
- One `docker-compose up` command, one `.envrc` block to distribute

---

## 12. Open Questions for the Next Panel

1. **Should the daemon auto-start on install?** For enterprise MDM
   deployments, yes (launchd plist pushed with the binary). For startup
   self-serve, probably not (let them opt in). This needs a clear policy.

2. **How does Thermal consume Claude Code's OTEL data?** Two options:
   (a) Thermal's daemon queries the same collector that Claude Code pushes
   to, or (b) Thermal reads Claude Code's local JSONL directly. Option (b)
   is simpler and works without OTEL infrastructure, but only gives
   machine-local data. Option (a) requires a collector but enables the
   enrichment-at-the-collector pattern. Recommendation: support both.
   JSONL for free tier cost. Collector integration for enterprise
   enrichment.

3. **Does Thermal re-emit Claude Code's token data, or only its own
   enrichment metrics?** If Thermal re-emits token data, customers have
   one Grafana datasource. If it doesn't, they need to correlate two
   streams. Recommendation: Thermal re-emits a fused view
   (`thermal.cost.usd` sourced from Claude Code data, enriched with
   `team`, `threat_level`, `agent_count` attributes that Claude Code
   doesn't have). This is the enrichment value proposition made concrete.

4. **Cache efficiency as a team health metric.** The Perplexity research
   confirms `cache_read_tokens` and `cache_creation_tokens` are available
   from Claude Code OTEL. A team with 90% cache hit rate is using Claude
   well. A team at 40% is burning money on cold starts. This should be a
   Grafana panel and possibly an alerting rule. Is it free or enterprise?
   Recommendation: enterprise (it requires fleet aggregation to be
   meaningful).

5. **Bedrock and Vertex customers.** Claude Code's OTEL telemetry works
   regardless of backend (Anthropic API, Bedrock, Vertex). But the
   Anthropic Admin API (`/v1/organizations/usage_report/claude_code`)
   explicitly excludes Bedrock usage. For customers running Claude Code
   on Bedrock, Thermal's OTEL-based approach is strictly superior to the
   Admin API for cost visibility. This is a selling point for Bedrock
   shops.

6. **The compliance package for Max/Pro.** The Enterprise tier includes
   compliance docs (PIA, SIG Lite, data flow diagrams). Do startups on
   Max/Pro plans need these? Usually not, but SOC 2-seeking startups might.
   Consider making the compliance package available as a standalone add-on
   or including it in Team tier.

---

## 13. Summary: What's Different in v2

| v1 Assumption | v2 Reality | Implication |
|---------------|-----------|-------------|
| Thermal is the sole OTEL emitter | Claude Code emits rich OTEL natively | Thermal is an enricher, not the authority |
| Token data is blocked on external sources | Token data is available today via OTEL and JSONL | Cost statusline can ship immediately as free tier |
| Max/Pro customers may never get token data | All plan types get identical OTEL data | Max/Pro segment is viable for paid conversion |
| Gate ROI is the headline metric | Cost attribution is immediately available | Lead with cost, close with gate ROI |
| The demo sells a dashboard | The demo sells context that token data alone can't provide | "You have the data. You're missing the context." |
| One enterprise tier | Two paid tiers (Team + Enterprise) + site license | Self-serve startup path + procurement-ready enterprise path |
| OTEL export is the product | Enrichment of existing OTEL data is the product | The moat is system state fusion, not data collection |

The product is stronger than the original spec imagined. The market has
moved in our direction — Claude Code's native OTEL means every enterprise
customer is already generating the token data. They just don't have the
system context to make sense of it. That's the gap Thermal fills. The
free tier creates the champions. The daemon creates the fleet data. The
enrichment creates the enterprise value. Ship the cost statusline first.

---

*End of findings. Ready for spec revision.*
