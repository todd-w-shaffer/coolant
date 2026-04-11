# Thermal Enterprise: Sales Engineering Assessment

## 1. What enterprises actually need

Engineering leadership buying AI coding tools at scale asks three questions in this order:

**"What does it cost?"** Not the license -- the compute. A VP of Engineering running 200 developers on Claude Code needs per-developer, per-project, and per-team cost attribution. Today that data is buried in API billing dashboards that show aggregate spend with no breakdown by who triggered what. Thermal already tracks agent lifecycles (agent.start/agent.stop events with session and agent IDs), process trees per session, and gate suppressions. The JSONL event bus has the lineage data -- it just needs to emit it as OTEL metrics with the right dimensional labels.

**"Is it actually helping?"** Cache hit ratio is the single most legible efficiency metric for Claude Code. A team with 90% cache hits is using Claude well -- their prompts are structured, their context is warm, their workflows are tight. A team at 40% is burning money on cold starts. Thermal can surface this if the collector gains access to cache metadata (either from Claude's own telemetry or from intercepting response headers). Build suppression ROI is the other concrete number: "Thermal prevented 847 redundant tsc/eslint/vitest invocations this week, saving approximately 12 hours of CPU time across your fleet." Gate events already carry this data.

**"What's the blast radius?"** CTOs want a meltdown map. Which teams are hitting swap? Which projects spawn the most agents? Are there runaway sessions? Thermal's threat classification (COOL/WARM/HOT/MELTDOWN) is already a severity model. Aggregated across a fleet, it becomes an incident heat map. A Grafana panel showing "3 of 47 developers currently in MELTDOWN" is the kind of thing that gets Thermal embedded in platform team runbooks.

The metrics that matter, in OTEL terms:
- `thermal.agent.count` (gauge, per-machine, per-session)
- `thermal.agent.duration` (histogram, start-to-stop lifecycle)
- `thermal.gate.suppressions` (counter, by tool name)
- `thermal.gate.cap_rewrites` (counter, by tool name)
- `thermal.threat.level` (gauge, 0-3 mapping to COOL-MELTDOWN)
- `thermal.system.cpu_percent`, `thermal.system.mem_percent`, `thermal.system.swap_bytes` (gauges)
- `thermal.system.decompressions` (counter, memory pressure proxy)

All labeled with `machine_id`, `user`, `team` (from config), `project` (from git remote or working directory).

## 2. Adoption blockers

Enterprise security teams will ask five questions about any new developer tool:

**What data leaves the machine?** Thermal's answer is the best possible one: nothing, by default. The dashboard is a local binary reading local process tables and a local JSONL file. OTEL export is opt-in, configured by the customer's own platform team, pointed at their own collector. No Thermal cloud. No phone-home. No telemetry to us. This is a massive advantage over every SaaS monitoring tool.

**What does it execute?** Thermal is a read-only observer. It reads /proc (or macOS equivalents via sysctl/vm_stat/ps), reads a JSONL file, and renders braille characters. The bash hooks make gate decisions (allow/deny/rewrite) but those are Claude Code plugin hooks with a well-defined contract. No root access, no kernel modules, no agents phoning home.

**Network access?** The only network call in the current codebase is a TCP connectivity check to api.anthropic.com (collector/network.go). OTEL export would add outbound to the customer's own collector endpoint. Both are auditable, configurable, and blockable by network policy.

**SOC2/compliance?** Thermal processes no customer data. It watches process names, PIDs, CPU percentages, and memory counters. It does not read file contents, source code, prompts, or responses. The JSONL events contain tool names and commands (e.g., "vitest --reporter=verbose") but not their output. For HIPAA-adjacent environments, the fact that zero PHI touches Thermal's data path is a clean story.

**IT deployment?** Single static binary, no runtime dependencies (Go compiles everything in), no install footprint beyond the binary and a TOML config file. Can be distributed via MDM, Homebrew tap, or internal artifact registry. The install script already handles arch detection. For fleet deployment, the platform team sets a shared TOML config with OTEL endpoint and team labels, drops the binary in PATH, done.

## 3. Tier logic

**Free (Thermal Core):**
- Terminal dashboard (the beautiful tmux strip that sells itself)
- All themes and animation profiles
- Local JSONL event logging
- Gate system (test capping, build suppression)
- Single-machine monitoring
- This stays free forever. It is the product-led growth engine.

**Enterprise (Thermal Fleet):**
- OTEL metric export (configurable endpoint, labels, interval)
- Pre-built Grafana dashboards (JSON shipped in-repo)
- Fleet-wide labels (team, project, environment) in config
- Alerting rule templates (Prometheus/Alertmanager YAML)
- Historical event export (JSONL to S3/GCS for audit trails)
- Priority support channel

**Not in Enterprise (avoid scope creep):**
- Centralized config management -- let customers use their existing config management (Puppet, Ansible, MDM). Do not build a control plane.
- Team dashboards hosted by us -- customers use their own Grafana. We ship the JSON, they own the infrastructure.
- User management or auth -- there are no accounts, no login, no SaaS surface area.

The key insight: Enterprise is a data export tier, not a feature-gate tier. The local experience is identical. Enterprise customers pay for the bridge to their existing observability stack. This means zero user-facing degradation of the free tier, which keeps the PLG flywheel spinning.

## 4. The demo flow

**Beat 1: Peripheral vision (0:00-5:00)**
Todd is advising the CTO's team on Claude Code workflows. He's live-coding something real. Thermal is running in his tmux bottom strip, doing its thing -- braille sparklines scrolling, agent dots breathing, threat level sitting at COOL. He does not mention it. The platform engineers in the room notice. Someone will ask.

**Beat 2: The question (5:00-5:30)**
"What's that thing at the bottom of your terminal?" Todd glances down. "Oh -- Thermal. I run it on every machine. It watches what Claude's doing so I don't have to." Brief pause. Back to the main topic. Let them want more.

**Beat 3: The casual demo (8:00-10:00)**
Natural pause in the conversation. Todd spawns three parallel agents on a real task. The dashboard lights up -- agent dots appear, CPU sparkline climbs, threat level shifts to WARM. "See, three agents. This is where most people's laptops start struggling." He points at the gate suppression count ticking up. "It just killed four redundant tsc invocations. My machine stays usable."

**Beat 4: The pivot (10:00-12:00)**
"Now imagine this across your 200-person org. Right now you have no idea which teams are burning API credits on cold cache starts, which projects spawn runaway agents, or whether your developers' machines are melting." Pause. "Thermal has an enterprise tier. Flip one config flag and it exports everything you're seeing here -- agent counts, threat levels, gate suppressions, resource usage -- to your existing Prometheus/Grafana stack."

**Beat 5: The screenshot (12:00-14:00)**
Todd pulls up a pre-built Grafana dashboard on his laptop (or a screenshot if offline). Three panels: fleet heat map (machines by threat level), cost attribution (agents per team per day), gate ROI (suppressions saved this week). "This is what it looks like when your platform team can actually see what 200 Claude agents are doing to your infrastructure."

**Beat 6: The close (14:00-15:00)**
"The dashboard itself is free. Open source, MIT license. Your developers can install it today. The fleet export is what you'd license." Hand them the repo URL. The tool sells itself from here.

## 5. Pricing model considerations

**Per-seat, annual, tiered.** This is the proven model for developer tools at enterprise scale.

Precedent pricing:
- GitHub Copilot Business: $19/user/month
- Cursor Business: $40/user/month (higher because it replaces the editor)
- Datadog APM: $31/host/month (infrastructure monitoring precedent)

Thermal Enterprise should price below the AI coding tool itself but above commodity monitoring. Suggested range: **$8-15/developer/month**, billed annually. This is low enough that it disappears into the Claude Code budget line item (which is already $50-200/developer/month in API costs) and high enough to be a real business.

Per-machine pricing is tempting but wrong -- developers use multiple machines, and IT departments hate counting hardware. Per-seat with unlimited machines per seat is cleaner.

Usage-based pricing (per-metric, per-event) is the Datadog model and enterprises hate the unpredictability. Avoid it. Flat per-seat is what procurement teams know how to approve.

Volume tiers: 1-50 seats (standard), 51-200 (10% discount), 200+ (custom/negotiated). Standard enterprise playbook.

## 6. Competitive landscape

The honest answer: there is almost nothing here.

**Existing AI coding tool monitoring:**
- GitHub Copilot has usage analytics in the admin console, but it is shallow (acceptance rates, suggestions per user) and locked to Copilot.
- Cursor has no fleet monitoring.
- Claude Code has no built-in observability beyond the conversation log.
- Generic APM tools (Datadog, New Relic) can monitor the processes but have no semantic understanding of what an "agent" is, what a "gate suppression" means, or what threat levels map to.

**The moat is threefold:**

First, Thermal understands the domain. It knows what a vitest invocation is, it knows agent start/stop lifecycles, it knows what swap pressure means for a Claude session. Generic monitoring tools see processes; Thermal sees intent.

Second, the advisor channel. Todd is in the room when enterprises decide how to adopt Claude. The tool is already running. There is no cold outbound, no POC request, no vendor evaluation -- the buyer has already seen the product working in production (Todd's production). This is the most efficient go-to-market motion possible for developer tooling.

Third, the free tier creates lock-in before the enterprise conversation starts. Developers install it because it looks incredible and actually helps. By the time the platform team evaluates fleet monitoring options, Thermal is already on every machine. The enterprise tier is an unlock, not an install.

**Potential future competitors:** Anthropic themselves could build this into Claude Code's admin console. This is the existential risk. Mitigation: ship fast, build the Grafana ecosystem, make Thermal the standard before Anthropic prioritizes it. Being open source and infrastructure-agnostic (works with any OTEL backend) is also a hedge -- if Anthropic builds a walled garden, Thermal is the open alternative.

## 7. Pre-built Grafana dashboards

Ship three dashboard JSONs in `thermal/grafana/`:

### Dashboard 1: Fleet Overview
Single pane of glass for the platform team.

- **Row 1:** Stat panels -- total active agents (fleet-wide), machines in MELTDOWN (red), machines in HOT (amber), total gate suppressions today
- **Row 2:** Heat map -- machines as cells, colored by threat level, sorted by severity. Click through to per-machine detail.
- **Row 3:** Time series -- fleet-wide agent count over 24h, fleet-wide CPU p50/p95, fleet-wide memory p50/p95
- **Row 4:** Table -- top 10 machines by agent count, with user/team/project labels

### Dashboard 2: Cost and Efficiency
For engineering managers tracking ROI.

- **Row 1:** Stat panels -- total agent-hours this week, estimated API cost (agent-hours * configurable rate), gate suppressions (count + estimated time saved)
- **Row 2:** Bar chart -- agent-hours by team (stacked by project), week-over-week comparison
- **Row 3:** Time series -- gate suppressions by tool type (tsc, eslint, vitest, cargo build), showing which tools get suppressed most
- **Row 4:** Table -- per-developer breakdown: agent count, agent-hours, suppressions, threat-level distribution (% time in each state)

### Dashboard 3: Machine Deep Dive
Drill-down for individual developer machines (linked from Fleet Overview).

- **Row 1:** Current state -- threat level gauge, active agent count, CPU/MEM/SWAP gauges (mirroring the terminal dashboard)
- **Row 2:** Time series -- CPU, memory, swap, decompressions over 24h with threat-level bands as colored regions
- **Row 3:** Event log -- scrolling table of gate events (suppressions, cap rewrites, agent start/stop) with timestamps
- **Row 4:** Agent timeline -- Gantt-style visualization of agent lifecycles (start to stop), colored by threat level during their runtime

Each dashboard ships as a provisioned JSON file with templated datasource variables so customers point it at their Prometheus instance and it works immediately. Include a README with setup instructions for common stacks (Prometheus + Grafana, Grafana Cloud, Datadog with OTLP intake).
