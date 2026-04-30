# Claude Code OTEL — Authoritative Reference

**Captured:** 2026-04-26
**Sources read:**
- https://code.claude.com/docs/en/monitoring-usage
- https://code.claude.com/docs/en/agent-sdk/observability
- https://code.claude.com/docs/en/env-vars

> ## Staleness warning
>
> Claude Code's OTEL surface is partially in beta and the docs flag
> tracing explicitly as "may change between releases."
>
> **Re-verify before:**
> - Any spec work that depends on attribute names, span shapes, or
>   env var behavior
> - Any release of coolant that touches the cc-otel adapter
> - **Any time more than 2 weeks have passed since the captured date
>   above.** Check the source URLs and diff against this file.
>
> If the docs have drifted, update this file in place with the new
> capture date — don't keep stale claims around. Cross-reference
> any changes against `docs/_drafts/cc-otel-beta-adapter.spec.md`.

---

## 1. Enable Mechanism

**Required env vars (in order of dependency):**

```bash
# Master enable flag — required for any signal
export CLAUDE_CODE_ENABLE_TELEMETRY=1

# Per-signal exporter selection (each independently toggleable)
export OTEL_METRICS_EXPORTER=otlp     # otlp | prometheus | console | none
export OTEL_LOGS_EXPORTER=otlp        # otlp | console | none
export OTEL_TRACES_EXPORTER=otlp      # otlp | console | none

# Distributed tracing requires an ADDITIONAL beta gate
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1
```

**No `~/.claude/settings.json` field exists for enable/disable.** The
only settings.json field related is `otelHeadersHelper` (path to a
script that generates dynamic headers, e.g. for mTLS).

**OTLP transport configuration (standard OpenTelemetry env vars):**

```bash
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc            # grpc | http/json | http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer token"
```

**Per-signal endpoint and protocol overrides** (override the
single-endpoint vars above when set):

- `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` / `_PROTOCOL`
- `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` / `_PROTOCOL`
- `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` / `_PROTOCOL`

**⚠ Coolant gotcha:** CC's default protocol is **gRPC at port 4317**;
coolant's `dev/otel/` Prometheus native OTLP receiver is **HTTP at
9090/api/v1/otlp/v1/metrics**. Enable scripts MUST set:

```bash
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://localhost:9090/api/v1/otlp/v1/metrics
```

**Default export intervals:**

| Signal  | Default  | Override env var                |
|---------|----------|---------------------------------|
| Metrics | 60000 ms | `OTEL_METRIC_EXPORT_INTERVAL`   |
| Logs    | 5000 ms  | `OTEL_LOGS_EXPORT_INTERVAL`     |
| Traces  | 5000 ms  | `OTEL_TRACES_EXPORT_INTERVAL`   |

Note the asymmetry — metrics emit at 60s, logs/traces at 5s.

**Flush / shutdown timeouts at process exit:**

- `CLAUDE_CODE_OTEL_FLUSH_TIMEOUT_MS` (default 5000 ms)
- `CLAUDE_CODE_OTEL_SHUTDOWN_TIMEOUT_MS` (default 2000 ms)

If CC is killed before flush completes, pending data is lost.

---

## 2. Signal Types

Three independent signals, each independently toggleable:

| Signal      | Enable                                           | Stability |
|-------------|--------------------------------------------------|-----------|
| Metrics     | `OTEL_METRICS_EXPORTER` (≠ `none`)               | GA        |
| Log events  | `OTEL_LOGS_EXPORTER` (≠ `none`)                  | GA        |
| Traces      | `OTEL_TRACES_EXPORTER` (≠ `none`) AND `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1` | Beta      |

Detailed beta tracing (more attributes on hook spans, more content):
`ENABLE_BETA_TRACING_DETAILED=1` plus `BETA_TRACING_ENDPOINT` set.
Interactive CLI requires organization allowlisting for detailed
hook tracing; SDK and `-p` non-interactive sessions are not gated.

---

## 3. Metric Inventory

**Eight documented metrics** (Prometheus normalizes `.` to `_` and
appends `_total` to counters):

| Source name                              | Prometheus name                            | Unit   | Per-metric attributes (beyond standard) |
|------------------------------------------|--------------------------------------------|--------|------------------------------------------|
| `claude_code.session.count`              | `claude_code_session_count_total`          | count  | `start_type` (fresh/resume/continue)     |
| `claude_code.lines_of_code.count`        | `claude_code_lines_of_code_count_total`    | count  | `type` (added/removed)                   |
| `claude_code.pull_request.count`         | `claude_code_pull_request_count_total`     | count  | (none)                                   |
| `claude_code.commit.count`               | `claude_code_commit_count_total`           | count  | (none)                                   |
| `claude_code.cost.usage`                 | `claude_code_cost_usage_USD_total`         | USD    | `model`, `query_source`, `speed`, `effort` |
| `claude_code.token.usage`                | `claude_code_token_usage_tokens_total`     | tokens | `type` (input/output/cacheRead/cacheCreation), `model`, `query_source`, `speed`, `effort` |
| `claude_code.code_edit_tool.decision`    | `claude_code_code_edit_tool_decision_total`| count  | `tool_name`, `decision` (accept/reject), `source`, `language` |
| `claude_code.active_time.total`          | `claude_code_active_time_seconds_total`    | s      | `type` (user/cli)                        |

**`query_source` values:** `main`, `subagent`, `auxiliary`. CC OTEL
already attribute-splits cost/token usage by query source — coolant
does NOT need to add this dimension.

**`speed` values:** `fast` (Opus 4.6 in fast mode), `normal`.

### Standard attributes on every metric

| Attribute             | Default included? | Override env var                      |
|-----------------------|-------------------|---------------------------------------|
| `session.id`          | Yes               | `OTEL_METRICS_INCLUDE_SESSION_ID=false` to disable |
| `app.version`         | No                | `OTEL_METRICS_INCLUDE_VERSION=true` to enable      |
| `organization.id`     | Yes (when authenticated) | (always, no toggle)              |
| `user.account_uuid`   | Yes               | `OTEL_METRICS_INCLUDE_ACCOUNT_UUID=false` to disable |
| `user.account_id`     | Yes (Anthropic-tagged format `user_01BWBeN28...`) | `OTEL_METRICS_INCLUDE_ACCOUNT_UUID=false` |
| `user.id`             | Yes (anonymous device ID) | (always)                       |
| `user.email`          | Yes (when OAuth)  | (always when authenticated via OAuth) |
| `terminal.type`       | Yes (when detected: iTerm.app, vscode, cursor, tmux, etc.) | (always when detected) |

**Cardinality implications for coolant:**
- Per-`session.id` reconciliation IS possible by default.
- A user can reduce cardinality by disabling `session.id` — coolant
  must handle absence gracefully (fall back to org-level rollups).

### `dev/otel/CLAUDE.md` legacy list

The local CLAUDE.md previously listed six metrics from a live capture.
Two metrics (`claude_code.pull_request.count`, `claude_code.commit.count`)
were missing — the test session likely didn't trigger PR or commit
emissions. The full list above (8 metrics) is now authoritative.

---

## 4. Trace Shape (Beta)

**Status:** Beta. Docs explicitly state "Span names and attributes
may change between releases."

### Span hierarchy

```
claude_code.interaction (root span per user prompt)
├── claude_code.llm_request
├── claude_code.hook (detailed beta tracing only)
└── claude_code.tool
    ├── claude_code.tool.blocked_on_user
    ├── claude_code.tool.execution
    └── (nested llm_request/tool spans for Task subagents)
```

### Span attributes by type

**`claude_code.interaction`:**
- `user_prompt` (redacted by default; gated by `OTEL_LOG_USER_PROMPTS=1`)
- `user_prompt_length`
- `interaction.sequence`, `interaction.duration_ms`

**`claude_code.llm_request`:**
- `model`, `gen_ai.system` (always `"anthropic"`), `query_source`, `speed`
- Timing: `duration_ms`, `ttft_ms`
- Tokens: `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`
- Identity: `request_id`, `client_request_id`, `attempt`
- Outcome: `success`, `status_code`, `error`, `response.has_tool_call`

**`claude_code.tool`:**
- `tool_name`, `duration_ms` (includes permission wait), `result_tokens`
- Gated by `OTEL_LOG_TOOL_DETAILS=1`: `file_path`, `full_command`,
  `skill_name`, `subagent_type`
- Gated by `OTEL_LOG_TOOL_CONTENT=1` (traces only, 60 KB truncation):
  full input/output bodies as span events

**`claude_code.tool.blocked_on_user`:**
- `duration_ms`, `decision` (accept/reject), `source` (config/hook/user_*)

**`claude_code.tool.execution`:**
- `duration_ms`, `success`, `error` (category string by default;
  full error gated by `OTEL_LOG_TOOL_DETAILS=1`)

**`claude_code.hook`** (only present when `ENABLE_BETA_TRACING_DETAILED=1`
+ `BETA_TRACING_ENDPOINT` set):
- `hook_event` (PreToolUse, etc.), `hook_name`, `num_hooks`,
  `duration_ms`, `num_success`, `num_blocking`, `num_non_blocking_error`,
  `num_cancelled`
- `hook_definitions` (JSON, gated by `OTEL_LOG_TOOL_DETAILS=1`)

### Subagent linkage

When the `Task` tool spawns a subagent, the subagent's
`llm_request` and `tool` spans nest under the parent's
`claude_code.tool` span. **One trace = one delegation chain.** This
is the natural reconciliation surface for coolant's per-agent rollups.

---

## 5. Log Events (30+ types)

Every log event carries `prompt.id` — a UUID linking a user prompt
to all resulting API calls and tool executions. **`prompt.id` is the
correlation key**, not `session.id`.

| Event type                            | Key fields |
|---------------------------------------|------------|
| `claude_code.user_prompt`             | `prompt_length`, `prompt` (gated by `OTEL_LOG_USER_PROMPTS`), `command_name`, `command_source` |
| `claude_code.tool_result`             | `tool_name`, `tool_use_id`, `success`, `duration_ms`, `error_type`, `tool_input_size_bytes`, `tool_result_size_bytes`, `tool_parameters` (gated), `tool_input` (gated) |
| `claude_code.api_request`             | `model`, `cost_usd`, `duration_ms`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`, `request_id`, `speed`, `query_source`, `effort` |
| `claude_code.api_error`               | `model`, `error`, `status_code`, `duration_ms`, `attempt`, `request_id` |
| `claude_code.api_request_body`        | Gated by `OTEL_LOG_RAW_API_BODIES=1` or `=file:<dir>`; `body` or `body_ref`, `body_length`, `body_truncated` |
| `claude_code.api_response_body`       | Gated by `OTEL_LOG_RAW_API_BODIES`; extended-thinking always redacted |
| `claude_code.tool_decision`           | `tool_name`, `tool_use_id`, `decision`, `source` |
| `claude_code.permission_mode_changed` | `from_mode`, `to_mode`, `trigger` |
| `claude_code.auth`                    | `action`, `success`, `auth_method`, `error_category`, `status_code` |
| `claude_code.mcp_server_connection`   | `status`, `transport_type`, `server_scope`, `duration_ms`, `error_code` |
| `claude_code.hook_execution_start`    | `hook_event`, `hook_name`, `num_hooks` |
| `claude_code.hook_execution_complete` | counts of success / blocking / error / cancelled |
| `claude_code.skill_activated`         | `skill.name` (default `custom_skill` unless `OTEL_LOG_TOOL_DETAILS`), `skill.source`, `plugin.name` |
| `claude_code.plugin_installed`        | `marketplace.is_official`, `install.trigger`, plugin/marketplace names (gated) |
| `claude_code.api_retries_exhausted`   | `model`, `error`, `status_code`, `total_attempts`, `total_retry_duration_ms` |
| `claude_code.internal_error`          | `error_name`, `error_code` (never includes message/stack) |
| `claude_code.compaction`              | `trigger` (auto/manual), `success`, `duration_ms`, `pre_tokens`, `post_tokens` |

Other events the docs reference but didn't fully detail in capture:
context budget warnings, slash command invocations, plan-mode
transitions. Re-verify against the source docs if needed.

---

## 6. Resource Attributes

Standard OTEL resource attributes attached to all spans, metrics,
and events:

```
service.name        = "claude-code"
service.version     = (current Claude Code version)
os.type             = linux | darwin | windows
os.version          = (OS version string)
host.arch           = amd64 | arm64 | etc.
wsl.version         = (only on WSL)
```

**Override and extend:**

```bash
export OTEL_SERVICE_NAME="support-triage-agent"
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,team.id=platform"
```

Format rules (strict):
- Comma-separated `key=value` pairs; no spaces in values
- Only US-ASCII excluding control chars, whitespace, quotes,
  commas, semicolons, backslashes
- Special chars must be percent-encoded (`%27` apostrophe, `%20` space)

---

## 7. Privacy / PII Defaults

| Content                                    | Included by default? | Gate to enable |
|--------------------------------------------|----------------------|----------------|
| User prompt text                           | No (length only)     | `OTEL_LOG_USER_PROMPTS=1` |
| Tool input arguments (paths, commands, patterns) | No              | `OTEL_LOG_TOOL_DETAILS=1` |
| Tool input/output content bodies           | No                   | `OTEL_LOG_TOOL_CONTENT=1` (traces only; 60 KB truncation) |
| Full Messages API request/response JSON    | No                   | `OTEL_LOG_RAW_API_BODIES=1` or `=file:<dir>` |
| Raw file contents                          | **Never** (by design) | (no gate exists) |
| Code snippets                              | **Never** (by design) | (no gate exists) |
| Extended-thinking content                  | **Always redacted** regardless of flags | (no gate exists) |

**`user.email`** is included by default when authenticated via
OAuth. Organizations concerned about email exposure must filter at
the collector/backend, not at CC.

**No documented mechanism** for suppressing org names, user IDs,
or session IDs other than the per-attribute cardinality flags
(`OTEL_METRICS_INCLUDE_*`).

---

## 8. Beta vs GA Status

| Component                          | Status | Stability guarantee |
|------------------------------------|--------|---------------------|
| Metrics                            | GA     | Stable schema       |
| Log events                         | GA     | Stable schema       |
| Basic trace structure              | GA     | Stable              |
| **Distributed tracing details**    | Beta   | "Span names and attributes may change between releases" |
| **Detailed hook tracing**          | Beta   | Additional attributes "not part of the stable span schema" |

Tracing requires `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`. Detailed
hook tracing requires `ENABLE_BETA_TRACING_DETAILED=1` plus
`BETA_TRACING_ENDPOINT` set, and (in interactive CLI only)
organization allowlisting.

---

## 9. Known Limitations

| Limitation | Impact |
|------------|--------|
| Spans dropped if collector is slow | Lower export intervals (e.g., 1000 ms) and ensure collector readiness for short-lived calls |
| Flush timeout default 5 sec on exit | Pending data lost if process killed before flush |
| Shutdown timeout default 2 sec | Increase via `CLAUDE_CODE_OTEL_SHUTDOWN_TIMEOUT_MS` if metrics dropped at exit |
| Cost metrics are approximations | For billing, use Anthropic Console / Bedrock / Vertex AI |
| `console` exporter incompatible with Agent SDK | Corrupts SDK message channel; use a local collector instead |
| Per-retry attempts not separately logged | Only final `api_error` after retries exhausted; `attempt` count is on the event |
| `TRACEPARENT` inbound propagation ignored in interactive CLI | Honored only in SDK and `-p` sessions; prevents accidental inheritance from CI |

---

## 10. Implications for Coolant

### Reconciliation surface (much richer than initial spec assumed)

Coolant gets THREE comparison axes against CC OTEL, not one:

1. **Metrics axis:** counts/cost/tokens by `query_source`, `model`,
   `session.id`. Best for windowed totals and cardinality drift.
2. **Log events axis:** per-API-request cost+tokens (`claude_code.api_request`),
   per-tool-call success/duration (`claude_code.tool_result`),
   per-decision (`claude_code.tool_decision`). Correlated by
   `prompt.id` to a user prompt. Best for per-agent attribution.
3. **Traces axis:** hierarchical spans showing tool execution time,
   subagent delegation chains, hook execution. Best for
   per-agent-call latency and "where did the time go" analysis.

### Coolant's added value (narrowed but still real)

- CC OTEL knows: tokens / cost / latency / tool outcomes, attributed
  by `query_source` (main / subagent / auxiliary), `model`,
  `session.id`, `prompt.id`.
- Coolant adds: per-AGENT-ID attribution within `query_source=subagent`
  (CC OTEL doesn't carry the specific subagent's coolant agent_id),
  cross-session aggregates, leaderboards, the JSONL durability story
  on top of `$TMPDIR` ephemerality.

### Load-bearing gotchas for the adapter

1. Traces require **both** `CLAUDE_CODE_ENABLE_TELEMETRY=1` AND
   `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`. Single var = metrics + logs only.
2. CC's OTLP default is gRPC :4317; coolant's Prometheus is HTTP :9090.
   Enable script MUST force `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`
   and the metrics-specific endpoint.
3. Metrics export at 60s, logs/traces at 5s. A 60s reconciliation
   cadence aligns with metrics; logs/traces could reconcile much more
   often.
4. `OTEL_LOG_*` flags are independent content gates — enabling one
   doesn't enable others. Reconciliation MUST handle each
   independently.
5. `query_source` on cost/token metrics differentiates main /
   subagent / auxiliary in a single attribute — coolant must
   recognize this dimension and not double-count.
6. `prompt.id` is the natural correlation key for log-event
   reconciliation, NOT `session.id`. Coolant's JSONL doesn't
   currently carry `prompt.id`; bridging may require a hook-side
   capture (separate spec; not this one).

---

## Source URLs (re-check before relying on this file)

- https://code.claude.com/docs/en/monitoring-usage
- https://code.claude.com/docs/en/agent-sdk/observability
- https://code.claude.com/docs/en/env-vars
