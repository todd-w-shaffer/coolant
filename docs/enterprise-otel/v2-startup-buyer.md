# Thermal Enterprise: Startup Buyer Evaluation

**Author:** CTO, 35-person startup (18 developers on stacked Claude Max Pro $200/month)
**Date:** April 2026
**Context:** No platform team. No MDM. No OTEL collector. No Prometheus. No Grafana. AWS backend, CloudWatch + some Datadog APM for production. $43K/year in Claude seats. Zero visibility into which developers are getting value.

---

## Bottom Line Up Front

I read the entire spec package. The enterprise spec is not for me. It is for a VP of Platform Engineering at a Fortune 500 with Jamf, Grafana Alloy, a security review board, and 4-8 weeks of patience for daemon approval. I have none of those things. I have a credit card and a Slack channel.

But the underlying problem the spec identifies is exactly my problem: I am spending $43K/year on Claude seats and I cannot answer the question "is this money well spent?" The Anthropic billing page tells me nothing useful — it is per-seat, unlimited usage, flat rate. There is no "Developer X used 3x more than Developer Y" because there is no metering surface for Max/Pro plans.

What I actually want does not exist yet in this spec. But the pieces are all here, and the gap between "what the spec describes" and "what I would pay for" is surprisingly small if someone builds the right wrapper.

---

## What I Read and What I Thought

### The origin story

Entertaining. I like the "scratching your own itch" angle. But I do not care how the product was born. I care whether it solves my problem.

### The spec

This is a self-hosted, bring-your-own-Grafana product. I stopped reading the Grafana dashboard section because I do not have Grafana and I am not going to run Grafana. The TOML config, the MetricSink interface, the build tags — this is infrastructure-engineer pornography. I need a URL I can open in Safari on my phone.

### The cost attribution doc

This is where it got interesting. The distinction between Profile A (Enterprise, centralized billing, admin APIs) and Profile B (stacked Max/Pro, billing islands) — that is me. Profile B. And the key insight is correct: for Max/Pro customers, Thermal's proxy metrics may be the only fleet-level cost signal available. There is no Admin API for us. There is no centralized billing dashboard. Each developer is an island.

The agent-hours metric is the right proxy. I do not need exact token counts. I need "Alex ran Claude for 6.2 hours of active agent time this week, Jamie ran it for 1.4 hours." That tells me who is getting value and who is not, even without dollar amounts.

### The Perplexity data sources reference

The critical finding: `CLAUDE_CODE_ENABLE_TELEMETRY=1` works for all plan types. The `claude_code.api_request` log event carries per-request token counts, cost_usd, model, and session ID regardless of whether the account is Enterprise, Max, or Pro. This means my developers CAN emit telemetry. The question is: emit it to where?

For me, the answer cannot be "your OTEL collector" because I do not have one. The answer needs to be "our endpoint."

### The platform architect

The progressive value ladder and daemon architecture are well-thought-out but irrelevant to my deployment model. I am not going to run a daemon on 18 machines via launchd plists. I do not have MDM. I do not have a platform team to write one. The daemon is a future state for me, maybe, if the simpler thing works first.

The one thing that caught my eye: the architect says Rung 1 (cost statusline) requires zero enterprise code and should be free. Good. That is my entry point.

### The sales engineer

The sales engineer understands my segment better than the rest of the spec. The Max/Pro startup path (Section 7) is realistic: organic adoption, word of mouth, engineering manager asks "can I see the whole team?" But then it falls apart — the answer to that question is "stand up a collector, docker-compose, import Grafana dashboards." That is 2 hours of a competent developer's time. I could do it. But I will not. Because next week I will need to maintain it, and the week after that it will break, and then nobody looks at it anymore.

The docker-compose startup kit is a band-aid over the real problem: I should not be running infrastructure to monitor my AI spend.

### The security review

Thorough. The whitelist parser for session JSONL is the right architecture — extract only numeric usage fields, never touch conversation content. The data classification is helpful for my mental model of what data is and is not sensitive.

For the hosted angle I am about to describe: the security review's classification of token usage objects as "Internal" (numeric counters, no content leakage, reveals workload intensity but not content) is exactly the line I would draw. Numbers are fine to send to a third party. Content is not.

### The enterprise buyer

This is the anti-me. They have 300 developers, Grafana Alloy, Jamf, an infosec team, and a 10-week deployment timeline. Their Rung 2 (push 3 env vars via Jamf, point at existing collector) gets them 80% of value. I cannot do their Rung 2 because I have no collector to point at.

But their honest assessment is instructive: even with all that infrastructure, the enterprise buyer says "if we could only get to Rung 2 this quarter, it is enough." Cost visibility is the thing. Everything else is nice-to-have.

---

## The Hosted Dashboard: What I Actually Want

Here is the product I would pay for today:

### Sign up

I go to a website. I enter my email and company name. I get back:
- An API key (or bearer token)
- An endpoint URL (something like `https://ingest.thermal.dev/v1/metrics`)
- A link to my dashboard at `https://app.thermal.dev/my-company`

Total time: 2 minutes.

### Developer setup

I post this in Slack:

```
Hey team — we're tracking our Claude usage now. Run this in your terminal:

export CLAUDE_CODE_ENABLE_TELEMETRY=1
export OTEL_LOGS_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_ENDPOINT=https://ingest.thermal.dev/v1
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer tk_abc123"

Add those four lines to your .zshrc and restart your terminal.
That's it. Takes 30 seconds.
```

No binary to install. No daemon. No config file. No Docker. Claude Code's own OTEL telemetry does the heavy lifting. Thermal hosts the collector and the dashboards. The developer does not install anything — they set env vars that Claude Code already supports.

Total developer time: 2 minutes (copy-paste into .zshrc, restart terminal).

### What I see

I open `https://app.thermal.dev/my-company` on my phone. I see:

**Team Overview (this week)**
- Total Claude spend across team: $2,847
- Active developers: 16 of 18
- Average session cost: $12.40
- Top spender: Jamie ($489) — 38 sessions, 4.2% cache hit rate
- Most efficient: Alex ($340) — 12 sessions, 91% cache hit rate, 12 PRs merged

**Per-Developer Table (sortable)**

| Developer | Sessions | Spend | Cache Hit % | Avg Session | Trend |
|-----------|----------|-------|-------------|-------------|-------|
| Jamie | 38 | $489 | 4.2% | $12.87 | up |
| Morgan | 27 | $412 | 23% | $15.26 | flat |
| Alex | 12 | $340 | 91% | $28.33 | down |
| ... | | | | | |

**Insights**
- Jamie's cache hit rate is 4.2%. Team average is 47%. This developer may be starting fresh sessions too frequently instead of continuing existing ones.
- 3 developers have not used Claude this week. (Riley, Sam, Jordan)
- Team spend is up 23% week-over-week.

That is the dashboard. Not Grafana. Not Prometheus queries. Not OTEL metric names. A web page that answers business questions.

### What I do NOT see

- No conversation content. Ever.
- No prompt text.
- No code snippets.
- No file paths.
- No command strings.
- No tool outputs.

The hosted endpoint receives only what Claude Code's OTEL emits by default: token counts, cost_usd, model, session.id, user.account_id, duration_ms. Numeric counters and identifiers. The security review classifies this as "Internal" — workload intensity, not content.

### Optional: Install Thermal for enrichment

If a developer also installs the Thermal plugin and dashboard, I get bonus data in the hosted dashboard:
- Gate suppression counts ("Thermal prevented 47 redundant tsc runs for this developer this week")
- Agent lifecycle (concurrent agent counts, agent-hours)
- Threat level distribution (% time in COOL/WARM/HOT/MELTDOWN)
- System utilization correlation

This is the Thermal-specific value add on top of Claude Code's native telemetry. It is additive, not required. The baseline dashboard works with zero Thermal installation — just Claude Code's own OTEL.

---

## Would I Pay for This?

Yes. Without hesitation. Here is my math:

I am spending $43,200/year on Claude seats (18 developers x $200/month). I have zero visibility into whether that money is well-spent. If this dashboard tells me that 3 developers barely use Claude, I can downgrade them to the $20 Pro plan and save $6,480/year. If it tells me that Jamie's 4% cache hit rate is costing us $200/month extra because of cold-start thrashing, and a 30-minute coaching session fixes it, the dashboard paid for itself in month one.

### What I would pay

- $5-8/developer/month for the hosted dashboard with Claude Code OTEL data only.
- $10-12/developer/month if it includes the Thermal enrichment layer (gate ROI, agent lifecycle, system correlation) for developers who have Thermal installed.
- At 18 developers, that is $90-$216/month. Under $2,600/year. That is a rounding error against $43K in Claude spend.
- Self-serve. Credit card. Monthly billing. No annual commitment required. No sales call.

### What I would NOT pay for

- $15+/developer/month. At that price for 18 seats I start thinking "I could just check in with developers manually."
- Anything that requires a sales call to purchase.
- Compliance documentation packages. I do not need a PIA template or SIG Lite answers. I am not filling out security questionnaires for a monitoring dashboard.
- mTLS configuration. Endpoint allowlisting. MDM config validation. I do not have MDM.
- Self-hosted anything. Docker-compose kits. Collector configuration templates. I am paying you to not run infrastructure.

---

## What Data I Am Comfortable Sending

### Yes, send it (the "Internal" classification from the security review)

- Token counts (input, output, cache read, cache creation) per request
- Cost in USD per request
- Model name
- Session ID
- User account ID (for per-developer attribution)
- Request duration
- Cache hit rate (derived from token counts)
- Thermal gate events (tool name only — "tsc", "eslint" — not the command string)
- Thermal agent lifecycle (spawn, death, duration)
- Thermal system metrics (CPU %, memory %, threat level)

These are numbers. They reveal workload intensity. They do not reveal what my developers are building.

### No, absolutely not

- Prompts. Never.
- Code. Never.
- Tool outputs (file contents, command results). Never.
- File paths (encode project names and repository structure). No.
- Full command strings (routinely contain secrets, connection strings). No.
- Organization structure beyond what I explicitly configure (team names are fine because I typed them).

### The line

The line is: aggregate numeric metrics and event counts are fine. Anything that could reconstruct what a developer was working on, thinking about, or discussing with Claude is not fine. The security review's data classification is almost exactly right — I would send everything classified as "Internal," some things classified as "Confidential" (session ID, user ID — needed for attribution), and nothing classified as "Restricted."

If I had to explain this to an investor: "We send Claude usage metrics — how many tokens each developer consumes, cost per session, cache efficiency rates — to a monitoring dashboard. We do not send any code, prompts, or conversation content. The monitoring service sees numbers, not intellectual property."

That is a sentence I can say with a straight face.

---

## How I Get 18 Developers to Configure This

### The realistic path

I do not have MDM. I cannot push configuration to machines. Here is what I can do:

1. **Slack message.** "Hey team, paste these 4 lines into your .zshrc. Takes 30 seconds." This gets 10-12 of 18 developers within 48 hours. The ones who care about tooling do it immediately. The ones who are in the middle of something do it when they see the reminder.

2. **Follow-up in standup.** "Has everyone set up the Claude monitoring? Riley, Sam, Jordan — ping me if you need help." This gets 3-4 more.

3. **Add to onboarding doc.** New hires get it automatically.

4. **The stragglers.** 1-2 developers will never do it until I sit next to them and do it for them. This is fine. 16 of 18 is enough for useful data.

### What makes this work

- **Zero binary installation.** If it requires installing software, adoption drops by 50%. Env vars in .zshrc are copy-paste.
- **No config file.** If it requires creating a TOML file in ~/.config/coolant/, half my developers will put it in the wrong place.
- **Immediate value.** The developer should see something change. Even if it is just "OTEL telemetry enabled" in the Claude Code output, they know it worked.
- **No ongoing maintenance.** Set it once, forget it. If the endpoint changes, I post a new Slack message.

### What kills adoption

- **A 5-minute setup guide.** If the Slack message needs a link to a guide, half the developers will open the link and close the tab. The setup must be the Slack message itself — no external document.
- **A binary download + install step.** Even `brew install thermal` adds friction that env vars do not have. The baseline (Claude Code OTEL to hosted endpoint) must be zero-install.
- **Configuration that requires understanding OTEL.** My developers do not know what OTEL is. They should not need to. "Paste these env vars" is all they need to know.
- **Anything that requires Docker on a developer machine.** I would lose half the team before they finished pulling the image.

---

## The Progressive Value Ladder From My Perspective

The enterprise spec's rungs do not map to my world. Here is my ladder:

### Step 1: I try Thermal on my own machine (free, 10 minutes)

I install the plugin. I install the dashboard binary. I see the braille strip in tmux. The gate system caps my parallel test runners. I think "that is cool."

**Friction:** Low. Install script. Standard developer tooling adoption.

**Value:** Personal. I see my machine not melting. Neat.

### Step 2: I see my session cost (free, 1 minute)

I add `cost = true` to my config. The statusline shows "$4.20 this session." I think "huh, that refactoring session cost me $14."

**Friction:** One config line. Zero friction.

**Value:** Personal cost awareness. This is the "oh cool" moment from the sales engineer doc. They are right — this is the PLG hook.

### Step 3: I tell my team to try Thermal (Slack message, 5 minutes each)

I post in #engineering: "Install this, it is cool, you can see what Claude is costing you per session." 8-10 developers install it over the next week. Organic adoption.

**Friction:** Each developer runs an install script. Medium friction — some will, some will not.

**Value:** Individual cost awareness spreading. Developers start talking about cost. "I burned $22 on that migration."

### Step 4: I want to see the whole team (THIS IS THE GAP)

I want a dashboard showing all 18 developers. I cannot get it from the current spec.

**Option A (spec's answer):** Stand up an OTEL collector, Prometheus, and Grafana. Import dashboard JSONs. Maintain the infrastructure.

**Option B (what I want):** Sign up for a hosted service. Get an endpoint. Tell developers to set 4 env vars.

Option A is a project. Option B is a purchase. I will do B every time.

**This is where the spec stalls for my segment.** The jump from "individual developer tool" to "fleet visibility" requires infrastructure I do not have and will not build.

### Step 5: Developers set env vars pointing at hosted endpoint (2 minutes each)

Slack message with 4 env vars. Developers copy-paste into .zshrc. Claude Code starts emitting telemetry to the hosted endpoint.

**Friction:** Minimal if the Slack message is self-contained. No binary installation required.

**Value:** Data flowing. I can see usage. This is the inflection point.

### Step 6: I open a browser and see my team's Claude spend

The hosted dashboard shows per-developer cost, cache efficiency, usage trends. I can answer "how much are we spending on Claude?" and "who is getting value?"

**Friction:** Zero. I open a URL.

**Value:** The thing I have been wanting since I started paying $43K/year for Claude seats.

### Where it stalls

**Step 3 to Step 4 is the cliff.** This is the gap between "cool individual tool" and "useful management tool." For the enterprise buyer, this gap is bridged by their existing infrastructure (push env vars via Jamf, point at existing collector). For me, there is no bridge. The hosted dashboard IS the bridge.

**Step 5 is the second friction point.** Getting 18 developers to set env vars without MDM is a social engineering problem, not a technical one. It works if the Slack message is copy-paste simple. It fails if it requires reading documentation.

---

## Comparison to Just Checking the Anthropic Billing Page

The Anthropic billing page for Max/Pro plans shows: you are on the $200/month plan. That is it. There is no per-developer usage breakdown because each developer has their own account. There is no aggregate view because there is no organization. There is no "Developer X used 3x more than Developer Y" because Anthropic has no concept of "your team."

Even the Enterprise Admin API, which the Perplexity doc covers in detail, is not available to Max/Pro accounts. The Claude Code Analytics API that returns per-user daily token breakdowns requires an Admin API key that only Enterprise and API orgs can generate.

**The delta value of a hosted dashboard over the Anthropic billing page is approximately infinite for Max/Pro customers.** We go from zero visibility to full visibility. There is nothing to compare against because the current state is nothing.

This is the strongest argument for the hosted product: for the startup segment, there is no alternative. Not a worse alternative — no alternative at all.

---

## Would I Trust a Startup (Thermal) With This Data?

### What I need to see

1. **A clear privacy policy** stating: "We receive only aggregate usage metrics (token counts, cost, model, session identifiers). We do not receive, store, or process prompts, code, conversation content, or tool outputs."

2. **An architecture diagram** showing that the hosted endpoint is an OTEL collector that receives Claude Code's standard telemetry signals. The diagram should make clear that prompt content and tool details are not emitted unless the developer explicitly enables `OTEL_LOG_USER_PROMPTS=1` and `OTEL_LOG_TOOL_DETAILS=1`, which are off by default and which the setup instructions never tell anyone to enable.

3. **SOC 2 Type II or equivalent** within the first year. I do not need it to sign up, but I need to know it is coming. "We are pursuing SOC 2" is sufficient for month 1.

4. **Data retention policy.** 90 days of detailed data, 1 year of aggregates. I can live with that.

5. **Data deletion on account cancellation.** Standard stuff.

6. **The ability to see exactly what is being sent.** The `OTEL_LOGS_EXPORTER=console` fallback lets developers inspect the payload. Transparency is the trust mechanism, not contracts.

### What would make me uncomfortable

- If the hosted endpoint required `OTEL_LOG_USER_PROMPTS=1` to function. It must not. Prompts must never flow to the hosted service.
- If the hosted service also offered a "conversation analysis" or "prompt quality scoring" feature that required content access. Even as an optional upsell. It signals the wrong priorities.
- If the hosted service were acquired by Anthropic (conflict of interest — they would know our usage patterns) or by a competitor.
- If the data were stored in a jurisdiction with weak privacy protections.

### Honest assessment of trust

I am more comfortable sending aggregate metrics to a hosted endpoint than I am running my own Grafana. My Grafana would be misconfigured, under-secured, and eventually forgotten. A hosted service with a real privacy policy and an economic incentive to not leak my data is a better security posture than my DIY alternative.

---

## Shadow IT: Would This Work for Enterprise Teams Skipping the Process?

Yes, and this might be the most interesting market segment.

Picture a VP of Engineering at a Fortune 500. They have 40 developers on Claude. The official process for deploying Thermal Enterprise would take 10 weeks (the enterprise buyer doc says 8-10 weeks for daemon approval). The VP does not have 10 weeks. The CFO asked about Claude spend last Tuesday.

The hosted dashboard solves this: 4 env vars per developer, no daemon, no MDM, no infosec review. Claude Code emits its own telemetry to Thermal's hosted endpoint. No new software on any machine. The "infosec" question is: "is it OK to send aggregate usage metrics to a SaaS monitoring tool?" This is the same question they answer for Datadog, New Relic, and every other monitoring SaaS. The answer is usually yes, especially when no code or prompts are involved.

The shadow IT path:
1. VP signs up with a corporate credit card.
2. Emails the team: "set these env vars."
3. Opens the dashboard the next day.
4. Answers the CFO's question.
5. When infosec eventually asks, says: "we send token counts and cost metrics. No code, no prompts. Here is the privacy policy."

This is exactly how Datadog, Slack, and every other successful B2B SaaS tool got into the enterprise: someone with a credit card and a problem bypassed the process.

---

## What the Hosted Product Needs to Get Right

### Onboarding (must be under 5 minutes total)

1. Sign up with email. No "schedule a demo" button. No "contact sales" form.
2. Name your organization. Get an API key and endpoint URL.
3. Copy the 4-line env var block. Paste to developers via Slack.
4. See first data within 10 minutes of a developer setting the env vars.

### Dashboard (must answer business questions, not show metrics)

Do not show me OTEL metric names. Do not show me Prometheus queries. Do not show me time-series graphs with y-axes labeled "claude_code.token.usage." Show me:

- "Your team spent $2,847 on Claude this week."
- "Jamie is your highest spender at $489 with the lowest cache efficiency at 4.2%."
- "3 developers have not used Claude this week."
- "Alex completed 12 PRs with $340 in Claude spend. That is $28 per PR."

Business language. Dollar amounts. Developer names. Actionable insights.

### Alerts (must be simple)

- "Jamie's daily spend exceeded $100 for the third day in a row."
- "Team weekly spend is 30% above last week."
- "2 developers have not used Claude in 5 days — are they blocked or not using their seats?"

Email or Slack webhook. Not PagerDuty. Not OpsGenie. I do not have those.

### Pricing (must be self-serve)

Free tier: 5 developers, 7 days of data retention. Enough for me to try it.
Paid tier: $5-8 per developer per month. Monthly billing. Credit card. Cancel anytime.

No annual commitment to start. Annual discount available for people who want it.

---

## What I Would Tell Todd

You have two products here. You are trying to make one of them.

**Product 1: Thermal Enterprise.** Self-hosted OTEL export for Fortune 500 platform teams with existing observability infrastructure. Bring your own Grafana. $8-15/dev/month. MDM deployment. 10-week sales cycle. Compliance documentation. This is the product the spec describes.

**Product 2: Thermal Cloud.** Hosted dashboard for startups and mid-market teams with zero observability infrastructure. Sign up, set env vars, see dashboards. $5-8/dev/month. Self-serve. Credit card. Same day. This is the product that does not exist yet but would sell faster.

Product 1 and Product 2 share the same backend: an OTEL collector that receives Claude Code telemetry, a data store, and a dashboard. The difference is who runs the infrastructure. For Product 1, the customer does. For Product 2, Thermal does.

My bet: Product 2 has a larger addressable market. There are maybe 500 Fortune 500 companies with 200+ Claude developers and existing Grafana. There are tens of thousands of startups and mid-market companies with 5-50 Claude developers and no observability stack. The enterprise buyer in the spec will pay more per seat, but there are dramatically more buyers at the smaller end.

The progressive value ladder still works for Product 2. It just replaces the infrastructure rungs:

| Enterprise rung | Startup equivalent |
|---|---|
| Rung 0: Free Thermal | Same |
| Rung 1: Cost statusline | Same |
| Rung 2: Push env vars via MDM to existing collector | Set env vars manually, point at hosted endpoint |
| Rung 3: Deploy daemon via MDM | Maybe later. Or never. The hosted dashboard works without it. |
| Rung 4: Enterprise rates + labels | Labels via hosted dashboard settings page. No config files. |
| Rung 5: Full integration | The hosted dashboard IS the full integration. |

The critical difference: for startups, the ladder has fewer rungs and each rung is lower friction. You do not need to build a daemon, MDM integration, or compliance packages to serve us. You need a hosted OTEL collector, a web dashboard, and Stripe.

Build Product 2 first. It validates the data model, builds revenue, and creates the customer base. Product 1 is the upsell for customers who outgrow the hosted service and want to bring everything in-house.

---

## Summary

| Question | Answer |
|----------|--------|
| Would I pay? | Yes, $5-8/dev/month for a hosted dashboard. Today. |
| What data would I send? | Token counts, cost, model, session ID, user ID, duration. Numbers only. |
| What data would I refuse to send? | Prompts, code, file paths, command strings, tool outputs. Content of any kind. |
| How do I get 18 devs to configure it? | Slack message with 4 env vars. Copy-paste into .zshrc. No binary installation. |
| What does onboarding look like? | Sign up, get API key + endpoint, share with team. Under 5 minutes. |
| What dashboard views do I want? | Per-developer cost, cache efficiency, usage trends, spend alerts. Business language, not OTEL jargon. |
| How does this compare to Anthropic billing? | Anthropic billing shows nothing useful for Max/Pro. This is infinite improvement over zero. |
| Would I trust Thermal with this data? | Yes, with a clear privacy policy and the understanding that prompts/code never flow to the service. |
| Shadow IT for enterprise teams? | Absolutely. This is how every successful SaaS tool enters the enterprise. |
| What would make me say no? | If setup requires more than copy-pasting env vars. If it requires running infrastructure. If it costs more than $10/dev/month. If it requires a sales call. |
