# Thermal Enterprise: Buyer Evaluation

**Author:** VP of Platform Engineering (Fortune 500, 300 Claude Code developers)
**Date:** April 2026
**Context:** Existing stack is Grafana Alloy (OTEL collector), Prometheus, Grafana. MDM via Jamf. Mixed Bedrock + direct Anthropic API usage. Negotiated volume discount with Anthropic.

---

## Bottom Line Up Front

This product solves a real problem we have today. We have 300 developers using Claude Code and no unified view of what they are doing to our machines or our Anthropic bill. The progressive adoption model is the most compelling part of the pitch — we can start getting value next week without any daemon approval, and each subsequent step is independently justifiable. The architecture is sound, the security posture is thoughtful, and the fact that Claude Code already emits rich OTEL telemetry means the hardest infrastructure problem is already solved by Anthropic.

I would start deployment this quarter. Not because the product is perfect — it is not — but because the alternative is continued blindness into a line item that is growing 40% quarter-over-quarter.

---

## Deployment Friction Assessment

### What my team actually has to do

This is the central question. I walked through each rung and counted config touchpoints, approval gates, and calendar time.

**Rung 0 (free dashboard, individual install):** Zero platform team involvement. A developer runs an install script. This is happening already — three people on the devex team installed it after the demo. No MDM, no approval, no config management. The gate system (test capping) is the immediate win: developers stop accidentally fork-bombing their machines with parallel vitest runs.

**Rung 1 (cost statusline):** One config line per developer. Still zero platform involvement. The developer sees per-session spend in their terminal. This is a personal productivity tool at this rung.

**Rung 2 (Claude Code OTEL fleet-wide):** This is the first platform team touchpoint and the most important rung in the sequence. Two env vars pushed via Jamf:

```
CLAUDE_CODE_ENABLE_TELEMETRY=1
OTEL_LOGS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=https://otel-collector.corp.example.com:4318
```

This sends Claude Code's native telemetry — per-request token counts, cost, model, session ID, user ID — to our existing Grafana Alloy. No new binary on any machine. No daemon. No Thermal Enterprise license. Just env vars we already know how to push, pointing at a collector we already operate.

**Config touchpoints:** 3 env vars via Jamf profile. One Alloy receiver config (OTLP HTTP, which Alloy already supports). One Grafana dashboard import.

**Rung 3 (Thermal daemon fleet-wide):** This is where friction spikes. A new binary on 300 machines. Jamf package, launchd plist, config file distribution, monitoring for the daemon itself. This is a standard MDM deployment — we do dozens of these — but it requires the full approval chain.

**Rung 4 (enterprise rates + fleet labels):** Config file update to the already-deployed daemon. Low friction once Rung 3 is done.

**Rung 5 (full integration):** Dashboard imports, alerting rules, compliance documentation review. This is operational polish, not a deployment event.

### Time-to-value estimate

| Rung | Calendar time from "yes" to "data flowing" | Blocking dependency |
|------|---------------------------------------------|---------------------|
| 0 | Already done (3 devs have it) | None |
| 1 | Same day | None |
| 2 | 1-2 weeks | Jamf profile push (routine), Alloy config PR |
| 3 | 8-10 weeks | Infosec daemon approval (4 weeks) + Jamf packaging + change window (6 weeks, overlapping) |
| 4 | +1 day after Rung 3 | Config file update |
| 5 | +1-2 weeks after Rung 3 | Dashboard/alerting review |

**The critical insight:** Rung 2 gets us 80% of the token/cost visibility we need with zero new software on developer machines. The Perplexity report confirms that Claude Code's `api_request` log events carry per-request `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`, and `model` — all with `user.account_id` for per-developer attribution. That is the data we are missing today. We can have it flowing to Grafana in two weeks.

---

## Realistic Adoption Timeline for Our Org

### Week 1-2: Rung 2 (Claude Code OTEL)

**Who:** Platform team (me + 1 SRE), Jamf admin.
**What:** Push three env vars via Jamf managed settings. Add OTLP receiver to Alloy config. Build an initial Grafana dashboard showing per-team, per-developer token usage and cost.
**Approval needed:** Jamf profile change (routine, 2-day turnaround). No infosec review — we are configuring an Anthropic-built feature using standard OTEL, sending data to our own collector. This is telemetry configuration, not new software.
**Value delivered:** "We can see what 300 developers are spending on Claude, broken down by team, model, and cache efficiency." This alone justifies starting.

### Week 2-4: Let Rung 0/1 spread organically

**Who:** Devex team evangelizes to developers.
**What:** Developers who want the terminal dashboard install it themselves. Those who want the cost statusline enable it. No platform team involvement.
**Approval needed:** None. It is an open-source developer tool.
**Value delivered:** Developer self-service. The gate system reduces machine load during multi-agent work. Developers who see their per-session spend start self-regulating.

### Week 3: File infosec review for Rung 3

**Who:** Infosec lead, my team.
**What:** Submit daemon review package. The compliance documentation that ships with Thermal Enterprise is the answer to half their questions before they ask.
**Questions they will ask (see next section).**

### Week 4-6: Build Grafana dashboards on Rung 2 data

**Who:** Platform team + engineering managers.
**What:** Now that token data is flowing, build the dashboards that matter: per-team cost attribution, cache efficiency leaderboard, anomaly detection for runaway sessions. We do not need Thermal Enterprise for this — Claude Code's OTEL gives us the raw data, we build our own dashboards.
**Value delivered:** Engineering leadership gets the cost visibility they have been asking for since Q1.

### Week 7: Infosec approval (optimistic) or Week 9 (realistic)

**What:** Daemon approved with conditions. Typical conditions: binary signature verification, config file integrity via hash, endpoint allowlisting, monitoring requirements.

### Week 8-10: Rung 3 deployment via Jamf

**Who:** Jamf admin, platform team.
**What:** Package the enterprise binary, distribute launchd plist, push config via Jamf. The daemon starts reporting system-level context (CPU, memory, threat level) and gate intelligence (suppression counts, time saved) alongside the Claude Code OTEL data we have been collecting since Week 2.
**Value delivered:** The Grafana dashboards now show not just "how much are we spending" but "what is it doing to our machines" and "how much waste is Thermal preventing."

### Week 10-12: Rungs 4 and 5

**Who:** Platform team, finance.
**What:** Configure our negotiated Anthropic rates in the daemon config. Add fleet labels (team, project, cost center). Import the pre-built Grafana dashboards. Set up alerting rules.
**Value delivered:** Full fleet observability. Finance gets cost attribution that matches our actual contract rates.

### Where the process stalls

**The 4-6 week gap between Rung 2 and Rung 3.** This is the daemon approval window. During this gap, we have token/cost data flowing (from Claude Code OTEL) but no system context or gate intelligence. This gap is tolerable because Rung 2 data is independently valuable — it answers the cost question, which is the most urgent one.

**If we could only get to Rung 2 this quarter:** Yes, it is enough to justify starting. Per-developer, per-team token cost attribution in our existing Grafana is worth the two weeks of platform team effort regardless of whether we ever deploy the daemon. The daemon adds system context and gate ROI — nice to have, not table stakes.

---

## Infosec Questions the Spec Does Not Answer

The spec's security section is above average. The closed attribute set, the architectural exclusion of command strings, the transport security model, and the endpoint allowlisting are all things my infosec team will approve of. But they will ask:

### 1. Binary provenance and supply chain

**Question:** "How is the enterprise binary built, signed, and distributed? Is there reproducible build verification? What is the SBOM?"

**Gap in spec:** The spec mentions pre-built binaries on private GitHub Releases but does not discuss code signing, notarization (macOS Gatekeeper), reproducible builds, or SBOM generation. For an MDM-distributed binary running on 300 developer machines, my infosec team will require at minimum macOS notarization and ideally a signed SBOM.

### 2. Daemon privilege model

**Question:** "What permissions does the daemon require? Does it need root? Full disk access? Accessibility permissions? What syscalls does it make?"

**Gap in spec:** The spec documents what data the daemon collects (CPU via cgo mach calls, memory via sysctl/vm_stat, processes via ps, GPU via ioreg) but does not specify the privilege model. The cgo mach host_statistics call presumably runs unprivileged, but infosec will want this stated explicitly. The process discovery via `ps -Ao` is unprivileged. The ioreg call for GPU is unprivileged. If all of this runs as the logged-in user with no elevated permissions, say so — that is the single most important sentence for infosec approval.

### 3. Data at rest

**Question:** "The daemon collects system metrics and sends them to our OTEL collector. What happens to that data between collection and export? Is anything written to disk? Is the JSONL event log readable by other processes?"

**Gap in spec:** The spec says the PeriodicReader drops batches if the endpoint is unreachable — good, no unbounded disk buffering. But the JSONL event log (`$TMPDIR/coolant-$USER.events.jsonl`) is written by the bash hooks and tailed by the daemon. This file contains gate events. Infosec will want to know the file permissions, retention policy, and whether it contains anything beyond what the spec's closed attribute set allows.

### 4. Update mechanism

**Question:** "When a new version of the daemon ships, how does the update get to 300 machines? Is there auto-update? If not, what is the MDM package update cadence?"

**Gap in spec:** No update mechanism documented. For an MDM-deployed binary, the answer is probably "Jamf pushes the new package" — but this should be stated. Auto-update from the daemon itself would be a security concern (binary self-modification). Explicit statement that the daemon never self-updates would satisfy infosec.

### 5. Crash behavior and resource limits

**Question:** "If the daemon crashes, what happens? Does it restart automatically? Does it have memory limits? CPU limits? What prevents it from becoming the problem it is monitoring?"

**Gap in spec:** The performance budget section covers steady-state (~20KB memory, ~2-3us per tick) but does not address crash recovery, resource ceilings, or runaway protection. A launchd plist with `KeepAlive`, `ThrottleInterval`, and resource limits would answer this. The spec should include a reference launchd plist.

### 6. Network egress scope

**Question:** "The daemon connects to our OTEL collector. Does it connect to anything else? Anthropic servers? GitHub? Any telemetry about Thermal itself?"

**Gap in spec:** The spec should explicitly state that the daemon makes exactly one outbound connection (to the configured OTLP endpoint) and nothing else. No phone-home, no update checks, no analytics about Thermal usage. This is table stakes for enterprise daemon approval.

---

## Overlap with Existing Tools

### What we already have

We run Datadog agents on all developer machines for infrastructure monitoring. Datadog gives us CPU, memory, disk, network, and process-level metrics. We also have Prometheus + Grafana for application-tier observability.

### What overlaps

System metrics (CPU, memory, swap) overlap 100% with Datadog. We do not need Thermal to tell us a machine is at 94% memory utilization — Datadog already does that.

### What is genuinely new

1. **Agent lifecycle semantics.** Datadog sees processes. Thermal sees Claude Code agents — spawn, death, fresh vs. stale, active count, duration. Datadog cannot tell us "Developer X had 7 concurrent Claude agents running for 3 hours." Thermal can.

2. **Gate suppression intelligence.** Nothing in our stack knows that Claude Code just tried to run `tsc` for the 47th time and Thermal killed it. This is unique to Thermal and the ROI number ("saved 12 hours of CPU time this week") is genuinely compelling.

3. **Threat-level semantics.** Datadog can alert on high CPU. Thermal can alert on "MELTDOWN — this machine has too many Claude agents relative to available resources." The semantic layer matters for actionable alerts.

4. **Cost correlation.** Thermal's agent-hours metric, correlated with system resource utilization, gives us "this team's Claude usage is costing us X in infrastructure impact." Datadog cannot correlate process groups with AI agent intent.

### What we would get from Claude Code OTEL alone (Rung 2, no Thermal)

Per-request token counts, cost, model, session ID, user ID, cache efficiency. This is the core cost attribution data. We do not need Thermal for this — Claude Code emits it natively. Thermal adds the system context layer on top.

### Honest assessment

If cost attribution is the primary goal, Rung 2 (Claude Code OTEL) gets us 80% of the way. Thermal Enterprise (Rung 3+) adds the remaining 20%: system context, gate ROI, and the pre-built dashboards. Whether that 20% justifies the per-seat license depends on price. At $8/dev/month for 300 developers, that is $28,800/year. The gate suppression ROI number needs to credibly save more than that in developer productivity and machine wear.

---

## Monitoring the Monitor

### What if the daemon crashes?

The spec does not address this. Our expectation:

- **launchd KeepAlive:** The daemon should ship with a reference launchd plist that restarts on crash with throttling. Standard macOS daemon pattern.
- **Heartbeat metric:** Thermal should emit a `thermal.daemon.uptime` gauge. Absence of this metric in Prometheus for >2 minutes triggers an alert. This is how we monitor every other daemon.
- **Graceful degradation:** If the daemon dies, Claude Code continues working. The gate system (bash hooks) operates independently of the daemon. Developers lose the terminal dashboard and fleet telemetry, but their work is uninterrupted. This is correct — the monitor should never be in the critical path.

### Upgrade path

For 300 machines via Jamf: package the new binary, push via Jamf policy, launchd restarts the daemon. Standard MDM binary update. We do this for Datadog agents monthly. Not a concern if the daemon is stateless (which it appears to be — all state is derived from live system observation and the JSONL event bus).

### Resource consumption monitoring

We would add the Thermal daemon to our Datadog process monitoring. If `thermo` exceeds 100MB RSS or 5% CPU sustained, we get alerted. The spec's claimed ~20KB steady-state is excellent if accurate — we will verify in staging before fleet deployment.

---

## Developer Trust

### "We're installing a background process that reads your Claude session data"

This is the single biggest adoption risk. Our developers are sophisticated. They will ask:

1. **What data does it collect?** The closed attribute set in the spec is the right answer. No command strings, no file paths, no prompt content. System metrics, agent counts, gate events (tool name only, not the command). This is defensible.

2. **Where does it send the data?** To our own OTEL collector, which they can verify via `--otel-status`. The endpoint allowlisting via MDM means we control the destination. This is also defensible.

3. **Can I see what it sends?** The `--otel-status` diagnostic and the `OTEL_LOGS_EXPORTER=console` fallback let a developer inspect the exact payload. Transparency tools exist.

4. **Can I turn it off?** The `COOLANT_OTEL=0` kill switch lets a developer disable export. Whether we allow this as a policy decision is our call, not Thermal's. The architecture supports both "mandatory" and "opt-out" deployment models.

### The cost statusline question

"Is this something you want to roll out to all devs, or is it a management tool that developers would resent?"

Both. Here is why it works:

- **For developers:** It is a personal finance dashboard. "I just spent $14 on that refactoring session" is useful self-knowledge. Developers who see their spend in real time make better decisions about when to use Claude and when to think for themselves. This is not surveillance — the data stays on their machine unless they enable OTEL.
- **For management:** The fleet-level view (Rung 2+) shows per-team spend. This is a management tool. But the developer-facing statusline is opt-in and local-only, which defuses the "big brother" objection.

**The correct rollout:** Make Rung 0/1 available to all developers as opt-in. Let the devex team evangelize it. Do not mandate the statusline — let developers discover it. Mandate the fleet-level OTEL (Rung 2) via MDM, which does not put anything visible on the developer's screen. The distinction between "we can see aggregate usage" (OTEL, invisible to developer) and "you can see your personal usage" (statusline, developer opt-in) is important for trust.

---

## The Bedrock Problem

This is the most significant gap in the current architecture.

### The situation

Roughly 40% of our developers (120 people) use Claude via Bedrock. The rest use direct Anthropic API. We negotiated rates with both AWS and Anthropic.

### What Claude Code OTEL does

The Perplexity report confirms: Claude Code's OTEL telemetry emits the same token/cost fields regardless of whether the backend is Bedrock or direct API. The `api_request` log event carries `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`, and `model` for both paths. This means Rung 2 gives us unified visibility across both backends.

### What the Analytics Admin API does not do

The Claude Code Analytics Admin API (`/v1/organizations/usage_report/claude_code`) explicitly excludes Bedrock usage. This means server-side daily aggregation is Anthropic-direct only. This is a gap for reconciliation but not a dealbreaker — we have the real-time OTEL data for both.

### What Thermal's daemon adds

Thermal's system-level metrics (CPU, memory, agent lifecycle, gate events) are backend-agnostic. The daemon does not know or care whether the Claude process is hitting Bedrock or Anthropic. This is correct — system impact is system impact regardless of billing path.

### Is the Bedrock gap a dealbreaker?

No. The critical data path (Claude Code OTEL -> our Alloy -> Grafana) covers both Bedrock and direct API. The Analytics Admin API gap means we cannot reconcile OTEL-reported costs against Anthropic's server-side billing for Bedrock users, but we can reconcile against AWS Cost Explorer for Bedrock and against the Anthropic Admin API for direct users. Two reconciliation paths is manageable.

### What would make it a dealbreaker

If Claude Code's OTEL telemetry did not emit token data for Bedrock requests. The Perplexity report says it does. We need to verify this in our staging environment before fleet deployment.

---

## What Would Make Me Say No

1. **The daemon requires elevated permissions.** If it needs root, Full Disk Access, or any TCC entitlement beyond basic user-level access, the infosec approval timeline doubles and developer trust evaporates. The spec must explicitly state it runs unprivileged.

2. **The daemon phones home.** Any outbound connection to Anthropic, GitHub, or any endpoint other than our configured OTEL collector is a non-starter. The spec should state this explicitly.

3. **The daemon is not notarized.** An un-notarized binary on 300 Macs via MDM means Gatekeeper exceptions fleet-wide. Infosec will reject this.

4. **The OTEL export path is unreliable.** If the daemon's OTEL export silently drops data at scale, we cannot trust the dashboards. We need export success/failure metrics from the daemon itself.

5. **The pricing exceeds the value of the delta over Rung 2.** If Claude Code OTEL (free, no daemon) gives us 80% of what we need, the enterprise tier must price the remaining 20% appropriately. At $15/dev/month ($54K/year for 300 devs), the gate suppression ROI and system context layer need to demonstrably save more than that. At $8/dev/month ($28.8K/year), it is an easier sell.

6. **No clear data on Claude Code OTEL reliability at fleet scale.** We would be one of the first orgs to run Claude Code OTEL at 300 developers. If the OTEL export from Claude Code itself is buggy or resource-hungry, the entire Rung 2+ strategy collapses. We need to pilot with 10 developers first.

---

## What Would Make Me Say "We Need This Yesterday"

1. **A developer incident.** When someone's machine melts because 12 Claude agents spawned during a parallel refactor and nobody knew until the machine locked up. The gate system prevents this. It has already happened twice informally.

2. **The CFO asks "how much are we spending on Claude per team."** Today we cannot answer this question. Rung 2 answers it in two weeks. Rung 3+ answers it with infrastructure correlation.

3. **Cache efficiency disparity.** The Perplexity report describes cache hit ratio as a team-health metric. If we discover that one team operates at 90% cache hits and another at 30%, the coaching opportunity is worth the entire investment. We need Rung 2 data to see this.

4. **Gate suppression ROI is real.** If the "Thermal prevented 847 redundant tsc invocations" number holds up across 300 developers, the aggregate time saved is massive. Our TypeScript monorepo has an 18-second tsc build. 847 suppressions * 18 seconds = 4.2 hours of CPU time saved per week per the affected team. Across the fleet, this could be hundreds of hours weekly.

5. **The demo sells itself.** Three platform engineers saw Todd's terminal dashboard during the advisory session and asked about it unprompted. The product has natural pull. This is rare for infrastructure tooling.

---

## Recommendations

### Immediate (this sprint)

1. **Deploy Rung 2.** Push Claude Code OTEL env vars via Jamf. Point at our existing Alloy. Build initial Grafana dashboard for per-team token cost. This is zero-risk, zero-new-software, and answers the cost question.

2. **Let Rung 0/1 spread organically.** Post the install link in the #developer-tools Slack channel. Let the devex team demo it at the next engineering all-hands.

### This quarter

3. **File infosec review for the Thermal daemon.** Include the compliance documentation package. Request answers to the six gaps identified above.

4. **Pilot Rung 3 with 10 developers** on the platform team. Verify resource consumption, export reliability, and daemon stability before fleet deployment.

5. **Verify Claude Code OTEL coverage for Bedrock.** Confirm that our Bedrock developers' token data appears in the OTEL stream with the same fidelity as direct API users.

### Next quarter

6. **Fleet deployment of Rung 3** (assuming infosec approval and successful pilot).

7. **Configure enterprise rates** matching our Anthropic and Bedrock contracts.

8. **Build alerting rules** for MELTDOWN-state machines, anomalous spend, and cache efficiency degradation.

### Things to negotiate with Todd before signing

- **Notarized binary** — non-negotiable for MDM deployment.
- **Explicit "no phone home" statement** in documentation and code.
- **Reference launchd plist** with resource limits and crash recovery.
- **Export reliability metrics** (`thermal.otel.exports.success`, `thermal.otel.exports.failed`) from the daemon.
- **Pilot pricing** for the first quarter while we validate the delta over Rung 2.
- **SLA on security patches** — if a vulnerability is found in the OTEL dependency chain, what is the response time?

---

## Final Assessment

The progressive value model is the right architecture for enterprise adoption. The fact that Rung 2 (Claude Code's own OTEL, no Thermal software) delivers the majority of cost visibility value means the buying decision for Thermal Enterprise is not "do we want cost visibility" (answer: obviously yes, deploy Rung 2 immediately) but "do we want system context, gate intelligence, and pre-built dashboards on top of cost visibility" (answer: probably yes, but let us prove it in a pilot first).

The product fills a genuine gap. Nothing else in our stack tells us about Claude agent semantics, gate suppression value, or AI-specific threat levels. The question is whether that gap is worth $28.8K-$54K/year, and the answer depends on the gate suppression ROI number being real at fleet scale.

**Decision: Start Rung 2 deployment this week. File infosec review for Rung 3. Pilot with 10 developers next month. Fleet decision by end of quarter based on pilot data.**
