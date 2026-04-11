# Claude Code Token & Cost Observability — Full Source Reference

*Compiled April 2026 · Authoritative sources prioritized; secondary sources dated and flagged.*

***

## Executive Summary

For a third-party observability tool running on the same machine, the richest real-time token source is the **Claude Code OTEL `api_request` log event**, which carries per-call `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `cost_usd`, `model`, and `duration_ms` — all available within the default 5-second log export interval, without any proxy or API interception. A fallback path exists through local JSONL session files in `~/.claude/projects/`, which contain the same token data and are readable by any process with filesystem access. For Enterprise orgs, the Claude Code Analytics Admin API adds daily-aggregated per-user model breakdowns with the same four token types.

***

## Source 1: Claude Code OTEL (Highest Priority)

### What Claude Code emits

Claude Code exports **three independent OpenTelemetry signals**:[^1][^2]

| Signal | Enable with | Default export interval |
|--------|-------------|------------------------|
| Metrics (counters) | `OTEL_METRICS_EXPORTER` | 60 seconds |
| Log events (structured records) | `OTEL_LOGS_EXPORTER` | 5 seconds |
| Traces / spans (beta) | `OTEL_TRACES_EXPORTER` + `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` | 5 seconds |

Claude Code does **not** emit traces by default. Metrics and log events are independent signal types — you can enable just logs, just metrics, or both. The gating variable for everything is `CLAUDE_CODE_ENABLE_TELEMETRY=1`.[^1]

### Environment variables

The complete set of documented configuration variables:[^1]

| Variable | Description | Default |
|----------|-------------|---------|
| `CLAUDE_CODE_ENABLE_TELEMETRY` | Master switch (required) | off |
| `OTEL_METRICS_EXPORTER` | `otlp`, `prometheus`, `console`, `none` | none |
| `OTEL_LOGS_EXPORTER` | `otlp`, `console`, `none` | none |
| `OTEL_TRACES_EXPORTER` | `otlp`, `console`, `none` (beta) | none |
| `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA` | Enables trace spans (required for traces) | off |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc`, `http/json`, `http/protobuf` | — |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Collector endpoint for all signals | — |
| `OTEL_EXPORTER_OTLP_HEADERS` | Auth headers | — |
| `OTEL_EXPORTER_OTLP_METRICS_PROTOCOL` | Per-signal protocol override | — |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | Per-signal endpoint override | — |
| `OTEL_EXPORTER_OTLP_LOGS_PROTOCOL` | Per-signal protocol override | — |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | Per-signal endpoint override | — |
| `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` | Per-signal protocol override (beta) | — |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Per-signal endpoint override (beta) | — |
| `OTEL_METRIC_EXPORT_INTERVAL` | Metrics batch interval in ms | 60000 |
| `OTEL_LOGS_EXPORT_INTERVAL` | Logs batch interval in ms | 5000 |
| `OTEL_TRACES_EXPORT_INTERVAL` | Traces batch interval in ms | 5000 |
| `OTEL_LOG_USER_PROMPTS` | Include prompt text in events | off (redacted) |
| `OTEL_LOG_TOOL_DETAILS` | Include bash cmds, tool args in tool_result events | off |
| `OTEL_LOG_TOOL_CONTENT` | Full tool I/O bodies on spans (60 KB truncation) | off |
| `OTEL_METRICS_INCLUDE_SESSION_ID` | Include `session.id` in metrics | true |
| `OTEL_METRICS_INCLUDE_VERSION` | Include `app.version` in metrics | false |
| `OTEL_METRICS_INCLUDE_ACCOUNT_UUID` | Include `user.account_uuid`/`user.account_id` | true |
| `OTEL_RESOURCE_ATTRIBUTES` | Custom resource attributes (e.g., `team.id=platform`) | — |
| `OTEL_SERVICE_NAME` | Service name on all telemetry | `claude-code` |
| `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` | `delta` or `cumulative` | delta |
| `CLAUDE_CODE_OTEL_HEADERS_HELPER_DEBOUNCE_MS` | Dynamic header refresh interval | 1740000 (29 min) |

`otelHeadersHelper` can also be set in `.claude/settings.json` as a path to a shell script that outputs JSON headers — useful for OAuth token rotation in enterprise environments.[^1]

### Token usage: metrics schema

The `claude_code.token.usage` metric is a counter incremented **after each API request**:[^1]

```
Metric name:  claude_code.token.usage
Unit:         tokens
Attributes:
  type:   "input" | "output" | "cacheRead" | "cacheCreation"
  model:  e.g. "claude-sonnet-4-6"
  + all standard attributes (session.id, user.account_uuid, organization.id, …)
```

This covers all four token types (input, output, cache read, cache creation) and is emitted per-request with model attribution. The companion `claude_code.cost.usage` counter (unit: USD, attributed to `model`) is emitted on the same cadence.[^1]

### Token usage: log events schema (richer, preferred for per-request data)

The `claude_code.api_request` log event fires for each Claude API call:[^1]

```json
{
  "event.name":          "api_request",
  "event.timestamp":     "<ISO 8601>",
  "event.sequence":      <monotonically increasing integer>,
  "model":               "claude-sonnet-4-6",
  "input_tokens":        <integer>,
  "output_tokens":       <integer>,
  "cache_read_tokens":   <integer>,
  "cache_creation_tokens": <integer>,
  "cost_usd":            <float>,
  "duration_ms":         <integer>,
  "speed":               "fast" | "normal"
}
```

Plus all **standard attributes** on every event: `session.id`, `app.version`, `organization.id`, `user.account_uuid`, `user.account_id`, `user.id`, `user.email`, `terminal.type`.[^1]

The `prompt.id` attribute (UUID) correlates all events produced while processing a single user prompt — one user turn may produce multiple `api_request` events (e.g., tool calls that re-invoke the model). `prompt.id` is only on events, never on metrics, to prevent unbounded metric cardinality.[^1]

### Trace spans schema (beta)

With `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`, the following spans are exported:[^2]

| Span name | What it wraps | Token data |
|-----------|--------------|------------|
| `claude_code.interaction` | One complete user turn | — |
| `claude_code.llm_request` | One Claude API call | model name, latency, token counts as attributes |
| `claude_code.tool` | One tool invocation | — |
| `claude_code.tool.blocked_on_user` | Permission wait | — |
| `claude_code.tool.execution` | Actual tool execution | — |
| `claude_code.hook` | One hook execution | — |

Every span carries `session.id`. Bash and PowerShell subprocesses spawned by Claude inherit a `TRACEPARENT` W3C context header, enabling end-to-end distributed tracing through scripts.[^2][^1]

> ⚠️ **Official caveat:** "Tracing is in beta. Span names and attributes may change between releases."[^2]

### What is documented vs. not documented

**Documented (authoritative as of April 2026):** All of the above, including the complete metric and event schemas, all environment variables, resource attributes, and cardinality controls.[^1]

**Not documented:** The exact span attributes attached to `claude_code.llm_request` (token counts confirmed present in description, but specific attribute names like `gen_ai.usage.input_tokens` vs. proprietary names are not listed in the official reference). The Agent SDK observability page mentions "model name, latency, and token counts as attributes" on `claude_code.llm_request` spans  but does not enumerate field names.[^2]

### Enterprise vs. Max/Pro access

| Aspect | Enterprise (C4E, managed org) | Max/Pro (individual) |
|--------|-------------------------------|----------------------|
| OTEL activation | Admin pushes `env` block in managed settings file (MDM-distributed) — high precedence, cannot be overridden by users [^1] | Individual developer must set env vars before running `claude` |
| Collector endpoint | Centrally configured, all devs automatically route to org collector | Per-developer; no central enforcement |
| `organization.id` attribute | Populated | Not populated (or org UUID if enrolled in C4E team) |
| `user.account_id` | Populated (tagged format matching admin APIs) | Populated if OAuth authenticated |
| Hook policies | `allowManagedHooksOnly` blocks non-managed hooks; admin distributes hooks via managed policy settings [^3] | User-controlled |

No documented difference in the **data** emitted between Enterprise and Max/Pro — the same token fields are present regardless of plan.

### Export latency

- **Log events (api_request):** default 5 seconds; configurable to 1 second with `OTEL_LOGS_EXPORT_INTERVAL=1000`[^2][^1]
- **Metrics:** default 60 seconds; configurable to 1 second with `OTEL_METRIC_EXPORT_INTERVAL=1000`[^4]
- **Trace spans:** default 5 seconds[^2]
- On clean process exit, the CLI flushes pending telemetry, so no data is lost on normal termination; data in-buffer at a kill signal is lost[^2]

***

## Source 2: Claude Code Local Artifacts

### JSONL session transcripts

Claude Code writes every session to a local JSONL file:[^5]

```
~/.claude/projects/<encoded-cwd>/<session-id>.jsonl
```

Each line is a JSON event. Assistant messages carry a `usage` object with `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, and `cache_read_input_tokens` directly from the Anthropic API response. The `transcript_path` is passed to every hook on stdin, so a hook script can parse the running session file immediately.[^3][^6][^7]

This is the **lowest-latency path** to token data for a purely OS-level tool: no OTEL configuration required; just `tail -F` the JSONL file and parse `usage` fields from assistant turns.

**Caveats:**
- A bug (filed August 2025, reported fixed by January 2026) caused the `/cost` command to double-count `cache_creation_input_tokens` when parsing session JSONL. The underlying JSONL data itself is correct; only the `/cost` aggregation had the bug.[^6]
- Each API call appends one event; there's no atomic write fence — a tail-based reader should handle partial lines gracefully.

### Hooks system

The hooks system allows user-defined shell commands, HTTP endpoints, or LLM prompts to fire at specific lifecycle points. All hook types receive a JSON payload on stdin (command hooks) or as the request body (HTTP hooks).[^3]

**Hook events and token-data relevance:**

| Event | Token data relevance |
|-------|---------------------|
| `SessionStart` | Receives `session_id`, `model` — baseline for session tracking |
| `UserPromptSubmit` | Receives `session_id`, `transcript_path`, `prompt` — pre-turn |
| `PostToolUse` | Receives `session_id`, `transcript_path`, `tool_name`, `tool_input`, `tool_response` — post tool |
| `Stop` | Receives `session_id`, `transcript_path` — fires when Claude finishes responding; parse transcript for token totals |
| `SessionEnd` | Fires on session termination |
| `SubagentStart` / `SubagentStop` | `agent_id`, `agent_type` — for multi-agent cost attribution |

Common JSON fields on all hooks: `session_id`, `transcript_path`, `cwd`, `permission_mode`, `hook_event_name`. When running inside a subagent: `agent_id`, `agent_type`.[^3]

**Recommended pattern for real-time per-request cost data without OTEL:** Register a `PostToolUse` or `Stop` HTTP hook pointing at a local endpoint; on each `Stop` event, parse the `transcript_path` JSONL file to extract `usage` from the latest assistant messages.

**Hook locations and scope:**

| Location | Scope | Enterprise control |
|----------|-------|--------------------|
| `~/.claude/settings.json` | All projects, that machine | User-controlled |
| `.claude/settings.json` | Single project, committable | User/project-controlled |
| `.claude/settings.local.json` | Single project, gitignored | User-controlled |
| Managed policy settings | Organization-wide | Admin-controlled; can enforce via MDM |
| Plugin `hooks/hooks.json` | Plugin-scoped | Admin marketplace distribution |

With `allowManagedHooksOnly: true` in managed settings, admins can block all user/project hooks and only allow hooks distributed through the managed settings or approved plugins.[^3]

### MCP server protocol

MCP tools appear as regular tools in hook events (`PreToolUse`, `PostToolUse`) and follow the naming convention `mcp__<server>__<tool>`. There is **no documented MCP-specific usage metadata** beyond what hooks and OTEL already expose. Token overhead from MCP tool definitions is counted against the context window (visible via `/context`) but does not produce a separate telemetry signal.[^3]

As of January 2026, the `/context` command was updated to correctly report MCP tool token usage (a prior bug inflated counts ~3x).[^8]

### Enterprise admin console per-developer data

For Claude for Enterprise (managed org), the admin console exposes engagement metrics, not raw token counts. Token data flows through the Claude Code Analytics Admin API (see Source 7 below).

***

## Source 3: Anthropic Direct API

### Response body — usage object

Every `/v1/messages` response includes:[^9][^10]

```json
{
  "usage": {
    "input_tokens": <integer>,
    "output_tokens": <integer>,
    "cache_creation_input_tokens": <integer>,
    "cache_read_input_tokens": <integer>
  }
}
```

`input_tokens` represents non-cached input tokens. `cache_creation_input_tokens` counts tokens written to a prompt cache entry. `cache_read_input_tokens` counts tokens read from an existing cache entry.[^10]

### Streaming — SSE event sequence

In streaming mode, token counts appear in two SSE events:[^11][^12]

- **`message_start`**: carries `message.usage` with `input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens` (all known at request start)
- **`message_delta`**: carries `usage` with cumulative token counts (includes `output_tokens` and repeated input fields; these are **cumulative**, not deltas)[^13][^14]

The canonical, correct final token count is the `usage` from the `message_delta` event at the end of the stream.[^13]

### Usage/billing API

There is no per-request billing API endpoint accessible with standard API keys. Aggregate usage data requires an Admin API key (see Source 7). For Max/Pro subscription users, Anthropic does not expose any programmatic API to check remaining usage quota or per-request costs.[^15][^16]

***

## Source 4: Amazon Bedrock (Claude)

### InvokeModel / Converse response token fields

The Bedrock Converse API response body includes:[^17][^18]

```json
{
  "usage": {
    "input_tokens": <integer>,
    "output_tokens": <integer>
  }
}
```

For prompt caching (when enabled), the Converse API adds `CacheReadInputTokens` and `CacheWriteInputTokens` to the response. Token counts also appear in HTTP response headers:[^19][^17]

```
x-amzn-bedrock-input-token-count: <integer>
x-amzn-bedrock-output-token-count: <integer>
```

> **Note:** Cache token fields in Bedrock's streaming adapter have had tracking bugs in third-party SDK integrations (opik issue filed March 2026). The Bedrock API itself reports these fields, but integrations have been slow to map them.[^20]

### CloudWatch metrics

AWS Bedrock exports these CloudWatch metrics to the `AWS/Bedrock` namespace:[^21][^22]

| Metric | Description |
|--------|-------------|
| `InputTokenCount` | Tokens sent to the model |
| `OutputTokenCount` | Tokens received from the model |
| `CacheReadInputTokens` | Tokens read from prompt cache |
| `CacheWriteInputTokens` | Tokens written to prompt cache |
| `InvocationLatency` | End-to-end latency |
| `Invocations` | Request count |
| `InvocationThrottles` | Throttled requests |

**Granularity:** CloudWatch metrics are aggregated (minimum 1-minute resolution); they do **not** provide per-request granularity natively. Dimensions include `ModelId` and `Region`. For per-developer attribution, use invocation logging (below).[^23]

### Bedrock Model Invocation Logging (per-request data)

This account-wide setting routes per-invocation logs to CloudWatch Logs or S3. Each log record includes:[^24][^25][^26]

```json
{
  "timestamp": "…",
  "accountId": "…",
  "region": "…",
  "requestId": "…",
  "operation": "InvokeModelWithResponseStream",
  "modelId": "arn:aws:bedrock:…:claude-opus-4-6-v1",
  "identity": { "arn": "arn:aws:iam::…:user/developer1" },
  "input": {
    "inputTokenCount": 1000,
    "cacheReadInputTokenCount": 0,
    "cacheWriteInputTokenCount": 60000
  },
  "output": {
    "outputTokenCount": 100
  }
}
```

The `identity.arn` field provides **per-developer attribution** for organizations where each developer uses a distinct IAM identity. Logs are queryable via CloudWatch Logs Insights or Amazon Athena against S3.[^27][^26]

**Latency:** Near-real-time delivery to CloudWatch Logs; S3 batched.

### Bedrock Cost Explorer / tagging

Request-level tagging for cost attribution is not directly available on standard model invocations in Bedrock. Cost attribution is typically handled via IAM user/role-level billing tags on the AWS account. Inference profiles (cross-region) can be tagged.

**Access:** Both Enterprise and Max/Pro customers can use Bedrock as the backend for Claude Code. Bedrock-based usage is NOT tracked by the Anthropic Claude Code Analytics API.[^28]

***

## Source 5: Google Vertex AI (Claude)

### Predict / GenerateContent response fields

When calling Claude via Vertex AI using the Anthropic SDK (`AnthropicVertex`), the response includes the standard Anthropic `usage` object:[^29][^30]

```python
message.usage.input_tokens   # integer
message.usage.output_tokens  # integer
```

Anthropic's documentation states the `usage` object is "consistent across all platforms (first-party API, Foundry, Amazon Bedrock, and Google Vertex AI)". However, cache token mapping via third-party SDK integrations has shown issues (e.g., `input_tokens` and `output_tokens` not mapping to expected Langfuse keys, January 2026 GitHub issue ).[^31][^29]

### Cloud Monitoring metrics

Vertex AI exports metrics to Cloud Monitoring under `aiplatform.googleapis.com`. The standard monitored metrics for Vertex AI endpoints cover:[^32][^33]

- Prediction latency, error rate, predictions-per-second, CPU/memory/network for deployed model endpoints

**Token-count metrics in Cloud Monitoring: not present for Vertex AI partner models (Claude).** The `aiplatform.googleapis.com` metric catalog does not include per-request input/output token counts for Claude models accessed via publisher endpoints. This is confirmed by a Google Cloud developer forum discussion (April 2025) that recommended custom logging + BigQuery as the workaround for token tracking.[^34][^32]

**For per-request or per-user token tracking on Vertex AI:** implement application-side logging, export to Cloud Logging or BigQuery, and query via BigQuery or Data Studio. Alternatively, the GCP Billing report with SKU grouping shows token volume at a model level (monthly cadence).[^35][^34]

**Data Access Logs** can surface Vertex AI model invocations in Cloud Audit Logs, but these are expensive and log access patterns rather than token counts.[^36]

### Request tagging

Custom user-defined labels on Vertex AI requests have limited support for third-party partner models. There is an open GitHub discussion (April 2025) requesting label support for Claude on Vertex for per-team cost breakdowns; no GA resolution as of April 2026.[^34]

***

## Source 6: Azure AI Foundry (Claude)

### Response body

Azure AI Foundry returns the standard Anthropic `usage` object:[^37][^29]

```json
{
  "usage": {
    "input_tokens": <integer>,
    "output_tokens": <integer>,
    "cache_creation_input_tokens": <integer>,
    "cache_read_input_tokens": <integer>
  }
}
```

Anthropic explicitly documents this as consistent across deployment platforms.[^29]

### Azure Monitor metrics

Azure Monitor collects metrics for Foundry resources under `Microsoft.CognitiveServices/accounts`:[^38][^39]

| Metric (REST API name) | Description | Granularity |
|------------------------|-------------|------------|
| `ProcessedPromptTokens` | Input tokens | 1-minute (aggregated) |
| `GeneratedTokens` | Output (completion) tokens | 1-minute (aggregated) |
| `ProcessedInferenceTokens` | Prompt + generated tokens | 1-minute (aggregated) |
| `InputTokens` | Input tokens (Models category) | 1-minute (aggregated) |
| `OutputTokens` | Output tokens (Models category) | 1-minute (aggregated) |
| `TotalTokens` | Input + output (Models category) | 1-minute (aggregated) |

**Dimensions:** `ModelDeploymentName`, `ModelName`, `ModelVersion`, `Region`, `ApiName`.[^39][^38]

**Critical limitations for Claude:**
- These metrics follow the OpenAI schema; `cache_creation_input_tokens` (Anthropic-specific) does **not** have a corresponding Azure Monitor metric natively.
- No per-user or per-developer dimension — attribution is at the deployment level.
- Minimum granularity is 1 minute (aggregated).

### Per-developer tracking

Per-developer token attribution on Azure Foundry requires an **APIM + Application Insights** pattern: route requests through Azure API Management with user metadata headers, log token usage as custom events to Application Insights, and query via KQL in Log Analytics. An example KQL query  filters on `message == "llm.usage"` custom dimensions to surface `agent_name`, `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `cost_usd` per request.[^40]

### Billing granularity

Azure bills Claude usage at pay-as-you-go rates per token. Billing data appears in Azure Cost Management with SKU-level breakdowns; there is no sub-hourly cost API.

***

## Source 7: Anthropic Admin API & Console

### Claude Code Analytics Admin API

**Endpoint:** `GET /v1/organizations/usage_report/claude_code`[^28]
**Auth:** Admin API key (`sk-ant-admin…`)  
**Access:** Enterprise orgs and API orgs with an organization configured; **not available to individual accounts**[^28]

This API returns **daily aggregated, per-user Claude Code metrics** — the most complete server-side token data for Enterprise customers:

```json
{
  "date": "2025-09-01T00:00:00Z",
  "actor": { "type": "user_actor", "email_address": "dev@company.com" },
  "organization_id": "…",
  "customer_type": "api" | "subscription",
  "terminal_type": "vscode" | "tmux" | "iTerm.app",
  "core_metrics": {
    "num_sessions": 5,
    "lines_of_code": { "added": 1543, "removed": 892 },
    "commits_by_claude_code": 12,
    "pull_requests_by_claude_code": 2
  },
  "tool_actions": {
    "edit_tool": { "accepted": 45, "rejected": 5 },
    "write_tool": { "accepted": 8, "rejected": 1 }
  },
  "model_breakdown": [
    {
      "model": "claude-opus-4-6",
      "tokens": {
        "input": 100000, "output": 35000,
        "cache_read": 10000, "cache_creation": 5000
      },
      "estimated_cost": { "currency": "USD", "amount": 1025 }
    }
  ]
}
```

**Data freshness:** ~1 hour delay. **NOT real-time** — use OTEL for that.[^28]
**Coverage:** Anthropic API (1st party) only; Bedrock/Vertex usage excluded.[^28]
**Granularity:** Daily per user; no per-session or per-request breakdown.

### Anthropic Usage & Cost Admin API

**Endpoint:** `GET /v1/organizations/usage_report/messages`[^41]
**Auth:** Admin API key  
**Access:** Enterprise/API orgs; individual accounts not supported[^41]

Provides aggregate token buckets across the organization:[^41]

- Time buckets: `1m` (up to 1440 per query), `1h` (up to 168), `1d` (up to 31)
- Tokens tracked: `input_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, `output_tokens`
- Filter/group by: API key, workspace, model, service tier, context window, data residency, speed (fast/standard)
- Data freshness: ~5 minutes

This is **not per-session or per-request** — it is aggregate bucketed data useful for billing reconciliation. There is also a `/v1/organizations/cost_report` endpoint for daily cost breakdowns by workspace.[^41]

### Claude Enterprise Analytics API

**Endpoint:** `GET /v1/organizations/analytics/users`[^42]
**Auth:** `read:analytics` scoped key, Primary Owner only  
**Access:** Claude for Enterprise (managed org) only; data available from 2026-01-01; 3-day delay[^42]

Returns per-user, per-day engagement metrics — **no token counts**:

- `claude_code_metrics.core_metrics.distinct_session_count`, commit_count, pull_request_count, lines_of_code.added/removed
- `claude_code_metrics.tool_actions.edit_tool`, write_tool, notebook_edit_tool (accepted/rejected counts)
- `chat_metrics.*` (conversation counts, messages, files, artifacts)
- `cowork_metrics.*`, `office_metrics.*`

This API complements the Claude Code Analytics API (which has token data) but does not replace it.

***

## Cross-Source Access Matrix

| Source | Token Fields | Per-Request | Per-User | Enterprise | Max/Pro | Latency | Access Pattern |
|--------|-------------|-------------|----------|-----------|---------|---------|---------------|
| **Claude Code OTEL — api_request event** | input, output, cache_read, cache_creation, cost_usd, model | ✅ | ✅ (user.account_id) | ✅ Admin-enforced | ✅ Dev opt-in | ~5 s | OTLP push / Prometheus scrape |
| **Claude Code OTEL — token.usage metric** | input, output, cacheRead, cacheCreation, model | ✅ (per-request counter) | ✅ | ✅ | ✅ | ~60 s | OTLP push / Prometheus scrape |
| **Claude Code OTEL — traces (beta)** | token counts on llm_request span (field names undocumented) | ✅ | ✅ | ✅ | ✅ | ~5 s | OTLP push |
| **Local JSONL (~/.claude/projects/)** | input, output, cache_creation, cache_read | ✅ | Machine-local | ✅ | ✅ | Real-time (tail) | File tail / inotify |
| **Hooks — Stop event + transcript parse** | input, output, cache_creation, cache_read | Per-turn sum | Machine-local | ✅ | ✅ | End-of-turn | Hook script |
| **Anthropic API response body** | input, output, cache_creation, cache_read | ✅ | N/A (proxy required) | ✅ (direct API) | ✅ (direct API) | In response | Response inspection |
| **Claude Code Analytics API** | input, output, cache_read, cache_creation, estimated_cost per model | ❌ (daily per user) | ✅ | ✅ Admin key | ✅ (API/sub orgs) | ~1 h | REST poll |
| **Anthropic Usage & Cost API** | input, output, cache_read, cache_creation (bucketed) | ❌ (min 1m bucket) | ❌ (by API key/workspace) | ✅ Admin key | ❌ | ~5 min | REST poll |
| **Enterprise Analytics API** | ❌ (no token counts) | ❌ | ✅ | ✅ Primary Owner | ❌ | 3 days | REST poll |
| **Bedrock response body** | input, output, cache_read, cache_write | ✅ | N/A | ✅ (if on Bedrock) | ✅ (if on Bedrock) | In response | Response inspection |
| **Bedrock CloudWatch metrics** | InputTokenCount, OutputTokenCount, CacheReadInputTokens, CacheWriteInputTokens | ❌ (aggregated) | ✅ via IAM identity + invocation logs | ✅ | ✅ | ~1–5 min | CloudWatch API |
| **Bedrock Invocation Logging** | inputTokenCount, cacheReadInputTokenCount, cacheWriteInputTokenCount, outputTokenCount + identity.arn | ✅ | ✅ (via IAM identity) | ✅ (AWS account admin) | ✅ (if using Bedrock) | Near-real-time | CloudWatch Logs Insights / Athena |
| **Vertex AI response** | input_tokens, output_tokens (standard usage object) | ✅ | N/A | ✅ | ✅ | In response | Response inspection |
| **Vertex AI Cloud Monitoring** | ❌ (no token metrics for partner models) | ❌ | ❌ | ❌ | ❌ | N/A | Not applicable |
| **Vertex custom logging → BigQuery** | All token fields | ✅ | ✅ (with custom labels) | ✅ | ✅ | Minutes–hours | BigQuery query |
| **Azure Foundry response** | input, output, cache_creation, cache_read | ✅ | N/A | ✅ | ✅ | In response | Response inspection |
| **Azure Monitor metrics** | ProcessedPromptTokens, GeneratedTokens, ProcessedInferenceTokens | ❌ (1-min aggregated) | ❌ (deployment level) | ✅ (Azure admin) | N/A | ~1 min | Azure Monitor API |
| **Azure APIM + App Insights** | prompt_tokens, completion_tokens, total_tokens, cost_usd | ✅ (per request) | ✅ (with custom dimension) | ✅ | N/A | Minutes | Log Analytics KQL |

***

## Practical Recommendations by Customer Profile

### Enterprise (Claude for Enterprise, 200+ developers)

**Recommended architecture:**
1. Deploy an OTEL collector (e.g., OpenTelemetry Collector Contrib, Grafana Alloy, Datadog Agent) at a central endpoint accessible from developer machines.
2. Distribute `CLAUDE_CODE_ENABLE_TELEMETRY=1`, `OTEL_LOGS_EXPORTER=otlp`, `OTEL_METRICS_EXPORTER=otlp`, and collector endpoint via the managed settings `env` block — enforced via MDM (Jamf, Intune, etc.).[^1]
3. Primary token data: `claude_code.api_request` log events — per-request, ~5s latency, includes `user.account_id` for per-developer attribution.
4. Supplement with Claude Code Analytics API polls for daily per-user summaries with model-level cost breakdowns.[^28]
5. Deploy organization-wide `Stop` hooks (via managed policy) for session-completion summaries if you need end-of-session rollups without an OTEL backend.

**If developers use Bedrock:** Enable Bedrock Model Invocation Logging account-wide; use IAM identity (`identity.arn`) for per-developer attribution and query via Athena or CloudWatch Logs Insights.[^26][^24]

### Startup/SMB (Max/Pro stacked seats, no admin console)

**The hard reality:** There is no centralized server-side token API for Max/Pro individual accounts. The only options are:[^16][^15]

1. **OTEL with peer agreement:** Ask each developer to set `CLAUDE_CODE_ENABLE_TELEMETRY=1` and configure a shared OTEL collector endpoint. Since Max/Pro accounts don't have managed settings enforcement, this requires developer buy-in. A shared shell profile or `.envrc` block is the practical delivery mechanism.
2. **Local JSONL scraping:** Your OS-level observability tool can directly read `~/.claude/projects/*/**.jsonl` on each developer's machine, parse `usage` fields from assistant turns, and ship to a central aggregator. No configuration required beyond file-system access.
3. **Hook-based approach:** Distribute a `Stop` hook in project `.claude/settings.json` that posts session token totals to a local or remote endpoint. This is committable to the project repo.
4. **No server-side fallback:** There is no Max/Pro equivalent of the Admin API. Usage limits and quota data are not programmatically accessible (open GitHub issue as of March 2026 ).[^15]

***

## Known Gaps and Unresolved Items

| Item | Status |
|------|--------|
| Exact OTEL attribute names on `claude_code.llm_request` spans (beta) | **Not documented** as of April 2026; only described as "model name, latency, and token counts" [^2] |
| Whether `organization.id` is populated for Max/Pro subscription users | Not explicitly documented; appears absent for individual accounts |
| Vertex AI Cloud Monitoring token metrics for Claude partner models | **Not available** — confirmed by absence from metric catalog and community discussion [^34] |
| Azure Monitor native support for Anthropic `cache_creation_input_tokens` | **Not available** — Azure Monitor follows OpenAI schema [^38] |
| Programmatic access to Max/Pro usage quota/limits | **Not available** — GitHub issue filed March 2026, unresolved [^15] |
| Bedrock Invocation Logging availability for Claude Code sessions (vs. direct API calls) | Log format documented [^26]; Claude Code on Bedrock should follow same pattern, but Anthropic's Claude Code Analytics API explicitly excludes Bedrock usage [^28] |
| OTEL data from Enterprise admin console vs. OTEL pipeline | Admin console shows OTEL-derived aggregate dashboards; raw OTEL stream is separate |

---

## References

1. [Code edit tool decision counter](https://code.claude.com/docs/en/monitoring-usage) - Learn how to enable and configure OpenTelemetry for Claude Code.

2. [Observability with OpenTelemetry - Claude Code Docs](https://code.claude.com/docs/en/agent-sdk/observability) - Export traces, metrics, and events from the Agent SDK to your observability backend using OpenTeleme...

3. [claude-code-hooks-multi-agent-observability](https://cultofclaude.com/skills/claude-code-hooks-multi-agent-observability/) - Real-time monitoring and visualization for Claude Code agents through comprehensive hook event track...

4. [Bringing Observability to Claude Code: OpenTelemetry in ...](https://signoz.io/blog/claude-code-monitoring-with-opentelemetry/) - Monitor Claude Code usage with OpenTelemetry and SigNoz. This blog walks you through implementing co...

5. [Work with sessions - Claude API Docs](https://platform.claude.com/docs/en/agent-sdk/sessions) - How sessions persist agent conversation history, and when to use continue, resume, and fork to retur...

6. [[BUG] Local /cost doubles usage when parsing session JSONL ...](https://github.com/anthropics/claude-code/issues/5904) - Environment Platform (select one): Anthropic API AWS Bedrock Google Vertex AI Other: Custom local CL...

7. [Accessing Claude Code Previous Sessions via JSONL ...](https://fazm.ai/blog/claude-code-previous-sessions-jsonl-transcripts) - Where Claude Code stores previous session transcripts as JSONL files, how to find them in ~/.claude/...

8. [Do MCP Servers Really Eat Half Your Context Window?](https://www.async-let.com/posts/claude-code-mcp-token-reporting/) - The MCP vs. CLI debate hinges on token cost. I investigated whether the reported usage is real paylo...

9. [Prompt caching - Anthropic](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching?s=09)

10. [Prompt caching - Claude API Docs](https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching) - Input token counting: When thinking blocks are read from cache, they count as input tokens in your u...

11. [Stream Anthropic responses using the message-per-response pattern](https://ably.com/docs/guides/ai-transport/anthropic-message-per-response) - Stream tokens from the Anthropic Messages API over Ably in realtime using message appends.

12. [Streaming Messages - Claude API Docs](https://docs.anthropic.com/en/api/messages-streaming) - The token counts shown in the usage field of the message_delta event are cumulative. Ping events. Ev...

13. [fix(anthropic): incorrect `input_tokens` counting while streaming by adityamohta · Pull Request #32525 · langchain-ai/langchain](https://github.com/langchain-ai/langchain/pull/32525) - Supersedes: #32461 From docs: https://docs.anthropic.com/en/docs/build-with-claude/streaming#event-t...

14. [Anthropic API provider is not counting input and cache tokens from `messageDelta` events · Issue #4346 · cline/cline](https://github.com/cline/cline/issues/4346) - What happened? Anthropic API updated their spec at some point to include full usage details in messa...

15. [Expose Max plan usage limits via Claude Code API/SDK #32796](https://github.com/anthropics/claude-code/issues/32796) - Currently there's no programmatic way to check Claude Max plan usage limits (session %, weekly all-m...

16. [Claude Code Pricing Guide: Which Plan Actually Saves You Money](https://www.ksred.com/claude-code-pricing-guide-which-plan-actually-saves-you-money/) - Claude Max ($100 or $200/month): The 5x tier at $100/month gives you 5x Pro usage, and the 20x at $2...

17. [Prompt caching for faster model inference - Amazon Bedrock](https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html) - Prompt caching is an optional feature that you can use with supported models on Amazon Bedrock to re...

18. [Request and Response - Amazon Bedrock - AWS Documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages-request-response.html) - input_tokens – The number of input tokens in the request. output_tokens – The number tokens of that ...

19. [How to get used tokens from bedrock AI - Stack Overflow](https://stackoverflow.com/questions/78182312/how-to-get-used-tokens-from-bedrock-ai) - You can get the data off the response body. There's a usage property that has the input_tokens and o...

20. [[Bug]: bedrock claude adapter doesn't save cache tokens usage](https://github.com/comet-ml/opik/issues/6019) - I investigated and confirmed a bug in the Bedrock Claude adapter's streaming path where cache token ...

21. [Amazon Bedrock Token Management: Strategies for Smarter AI Usage](https://tutorialsdojo.com/amazon-bedrock-token-management-strategies-for-smarter-ai-usage/) - Did you know? Big tech companies, fast-growing startups, and even solo developers are already using ...

22. [How tokens are counted in Amazon Bedrock - AWS Documentation](https://docs.aws.amazon.com/bedrock/latest/userguide/quotas-token-burndown.html) - Learn how to calculate the token burndown rate for Amazon Bedrock.

23. [Is it possible to get token usage metrics or other usage details from ...](https://zilliz.com/ai-faq/is-it-possible-to-get-token-usage-metrics-or-other-usage-details-from-amazon-bedrock-after-making-a-request-to-track-costs-or-performance) - **Yes**, Amazon Bedrock provides mechanisms to track token usage and other metrics, though the appro...

24. [Monitor model invocation using CloudWatch Logs and Amazon S3](https://docs.aws.amazon.com/bedrock/latest/userguide/model-invocation-logging.html) - You can use model invocation logging to collect invocation logs, model input data, and model output ...

25. [AWS Bedrock Model Invocation Logging - an Overview](https://community.aws/content/2s5cG1VQe1478chNNsZBYUalvIW/aws-bedrock-model-invocation-logging-an-overview?lang=en) - Understanding what this important account-wide setting means for generative AI applications

26. [Visualizing User-Level Costs for Claude Code on Bedrock Using ...](https://zenn.dev/kiiwami/articles/claude_code_bedrock_cost_pattern?locale=en)

27. [Claude Code deployment patterns and best practices with ...](https://aws.amazon.com/blogs/machine-learning/claude-code-deployment-patterns-and-best-practices-with-amazon-bedrock/) - In this post, we explore deployment patterns and best practices for Claude Code with Amazon Bedrock,...

28. [Claude Code Analytics API](https://platform.claude.com/docs/en/build-with-claude/claude-code-analytics-api) - Programmatically access your organization's Claude Code usage analytics and productivity metrics wit...

29. [Claude in Microsoft Foundry - Claude API Docs](https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry) - Access Claude models through Microsoft Foundry with Azure-native endpoints and authentication.

30. [A developer's guide for building with Anthropic's Claude 4 models ...](https://discuss.google.dev/t/a-developers-guide-for-building-with-anthropic-s-claude-4-models-on-vertex-ai/191912) - In this blog, we'll guide you through building with the new Claude 4 models on Vertex AI. We'll begi...

31. [google-vertex-ai adapter not capturing token usage for Claude models](https://github.com/orgs/langfuse/discussions/11475) - Describe your question. using Claude Sonnet 4.5 via Google Vertex AI Model Garden. LLM-as-judge eval...

32. [Cloud Monitoring metrics for Vertex AI | Google Cloud Documentation](https://docs.cloud.google.com/vertex-ai/docs/general/monitoring-metrics) - Resource usage metrics can help you track your model's CPU usage, memory usage, and network usage. Y...

33. [Cloud Monitoring metrics for Vertex AI | Google Cloud](https://cloud.google.com/vertex-ai/docs/general/monitoring-metrics?authuser=09) - Learn about Cloud Monitoring metrics that are available in Vertex AI.

34. [Custom Label Support for Third-Party Models (e.g., Claude) in ...](https://discuss.google.dev/t/custom-label-support-for-third-party-models-e-g-claude-in-vertex-ai-for-cost-breakdown/186646) - Hi everyone, I'm currently using Vertex AI to access LLMs from third-party providers such as Claude....

35. [Monitor the usage of Gemini API on Vertex AI - Custom ML & MLOps](https://discuss.google.dev/t/monitor-the-usage-of-gemini-api-on-vertex-ai/171653) - Hello,. try logging token counts for each request within your app, then use Cloud Logging or BigQuer...

36. [Querying Vertex AI Model Usage through GCP Observability Metrics](https://ohlinger.co/posts/vertex-ai-metrics/) - You can view the different model invocations throughout your environment for the specified project. ...

37. [Claude in Microsoft Foundry](https://platform.claude.com/docs/en/build-with-claude/claude-in-microsoft-foundry?f80ce999_sort_date=desc) - Access Claude models through Microsoft Foundry with Azure-native endpoints and authentication.

38. [Monitoring data reference for Azure OpenAI - Microsoft Foundry](https://learn.microsoft.com/en-us/azure/foundry/openai/monitor-openai-reference) - This article contains important reference material you need when you monitor Azure OpenAI in Microso...

39. [Azure OpenAI monitoring data reference](https://learn.microsoft.com/en-us/azure/ai-foundry/openai/monitor-openai-reference) - This article contains important reference material you need when you monitor Azure OpenAI in Azure A...

40. [Tracking Every Token: Granular Cost and Usage Metrics ...](https://techcommunity.microsoft.com/blog/azure-ai-foundry-blog/tracking-every-token-granular-cost-and-usage-metrics-for-microsoft-foundry-agent/4503143) - As organizations scale their use of AI agents, one question keeps surfacing: how much is each agent ...

41. [Usage and Cost API - Claude Console](https://platform.claude.com/docs/en/build-with-claude/usage-cost-api) - Programmatically access your organization's API usage and cost data with the Usage & Cost Admin API.

42. [Claude Enterprise Analytics API reference guide](https://support.claude.com/en/articles/13703965-claude-enterprise-analytics-api-reference-guide) - Returns per-user engagement metrics for a single day. Each item in the response represents one user ...

