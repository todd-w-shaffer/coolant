# Perplexity Research Prompt: Token/Cost Data Sources

**Purpose:** Map every possible source of per-request token usage and cost
data across all Claude deployment surfaces. Results feed back into
[cost-attribution.md](cost-attribution.md) and the main spec.

---

I'm building an observability tool that monitors Claude Code (Anthropic's CLI coding agent) at the OS level — process trees, agent lifecycles, resource usage. I need to find every possible source of per-request token usage and cost data that a third-party tool running on the same machine could access.

**Critical context:** Claude Code already supports OTEL telemetry export via standard `OTEL_*` environment variables (e.g., `OTEL_LOGS_EXPORTER=console`). I deploy this at enterprise customer sites today as part of my advisory practice. I need to understand exactly what Claude Code emits so I can determine whether token/cost data is already available without building new infrastructure.

**Two customer profiles:**
- **Enterprise:** Claude for Enterprise (managed org, SSO, admin console, usage policies). These are Fortune 500 platform teams with 200+ developers.
- **Startup/SMB:** Stacked Claude Max Pro plans (individual seats, no centralized admin). These are 5-50 person teams where each developer has their own subscription.

For each source below, note whether it's available to both profiles or only one, and whether it requires org-admin access vs. individual developer access.

**Claude Code OTEL (highest priority):**
- What OTEL signals does Claude Code emit — traces, metrics, logs, or some combination?
- What `OTEL_*` environment variables does Claude Code respect?
- Do the emitted signals include token counts (input_tokens, output_tokens, cache_read_input_tokens, cache_creation_input_tokens)?
- Do they include model name, session ID, agent ID, or conversation ID?
- What is the schema of the spans/metrics/log records? What attributes are attached?
- Is this documented anywhere (Anthropic docs, GitHub, changelog)?
- Does the telemetry differ between Enterprise and Max/Pro plans?
- What is the export latency — per-request, batched, end-of-session?

**Claude Code local artifacts:**
- Does Claude Code write session logs, telemetry files, or usage summaries to disk?
- What hook types exist (PreToolUse, PostToolUse, SessionStart, etc.) and what data do they receive on stdin?
- Does the MCP server protocol expose usage metadata?
- Does the Claude for Enterprise admin console expose per-developer usage data or APIs?
- For Max plan users: is there any local or API access to usage/token counts, or is it a pure "unlimited" black box?

**Claude API (direct):**
- What response fields contain token counts (input, output, cache read, cache creation)?
- Are there response headers with usage data?
- Does the API support streaming usage chunks, or only final usage in the complete response?
- Is there a usage/billing API endpoint that returns per-request or per-session breakdowns?
- Does Claude for Enterprise expose additional usage APIs or admin endpoints that individual plans don't?

**Amazon Bedrock (Claude):**
- What does the Bedrock InvokeModel / Converse response include for token usage?
- Does Bedrock CloudWatch emit per-invocation token metrics? What metric names?
- Is there a Bedrock usage API or cost explorer breakdown by model/request?
- Can Bedrock usage be tagged per-developer or per-team via request metadata?
- Authoritative Source: https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-claude.html

**Google Vertex AI (Claude):**
- What does the Vertex Predict / GenerateContent response include for token usage?
- Does Vertex export per-request metrics to Cloud Monitoring? What metric names?
- Is there a Vertex usage API with per-call granularity?
- Can Vertex usage be labeled per-developer or per-team?
- Authoritative Source: https://docs.cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude

**Azure AI Foundry (Claude):**
- What does Azure AI Foundry's response include for token usage when calling Claude models?
- Does Azure Monitor / Application Insights emit per-invocation token metrics?
- Can usage be tagged per-developer or per-team via Azure resource tags or request metadata?
- What's the billing granularity — per-request, per-hour, per-day?
- Authoritative Source: https://learn.microsoft.com/en-us/azure/foundry/foundry-models/how-to/use-foundry-models-claude?tabs=python

**Anthropic Admin API / Console:**
- Does Anthropic expose an admin API for Enterprise orgs with usage breakdowns?
- What granularity — per-user, per-workspace, per-session, per-request?
- Is this accessible programmatically or only via web console?

For each source: what fields are available, what's the access pattern (file tail, API call, webhook, metric scrape), the latency (real-time, batched, end-of-session), and which customer profile (Enterprise, Max/Pro, or both) has access.

**Source quality instructions:**

Prioritize authoritative sources: Anthropic's official documentation (docs.anthropic.com), AWS Bedrock documentation, Google Cloud Vertex AI documentation, Azure AI Foundry documentation, official API references, and official changelogs/release notes.

Secondary sources — blog posts, GitHub issues/discussions, community forums, developer tweets, Stack Overflow answers — are valuable for discovering undocumented behavior or upcoming features, but flag them explicitly as secondary. For each secondary source, include the publication date. A secondary source from 2024 or earlier about Claude Code's capabilities is likely stale — Claude Code's OTEL support, hook system, and telemetry have evolved rapidly. Weight recent authoritative docs (2025-2026) far above older community reports.

Where authoritative documentation is silent on a topic (e.g., Claude Code's exact OTEL schema), say so explicitly rather than filling the gap with secondary speculation. "Not documented as of [date]" is more useful than an answer stitched from year-old blog posts.
