# Thermal Enterprise: Product & Architecture Spec (v2)

**Status:** Draft v2 — synthesized from five-persona panel (platform
architect, sales engineer, security/compliance, enterprise buyer, startup
buyer). Supersedes v1 (archived at `archive/spec-v1.md`).

**One-liner:** Thermal enriches Claude Code's native telemetry with
system state, gate intelligence, and agent lifecycle context that token
data alone cannot provide. Three products — Free, Cloud, Enterprise —
serve individual developers, startups, and Fortune 500 platform teams
through a progressive value ladder where every step is independently
useful.

---

## 1. The Insight

Claude Code already emits rich OTEL telemetry: per-request token counts,
`cost_usd`, model attribution, session correlation, and user identity —
all via standard `OTEL_*` env vars. Local JSONL session files carry the
same data with zero configuration.

**Thermal is not the token/cost authority. Claude Code is.** Thermal's
value is what it adds on top: system state (CPU/MEM/threat level), gate
intelligence (suppression ROI), and agent lifecycle semantics
(fresh/stale/ghost, agent-hours). The fusion of "what Claude costs" with
"what it does to your machine and what it prevents" is the enterprise
product.

The v1 spec's "blocked on external data source" framing for cost
attribution is obsolete. Token data is available today. The question is
not "can we get it" but "what do we add to it that nobody else can."

---

## 2. Product Architecture

### Three products, one progressive ladder

| Product | Customer | Infra required | Price | GTM |
|---------|----------|---------------|-------|-----|
| **Thermal Free** | Individual developers | None | Free forever | Bottom-up PLG |
| **Thermal Cloud** | Startups, shadow IT, enterprise fiefdoms | None (hosted) | $5-8/dev/month | Self-serve, credit card |
| **Thermal Enterprise** | Fortune 500 platform teams | Their own OTEL/Grafana | $8-15/dev/month | Advisor-channel (Todd in the room) |

All three products share the same data model and converge on the same
enterprise deal through different doors:

1. **Bottom-up PLG:** Developer installs free Thermal → loves it → tells
   team → statusline cost creates champions
2. **Shadow IT:** Team lead signs up for Cloud with a credit card → shows
   numbers in leadership meeting → Trojan horse into enterprise deal
3. **Top-down enterprise:** Todd in the room → CTO demo → platform team
   evaluates → self-hosted deployment

### What each product includes

| Capability | Free | Cloud | Enterprise |
|------------|------|-------|------------|
| Terminal dashboard (thermo TUI) | Yes | Yes | Yes |
| All themes + animations | Yes | Yes | Yes |
| Gate system (test caps, build suppression) | Yes | Yes | Yes |
| Local JSONL event logging | Yes | Yes | Yes |
| Cost statusline (list rates, local JSONL) | Yes | Yes | Yes |
| Hosted dashboard (per-dev cost, cache efficiency) | — | Yes | — |
| OTEL enrichment export (thermald daemon) | — | — | Yes |
| Fleet labels (team, project, cost_center) | — | Via dashboard settings | Via config/MDM |
| Enterprise negotiated rates | — | — | Yes |
| Pre-built Grafana dashboards | — | — | Yes |
| Alerting rule templates | — | — | Yes |
| Compliance documentation package | — | — | Yes |
| `--otel-status` diagnostic | — | — | Yes |

**Free is the PLG engine.** The local experience — dashboard, gates, cost
statusline — stays free forever. Cost consciousness is free because it
creates the internal champions who drive enterprise adoption.

### Pricing

| Tier | Price | Billing | How to buy |
|------|-------|---------|------------|
| Free | $0 | — | Plugin marketplace / install script |
| Cloud | $5-8/dev/month | Monthly, credit card | Self-serve website, no sales call |
| Enterprise (Team) | $8/dev/month | Annual | Self-serve or sales |
| Enterprise (Full) | $12/dev/month | Annual | Sales, includes compliance docs |
| Enterprise (Site) | Custom | Annual | Negotiated, 1000+ seats |

Cloud has a free tier: 5 developers, 7 days retention. Enough to try.

---

## 3. Progressive Value Ladder

The organizing principle. Each rung is independently valuable and
naturally motivates the next. No rung requires the one above it.

### Rung 0: Free Thermal (plugin + dashboard)

**What:** Developer installs the plugin and thermo binary.

**Value:** Machine doesn't melt. Gate system caps parallel test runners,
suppresses redundant builds. Beautiful dashboard in tmux.

**Components:** Open-source repo. No enterprise code. No configuration
beyond install.

**Security surface:** Process tables, gate event JSONL. Local only, zero
outbound data. Fast-track CISO approval (days).

### Rung 1: Cost statusline

**What:** One config line: `cost = true`. Statusline shows per-session
cost.

**Value:** "I can see what I'm spending." Every developer who sees their
cost talks about their cost. This is the champion creation moment.

**Components:** Statusline reads local session JSONL
(`~/.claude/projects/`), parses `usage` fields, multiplies by published
rates. Zero OTEL. Zero daemon. Zero enterprise code.

**Data source:** Session JSONL whitelist parser extracts only
`usage.input_tokens`, `usage.output_tokens`,
`usage.cache_read_input_tokens`, `usage.cache_creation_input_tokens`,
`timestamp`, `session_id`, `model`. Conversation content is never
allocated. See [Security: Session JSONL Access](#session-jsonl-access).

**Display:** Default shows token counts. Dollar display requires
`cost_display = "dollars"` opt-in. Redaction mode
(`cost_display = "redacted"` or `COOLANT_COST_DISPLAY=redacted`) for
screen-sharing.

**Security surface:** Reads session JSONL (Restricted data at file level,
but only Internal fields extracted). Dollar amounts on screen
(Confidential). No outbound data. CISO approval: 1-2 weeks with parser
documentation.

### Rung 2: Claude Code OTEL (fleet visibility, zero Thermal enterprise)

**What:** Platform team pushes env vars (MDM or `.envrc`). Claude Code
emits native OTEL to the org's collector (Enterprise) or Thermal's
hosted endpoint (Cloud).

```
CLAUDE_CODE_ENABLE_TELEMETRY=1
OTEL_LOGS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=<collector or hosted endpoint>
```

For Cloud: `install.sh` handles this — "Got a Thermal Cloud team? Paste
your endpoint (or press enter to skip)."

**Value:** Per-developer token cost attribution in Grafana (Enterprise)
or hosted dashboards (Cloud). Cache efficiency by developer. Model usage
breakdown. This is ~80% of cost visibility with zero new software on
machines.

**Components:** Claude Code's own OTEL. Thermal provides documentation,
collector config templates, and Grafana dashboard JSONs for Claude Code
metrics — shipped in the open repo as `docs/fleet-otel-quickstart/`.
For Cloud, Thermal hosts the collector and dashboards.

**Key data available (from Claude Code's `api_request` log events):**
`input_tokens`, `output_tokens`, `cache_read_tokens`,
`cache_creation_tokens`, `cost_usd`, `model`, `session.id`,
`user.account_id`, `duration_ms`. Emitted per-request at ~5s latency.

**Security surface:** Anthropic's security story, not Thermal's. The
CISO is approving Claude Code's telemetry export. Thermal is not in the
data path at this rung.

### Rung 3: Thermal daemon (enrichment — first enterprise code)

**What:** Platform team deploys `thermald` fleet-wide (MDM push). The
daemon collects system state, tails session JSONL for token enrichment,
and emits fused OTEL metrics alongside Claude Code's own stream.

**Value:** "You have the data. You're missing the context." The daemon
adds what Claude Code OTEL cannot see: CPU/MEM/threat level correlated
with cost spikes, gate suppression ROI ("prevented 847 redundant tsc
builds"), agent lifecycle (concurrent agent counts, agent-hours),
process categories.

**This is the enterprise paywall.** The first rung requiring proprietary
code from the private repo.

**Components:** `thermald` binary, launchd plist, OTEL export. Session
JSONL tailing for token enrichment. IPC socket for TUI connection.

**Security surface:** Persistent daemon with Restricted data access
(session JSONL) and outbound OTEL. Full CISO review: 4-8 weeks. See
[Security: Daemon Threat Model](#daemon-threat-model).

### Rung 4: Enterprise rates + fleet labels

**What:** Configure negotiated Anthropic rates and fleet identity labels
(team, project, cost_center).

**Value:** "We know exactly what each team costs, at our real rate."
Makes enterprise discounts tangible to every developer. Enables
per-team, per-project cost attribution in Grafana.

**Components:** Config only — no new binary components. Rates via
MDM-pushed env vars or managed config (not user-editable TOML).

**Security surface:** Incremental to Rung 3. Negotiated rates are
Confidential. CISO approval: 1-2 weeks.

### Rung 5: Full integration

**What:** Compliance docs, alerting rules, pre-built Grafana dashboards,
production deployment polish.

**Value:** "This is our Claude platform monitoring solution."

**Components:** Shipped as artifacts in the private repo. Grafana
dashboards join `thermal.*` and `claude_code.*` metrics.

**Security surface:** Operational review (1 week). Dashboard access
control and alerting channel confidentiality.

---

## 4. Architecture

### Two-emitter model

Claude Code and Thermal emit OTEL to the same collector. They are
independent streams with independent configurations. Grafana correlates
them via `session.id`.

```
Claude Code ──OTEL──→ Customer's collector ──→ Grafana
                              ↑
Thermal daemon ──OTEL──→ ─────┘
  ↑ reads:
  ├── OS data (CPU, MEM, SWAP, GPU, processes)
  ├── Gate event JSONL ($TMPDIR/coolant-$USER.events.jsonl)
  └── Session JSONL (~/.claude/projects/) — usage fields only
```

**Namespace separation:** Claude Code uses `claude_code.*`. Thermal uses
`thermal.*`. No collisions. A unit test in the enterprise repo asserts
all registered metric names start with `thermal.`.

**Correlation key:** `session.id` joins the two streams. Thermal learns
session IDs from JSONL filenames and hook events (which carry
`session_id` on stdin).

**Cardinality rule:** `session.id` appears ONLY on Thermal's log events
(for correlation queries), NEVER on metrics (prevents Prometheus storage
explosion). Claude Code's `OTEL_METRICS_INCLUDE_SESSION_ID` is their
decision; Thermal does not replicate it.

**Thermal does not duplicate or re-emit Claude Code's metrics.** No
`thermal.tokens.*`, no `thermal.cost.usd` that could be confused with
`claude_code.cost.usage`. Thermal ingests Claude Code data locally (for
TUI enrichment and daemon correlation) but never re-exports it. Thermal
emits only what it uniquely observes.

**Local ingestion for TUI enrichment:** The daemon may consume Claude
Code's OTEL signals locally to avoid round-tripping token data to a
remote collector and back for the developer's TUI. This is a local
optimization — the daemon ingests for its own enrichment, never
re-exports Claude Code's data to the fleet collector. Open design
question: whether the daemon runs a local OTEL receiver or reads
session JSONL is an implementation detail; the architectural rule is
that Claude Code's data flows to the fleet collector directly from
Claude Code, not through Thermal.

### Repo split: open core + private enterprise

**Open repo (coolant) — MIT licensed:**
- `internal/model/sink.go` — `MetricSink` interface (the public seam)
- `cmd/thermal/otel_stub.go` — nil stub, always compiled in free builds
- All dashboard, theme, animation, collector, gate code
- `claude-statusline/` — cost statusline (reads session JSONL)
- `docs/fleet-otel-quickstart/` — Claude Code OTEL setup guides,
  collector configs, Grafana dashboards for Claude Code native metrics

**Private repo (thermal-enterprise):**
- `thermald` binary — headless daemon (collector + model + OTEL sink)
- `internal/otel/` — OTEL implementation, metric instruments, closed
  attribute enforcement, transport security
- Session JSONL tailer and token enrichment
- IPC server for TUI connection
- Pre-built Grafana dashboards (joining `thermal.*` + `claude_code.*`)
- Alerting rule templates
- Compliance documentation package

Enterprise builds vendor the private repo as a Go module dependency.

```go
// cmd/thermal/otel_stub.go (open repo, always present)
//go:build !enterprise

func initOTEL(cfg config.OTELConfig) (model.MetricSink, error) { return nil, nil }
```

```go
// cmd/thermal/otel_enterprise.go (open repo, imports private module)
//go:build enterprise

import "github.com/todd-w-shaffer/thermal-enterprise/otel"

func initOTEL(cfg config.OTELConfig) (model.MetricSink, error) {
    return otel.New(cfg)
}
```

### Binaries

| Binary | Source | Size | Dependencies |
|--------|--------|------|-------------|
| `thermo` (free TUI) | Open repo | ~15MB | bubbletea, lipgloss, harmonica, go-colorful |
| `thermald` (enterprise daemon) | Private repo | ~18-20MB | Collector, model, OTEL SDK, protobuf. No bubbletea. |

`thermald` is a separate binary, not `thermo --daemon`. Different
lifecycle (launchd-managed), different dependencies (no TUI), different
release cadence.

### Daemon architecture

**What it runs:** Same collector goroutines as thermo (fast 150ms + slow
1s + event tailer 500ms), same `AppState` computation, with three
additions:

1. **MetricSink** — OTEL export via `PeriodicReader` (10s interval)
2. **Session JSONL tailer** — discovers and tails active session files
   for token enrichment
3. **IPC listener** — Unix domain socket serving `DaemonState` to TUI

**Collector intervals can be relaxed** for fleet deployment where
sparkline rendering isn't needed. 1s system metrics is sufficient for
OTEL (vs. 150ms for TUI animation). Configurable.

**Startup:** The daemon starts on Claude Code startup, not system boot.
No point collecting if no tokens are flowing. The `SessionStart` hook
triggers daemon launch if not already running. launchd `KeepAlive`
ensures it stays up for the duration of active Claude Code sessions.

**Process lifecycle:**
- **macOS:** launchd user agent (`~/Library/LaunchAgents/`). `KeepAlive`
  for crash recovery, `ThrottleInterval` 5s for crash-loop prevention.
  Never a system daemon — user-level only.
- **Privilege model:** Runs unprivileged as the developer's UID. No
  root. No Full Disk Access. No TCC entitlements. No elevated
  permissions of any kind. All data sources (process table, sysctl,
  vm_stat, session JSONL, gate JSONL) are readable by the user.
- **Network egress:** Connects ONLY to the configured OTLP endpoint.
  No phone-home. No update checks. No analytics about Thermal itself.
  No connections to Anthropic, GitHub, or any other endpoint.
- **Self-update:** Never. The daemon does not modify its own binary.
  Updates are pushed via MDM (enterprise) or manual download (self-serve).
- **Logs:** `~/Library/Logs/coolant/thermald.log` with 10MB rotation.
- **Shutdown:** SIGTERM trap → `MeterProvider.Shutdown()` with 5s
  deadline → flush final OTEL batch → close IPC → exit 0.
- **Health:** Heartbeat file at `$TMPDIR/coolant-$USER.daemon.pid`.
- **Binary notarization:** macOS builds signed with Developer ID and
  notarized for Gatekeeper. Required for MDM deployment.

### IPC: daemon → TUI

**Transport:** Unix domain socket at
`$TMPDIR/coolant-$USER.daemon.sock`. Length-prefixed JSON (4-byte
big-endian length + JSON payload).

**Payload:** `DaemonState` struct — superset of `Snapshot` + computed
`AppState` summaries + session cost accumulators + OTEL health status.
The TUI in daemon mode is a pure renderer — no collection, no
computation.

```go
type DaemonState struct {
    Snapshot     collector.Snapshot
    AppState     AppStateSummary
    Sessions     []SessionCost
    OTELStatus   OTELHealth
    DaemonUptime time.Duration
    Version      int            // IPC protocol version
}
```

**Versioning:** `Version` field in every message. Same major = proceed.
Different major = TUI falls back to standalone collection. Adding fields
is backward-compatible.

**Fallback:** No daemon socket → TUI runs standalone (current behavior).
No hot-switching — if daemon comes online while TUI runs, TUI stays
standalone until restart.

### Token data paths (defense in depth)

Three independent paths to token data, ordered by API stability:

1. **Claude Code OTEL signals** (preferred) — documented, supported
   integration surface. `api_request` log events with all token fields.
2. **Hooks** (`Stop` hook → parse `transcript_path`) — documented API.
   End-of-turn granularity, sufficient for OTEL batching.
3. **Session JSONL tailing** (fallback) — undocumented, used by
   community tools, breakage risk. Medium-High stability risk.

The daemon should prefer (1), fall back to (2), and use (3) only when
neither OTEL signals nor hooks are available. The statusline uses (3)
directly for real-time per-request cost (hooks fire only at end-of-turn).

### Session JSONL tailing mechanics

**Discovery:** Glob `~/.claude/projects/*/*.jsonl`. A session is "active"
if modified within the last 60s. Discovery goroutine scans periodically,
maintains map of active files → tailer goroutines.

**Tailing:** Same offset-tracking pattern as `events.go`. Seek past
previously-read bytes, parse new lines, handle partial writes. The
existing `TailEvents` core logic should be extracted into a generic
`JsonlTailer` reusable by both event and session tailers.

**Multi-session:** One goroutine per active session file. Files not
modified for 5 minutes have their tailers shut down.

**Schema versioning:** Maintain a schema map per Claude Code version.
If the `usage` object schema changes across Claude Code releases, the
parser adapts based on detected fields — not a rigid contract, but a
version-aware extraction. On unrecognized schema, log structured
warning and degrade to proxy-only metrics (agent-hours, gate ROI).
Feature flag `[daemon] jsonl_tailing = true` allows disabling without
a binary update.

**Confirmed working:** Claude Code OTEL with Bedrock backends is
validated at fleet scale in production customer environments. Token
fields are identical regardless of backend (Anthropic API, Bedrock,
Vertex, Azure AI Foundry).

### Performance budget

| Concern | Impact | Detail |
|---------|--------|--------|
| `Record()` calls | ~2-3us/tick | ~15 instruments, non-blocking writes to aggregation |
| Memory | ~20KB steady-state | ~100 aggregation cells at ~200 bytes each |
| Export I/O | Off hot path | PeriodicReader goroutine, 10s interval, async |
| Endpoint down | Zero impact | Batch dropped, next interval fresh, no buffering |
| Shutdown | 5s deadline | Final batch flush on clean exit |

### Go OTEL SDK packages

Metrics only. No traces, no logs.

- `go.opentelemetry.io/otel/sdk/metric` — MeterProvider, PeriodicReader
- `go.opentelemetry.io/otel/metric` — instrument types
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`
- gRPC optional behind `otel_grpc` build tag

---

## 5. Metric Catalog

Thermal emits only what it uniquely observes. Token/cost metrics are
Claude Code's authority — Thermal does not duplicate them.

### System metrics

| Metric | Type | Unit |
|--------|------|------|
| `thermal.cpu.utilization` | Float64Gauge | percent |
| `thermal.memory.used` | Int64Gauge | bytes |
| `thermal.memory.total` | Int64Gauge | bytes |
| `thermal.memory.utilization` | Float64Gauge | percent |
| `thermal.memory.headroom` | Int64Gauge | bytes |
| `thermal.memory.decompressions` | Int64Gauge | count |
| `thermal.swap.used` | Int64Gauge | bytes |
| `thermal.swap.total` | Int64Gauge | bytes |
| `thermal.gpu.utilization` | Float64Gauge | percent |

### Agent lifecycle

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.agents.active` | Int64Gauge | count | `state={fresh,stale}` |
| `thermal.agents.completed` | Int64Counter | count | — |
| `thermal.agents.spawn_rate` | Float64Gauge | per_second | — |
| `thermal.agents.death_rate` | Float64Gauge | per_second | — |
| `thermal.agents.duration_seconds` | Float64Counter | seconds | — |
| `thermal.sessions.active` | Int64Gauge | count | — |

### Threat level

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.threat.level` | Int64Gauge | level | `name={COOL,WARM,HOT,MELTDOWN}` |

Dual encoding (numeric 0-3 + name attribute) enables Grafana numeric
thresholds AND label-based annotations.

### Process categories

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.processes.count` | Int64Gauge | count | `category={build,shell,node,go,python,rust,swift}` |
| `thermal.processes.total` | Int64Gauge | count | — |

### Gate events (the moat)

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.gate.suppressions` | Int64Counter | count | `tool={tsc,eslint,vitest,cargo-build,...}` |
| `thermal.gate.caps` | Int64Counter | count | `tool={vitest,jest,go-test,...}` |
| `thermal.gate.time_saved_seconds` | Float64Counter | seconds | `tool` |

`tool` is a **closed set** from gate.sh's known list. Unknown tools
bucket to `other`. `time_saved_seconds` = suppression count × configured
average duration per tool.

**Gate suppression ROI is the headline enterprise metric.** "Thermal
prevented 847 redundant tsc invocations this week, saving 12 hours of
CPU time." This is unique to Thermal — nobody else can produce it.

### Network

| Metric | Type | Unit |
|--------|------|------|
| `thermal.api.online` | Int64Gauge | boolean |

### Daemon health

| Metric | Type | Unit |
|--------|------|------|
| `thermal.daemon.uptime` | Float64Gauge | seconds |
| `thermal.otel.exports.success` | Int64Counter | count |
| `thermal.otel.exports.failed` | Int64Counter | count |

### Cache efficiency (derived, not emitted)

Cache hit ratio = `cache_read_tokens / (cache_read_tokens + input_tokens)`
per developer. Computed in Grafana/Cloud dashboards from Claude Code's
OTEL data, not emitted as a Thermal metric.

- **Individual:** Free in statusline ("your cache hit rate this session")
- **Fleet comparison:** Cloud/Enterprise dashboards ("Jamie is at 4%,
  team average is 47% — coaching opportunity")

A team at 90% cache hits is using Claude well (warm sessions, good
prompt structure). A team at 4% is burning money on cold starts.

### What Thermal does NOT emit

- `thermal.tokens.*` — Claude Code is authoritative
  (`claude_code.token.usage`)
- `thermal.cost.usd` — Claude Code is authoritative
  (`claude_code.cost.usage`)
- `session.id` on any metric — cardinality bomb. Log events only.

---

## 6. Configuration

### TOML config block

```toml
[otel]
endpoint = "https://otel-collector.corp.example.com:4318"
# That's the flip. One line enables export.

# Optional — progressive disclosure:
# protocol = "http"        # "http" (default) or "grpc"
# interval = "10s"         # export batch interval
# timeout  = "10s"         # per-export timeout
# ca       = "path/to/ca.pem"           # custom CA
# cert     = "path/to/client.pem"       # mTLS client cert
# key      = "path/to/client-key.pem"   # mTLS client key

[otel.labels]
team = "platform"
environment = "staging"
# cost_center = "eng-tools"

[cost]
enabled = true              # show cost in statusline (free tier)
# cost_display = "dollars"  # "tokens" (default) | "dollars" | "redacted"
# cpu_hour_rate = 0.50      # $/hr for gate ROI calc (enterprise)
```

Enterprise negotiated rates are NOT in this file. They are pushed via
MDM environment variables or managed read-only config. See
[Security: Rate Config](#rate-config-access-control).

### Env var precedence

```
OTEL_EXPORTER_OTLP_ENDPOINT  >  config.toml [otel] endpoint  >  disabled
OTEL_EXPORTER_OTLP_HEADERS   >  (no config equivalent — secrets never in files)
COOLANT_OTEL=0               >  everything (kill switch)
```

### Kill switch

`COOLANT_OTEL=0` immediately disables all Thermal OTEL export regardless
of config. Allows incident response to halt data flow fleet-wide via MDM.

### Auth tokens

**Never in config files.** Sourced from:
- `OTEL_EXPORTER_OTLP_HEADERS` (standard OTEL)
- `auth_header_env = "MY_TOKEN_VAR"` in config (reads named env var)

Literal auth values in config trigger a startup warning.

### Protocol default: HTTP/protobuf

- Works through corporate proxies blocking HTTP/2
- Every OTEL collector accepts `http/protobuf` on `/v1/metrics`
- No `google.golang.org/grpc` dependency (~8MB savings)
- gRPC behind `otel_grpc` build tag

---

## 7. Security

### Transport security

- Remote endpoints: TLS required. `http://` refused with clear error.
- Localhost (`127.0.0.1`, `::1`, `localhost`): plaintext allowed
  **automatically** (no config field needed — hostname is sufficient).
- mTLS supported via `cert`/`key`/`ca` config fields.
- Private key files must be `0600`. Refused on group/world-readable.

### Closed attribute set

OTEL SDK auto-resource-detection **disabled**. Only explicitly listed
attributes emitted.

**Always present:** `service.name=thermal`, `service.version`,
`os.type`, `host.arch`.

**Opt-in:** `host.name`, `user`, `team`, `environment`, `cost_center`.

**Never emitted:** `GateEvent.Command/Original/Rewritten` (Restricted),
process `Comm` raw values, `CollectErrs` strings, `prompt.id`,
`user.email`, `user.account_uuid`. Enforced by unit test.

### Session JSONL access

The daemon reads session JSONL files containing full conversation
transcripts — Restricted data. **Architectural enforcement of
field-level extraction is the single most important security control.**

**Whitelist parser:** Typed Go struct extracts ONLY:
- `usage.input_tokens`, `output_tokens`, `cache_read_input_tokens`,
  `cache_creation_input_tokens`
- `type`, `timestamp`, `session_id`, `model`

Conversation content is never allocated as Go strings. Go's
`encoding/json` ignores fields not in the target struct.

**Enforcement:**
1. Unit test asserts no Restricted-classified fields in the struct
2. No raw byte retention beyond JSON decoder buffer
3. No debug logging of session content
4. Core dump disabled (`RLIMIT_CORE=0` in launchd plist)
5. CODEOWNERS on parser file — any change requires security review

### Daemon threat model

**Privilege:** User-level launchd agent. No root. No elevated
permissions. Installation must refuse system daemon placement.

**Top 5 risks:**

| Risk | Severity | Mitigation |
|------|----------|------------|
| Conversation leakage via parser defect | Critical | Whitelist parser, unit test, CODEOWNERS, no debug dumping, RLIMIT_CORE=0 |
| Daemon as persistent exfiltration agent | Critical | Binary integrity (code signing), endpoint allowlisting, closed attribute set, anomaly detection on export volume |
| Command string exfiltration via OTEL | Critical | Architecturally excluded, unit test enforced |
| Config redirect to attacker endpoint | High | Endpoint allowlisting via MDM, file permissions, startup logging |
| Crafted session JSONL exploiting parser | High | JSON decoder size limits (1MB/line), skip malformed lines, fuzz testing |

### Rate config access control

Negotiated pricing is Confidential (NDA-protected). Must NOT live in
user-editable TOML.

- **Rung 1 (statusline):** User configures public list prices in
  personal config. Acceptable — these are public.
- **Rung 4 (enterprise):** Rates pushed via MDM environment variables or
  managed read-only config. Not in user-writable files.

### Config file security

- `config.toml` should be `0600`. OTEL config refused from group/world-
  readable files.
- Endpoint allowlisting via `COOLANT_OTEL_ALLOWED_ENDPOINTS` env var
  (MDM-controlled).
- Optional `config_hash` validation for MDM-pushed configs.

### Per-rung CISO approval timeline

| Rung | Surface | Approval | Timeline |
|------|---------|----------|----------|
| 0 | Local tool, no outbound | Standard plugin approval | Days |
| 1 | + Session JSONL read (usage fields only) | Parser documentation review | 1-2 weeks |
| 2 | Claude Code's OTEL (Anthropic's story) | Claude Code approval, not Thermal's | Varies |
| 3 | Persistent daemon + OTEL export | Full security review | 4-8 weeks |
| 4 | + Negotiated rates, fleet labels | Config/policy review | 1-2 weeks |
| 5 | + Dashboard access, alerting channels | Operational review | 1 week |

**Rungs 0-2 can proceed while Rung 3 is in security review.** This is
by design — teams get value immediately while the daemon goes through
the approval pipeline.

### Compliance documentation (ships with enterprise)

1. Data flow diagram (both OTEL streams + correlation surface)
2. Attribute inventory with classifications
3. Privacy Impact Assessment template (GDPR, pre-filled)
4. Security questionnaire answers (SIG Lite / CAIQ)
5. Incident response runbook for credential compromise
6. Daemon-specific controls: parser enforcement, binary integrity,
   plist security, session access scope

### Gate criteria (20 items)

**Carried from v1 (1-10):** Closed attributes, TLS, no secrets in
config, endpoint allowlisting, command strings excluded, hostname/user
opt-in, compliance docs, dependency hygiene, audit logging, kill switch.

**New for daemon (11-20):** Session JSONL whitelist parser with test, no
raw session data retention, unprivileged daemon, binary integrity
verification, plist permission enforcement, dual-stream documentation,
rate config access control, cost display redaction, session access
scoping (active only, no historical), daemon lifecycle logging.

---

## 8. The Demo

### "You have the data. You're missing the context."

**Beat 1: Peripheral vision (0:00–5:00)**
Todd advises on Claude adoption. Thermal runs in tmux. Nobody mentions
it.

**Beat 2: The question (5:00–5:30)**
"What's that at the bottom?" — "Oh, Thermal. Watches what Claude's
doing so I don't have to." Back to main topic.

**Beat 3: The casual demo (8:00–10:00)**
Spawn three parallel agents. Dashboard lights up. Point at statusline
cost. "That's my session cost, real-time. I negotiated 7% off with
Anthropic, so that's my actual rate." Point at gate count. "It just
killed four redundant tsc invocations."

**Beat 4: The pivot — acknowledge what they have (10:00–12:00)**
"You're probably already collecting Claude Code OTEL. Most of my
customers are. So you can see token spend per developer. Great. But
answer me this: when a developer burns $200 in a morning, do you know
*why*? Was it cold cache thrashing? Runaway agents? A build loop that
should have been suppressed? You can see the cost. You can't see the
cause."

**Beat 5: The enrichment demo (12:00–15:00)**
Grafana, two panels side by side:
- Left: Claude Code's `cost_usd`. Developer X spent $47 today.
- Right: Thermal's enrichment. Same developer. 14 agents, 8 concurrent
  at peak, 340 gate suppressions, MELTDOWN twice, 2GB headroom. Gate
  time saved: 45 minutes.

"The left tells you what they spent. The right tells you why, and what
Thermal prevented."

**Beat 6: The close (15:00–16:00)**
"The dashboard is free. Cost statusline is free. The enrichment layer
that tells your platform team *why* costs are what they are — that's
the enterprise license. Your developers install it today. Your platform
team sees the value next week."

---

## 9. Grafana Dashboards

Ship as provisioned JSON in private repo. Templated datasource
variables. Designed to query BOTH `thermal.*` and `claude_code.*` metric
namespaces via `session.id` correlation.

### Design philosophy

- **Dark theme only.** Matches terminal dashboard.
- **Classic severity colors.** Exact hex from `theme/classic.go`.
- **Sparkline-heavy, not number-heavy.** Time-series panels with fill.
- **Layout mirrors terminal hierarchy.**
- **Two datasources, one story.** Claude Code OTEL for token/cost.
  Thermal OTEL for system context and gate ROI. Joined at query time.

### Dashboard 1: Fleet Overview (platform team)

- **Row 1:** Total active agents, machines in MELTDOWN/HOT, gate
  suppressions today, fleet cost today (from `claude_code.cost.usage`)
- **Row 2:** Heat map — machines by threat level, click-through
- **Row 3:** Fleet-wide agent count (24h), CPU/MEM p50/p95
- **Row 4:** Top machines by agent count, with team/project labels

### Dashboard 2: Cost & Efficiency (engineering managers)

- **Row 1:** Agent-hours this week, gate time saved, fleet cost (from
  Claude Code), cache efficiency fleet average
- **Row 2:** Cost by team (from `claude_code.cost.usage` grouped by
  Thermal's team labels)
- **Row 3:** Gate suppressions by tool type — which tools suppressed most
- **Row 4:** Per-developer: cost (Claude Code) + agent count + gate
  suppressions + threat distribution (Thermal)

### Dashboard 3: Machine Deep Dive (drill-down)

- **Row 1:** Threat gauge, agents, CPU/MEM/SWAP (mirrors terminal)
- **Row 2:** CPU/memory/swap/decompressions (24h) with threat bands
- **Row 3:** Gate event log with timestamps
- **Row 4:** Agent timeline (Gantt-style, colored by threat level)

---

## 10. Thermal Cloud

The hosted product for startups, shadow IT, and Todd's personal demo
weapon. Detailed spec forthcoming in `cloud-spec.md`. Startup buyer
evaluation at [v2-startup-buyer.md](v2-startup-buyer.md).

**Dogfood-first:** Build this for Todd's own use. A personal spend
dashboard that runs against his real Claude Code usage. The demo flow:
customer sees the statusline, asks about it, Todd opens his phone and
shows his personal Thermal Cloud dashboard. Jaws drop. "Want one for
your team? Here's the link."

### Architecture

Thermal hosts an OTEL collector endpoint. Customers configure Claude
Code to emit to it. Thermal stores metrics, computes dashboards, serves
a web UI.

```
Developer machine                  Thermal Cloud
─────────────────                  ──���──────────
Claude Code ──OTEL──→              OTEL collector
                                        │
(Optional) Thermal plugin hooks ──→     │
  gate events, agent lifecycle          ▼
                                   Metric store
                                        │
                                        ▼
                                   Web dashboard
                                   (app.thermal.dev)
```

### Onboarding (under 5 minutes)

1. Sign up with email. Get API key + endpoint URL + dashboard link.
2. Share with team: install script handles env var wiring ("Got a
   Thermal Cloud team? Paste your endpoint.")
3. First data appears within 10 minutes.

### Dashboard design

Business language, not OTEL jargon. "Your team spent $2,847 this week"
not `claude_code.token.usage{type="input"}`. Per-developer cost, cache
efficiency, usage trends, actionable insights. Accessible from phone.

### Data boundary

Hosted endpoint receives ONLY Claude Code's default OTEL payload (token
counts, cost, model, session ID, user ID, duration). Prompts, code, tool
outputs are off by default (`OTEL_LOG_USER_PROMPTS` and
`OTEL_LOG_TOOL_DETAILS` default to off). Setup instructions never enable
them. Privacy policy explicitly states: "We receive aggregate usage
metrics. We do not receive prompts, code, or conversation content."

### Enrichment tier

If a developer also installs the Thermal plugin, gate events and agent
lifecycle data can optionally flow to the hosted endpoint, adding:
suppression counts, agent-hours, threat distribution. Additive, not
required. The baseline works with zero Thermal installation.

---

## 11. Implementation Sequence

Organized by progressive value rung. Each phase ships independently.

### Phase 0: Rung 1 — Cost statusline (open repo, free tier)

1. Generic `JsonlTailer` extracted from `events.go` pattern
2. Session JSONL discovery (`resolveActiveSessions`)
3. Whitelist parser with typed extraction struct + unit test
4. Rate config TOML `[cost]` block in config parser
5. Statusline cost display (tokens default, dollars opt-in, redaction)
6. Tests: JSONL tailing with schema changes, rate calculation,
   multi-session, parser security

**Ships as:** Open repo release. PLG flywheel starts.

### Phase 1: Rung 2 — Fleet OTEL quickstart (open repo, docs only)

7. `docs/fleet-otel-quickstart/` — Claude Code OTEL setup guide
8. Grafana dashboard JSONs for Claude Code native OTEL metrics
9. Collector config templates (Alloy, Datadog Agent, vanilla OTEL)

**Ships as:** Open repo documentation. Zero enterprise code.

### Phase 2: Rung 3 — Enterprise daemon plumbing (private repo)

10. `MetricSink` interface in open repo
11. Nil stub in open repo
12. Enterprise `MetricSink` implementation in private repo
13. System metrics + agent lifecycle + threat + gate counters
14. `thermald` binary — headless collector + OTEL emitter
15. launchd plist template with KeepAlive + resource limits
16. Session JSONL tailer in daemon (reuses Phase 0 code)
17. Token enrichment: fuse session token data with system state
18. Build tag isolation

### Phase 3: IPC + TUI integration (private repo)

19. `DaemonState` struct
20. UDS IPC server in daemon
21. UDS IPC client in TUI (daemon mode)
22. Fallback logic: detect daemon, connect or standalone
23. `--otel-status` diagnostic

### Phase 4: Security hardening (private repo)

24. Closed attribute enforcement with unit test
25. Metric namespace test (all names start with `thermal.`)
26. Transport security (TLS, localhost exception, mTLS)
27. Config file permission checks
28. Endpoint allowlisting
29. Kill switch
30. Binary notarization (macOS Gatekeeper)

### Phase 4.5: Pilot (10 developers, private repo)

31. Deploy daemon to 10 developers (platform team or friendlies)
32. Verify resource consumption matches performance budget
33. Verify OTEL export reliability at sustained load
34. Verify daemon stability (crash recovery, log rotation)
35. Verify Grafana dashboard queries work with real dual-stream data
36. Fuzz test session JSONL parser

**Gate:** Fleet deployment (Phase 5) blocked until pilot validates.

### Phase 5: Fleet polish (private repo)

37. Grafana dashboards (Fleet, Cost, Deep Dive) — dual-namespace
38. Alerting rule templates
39. Fleet labels as resource attributes
40. Enterprise rate overrides
41. `↑` glyph indicator with failure states
42. Compliance documentation package (post-PMF priority)
43. Daemon health metrics

### Phase 6: Thermal Cloud (separate infrastructure — dogfood first)

44. Todd's personal Cloud instance (dogfood the whole flow)
45. Hosted OTEL collector endpoint
46. Metric store + web dashboard
47. Signup flow + API key provisioning
48. Stripe integration
49. Privacy policy + data boundary enforcement
50. Install script integration ("Got a Thermal Cloud team?")

---

## 12. Resolved Design Tensions

| Tension | Resolution |
|---------|------------|
| **Who owns token/cost data** | Claude Code is authoritative. Thermal enriches, never duplicates. |
| **Transport default** | HTTP/protobuf. Localhost exception for dev. |
| **Attribute emission** | Closed set. PII opt-in only. Command strings excluded. |
| **Config vs flags** | OTEL env vars > config > disabled. No CLI flags. |
| **Secrets in config** | Env vars only. Literal tokens trigger warning. |
| **Cost statusline: free or enterprise** | Free. It's the PLG hook. |
| **Dashboard vs daemon** | Two binaries. `thermo` = TUI. `thermald` = headless enrichment. |
| **Self-hosted vs hosted** | Both. Enterprise = self-hosted. Cloud = hosted. Same data model. |
| **Session JSONL tailing risk** | Defense in depth: OTEL > hooks > JSONL. Feature flag to disable. Version-aware schema map. |
| **Negotiated rates in config** | MDM-pushed env vars for enterprise. User TOML for personal list prices only. |
| **Cost display default** | Token counts. Dollars require opt-in. Redaction mode exists. |
| **Re-emit Claude Code data?** | No. Ingest locally for TUI enrichment, never re-export. Claude Code's data flows to fleet collector directly. |
| **Daemon startup trigger** | On Claude Code startup (SessionStart hook), not system boot. No point collecting with no tokens flowing. |
| **Daemon local OTEL consumption** | Open design question. Daemon may run a local OTEL receiver for round-trip avoidance, or read session JSONL. Implementation detail — architectural rule is no re-export. |
| **Cache efficiency: free or enterprise?** | Individual rate in free statusline. Fleet comparison in Cloud/Enterprise dashboards. |
| **Compliance timing** | Post-PMF. Architecture must be sound, but SOC 2 paperwork follows product-market fit. Ship on SOC 2-compliant infra (AWS). |
| **Data at rest posture** | Token counts and system metrics are not crown jewels. Reasonable security, not encryption-lockdown. Session JSONL (conversation content) is the sensitive surface — whitelist parser is the control. |
| **Bedrock fleet-scale OTEL** | Confirmed working in production customer environments. Not a validation gap — it's deployed. |

---

## Appendix: Persona Findings

### v1 panel (archived at `archive/`)

- [Platform Engineer](platform-engineer.md) — SDK, integration, perf
- [Sales Engineer](sales-engineer.md) — Enterprise needs, pricing, demo
- [Product Designer](product-designer.md) — Config UX, dashboard indicator
- [Security & Compliance](security-compliance.md) — Data classification, threats

### v2 panel

- [Platform Architect](v2-platform-architect.md) — Daemon architecture,
  IPC, two-emitter model, build pipeline
- [Sales Engineer](v2-sales-engineer.md) — Revised GTM, pricing tiers,
  progressive value as sales funnel
- [Security & Compliance](v2-security-compliance.md) — Daemon threat
  model, session JSONL whitelist parser, per-rung security
- [Enterprise Buyer](v2-enterprise-buyer.md) — Deployment friction,
  infosec gaps, realistic adoption timeline
- [Startup Buyer](v2-startup-buyer.md) — Hosted dashboard, shadow IT,
  self-serve onboarding

### Reference

- [Origin Story](origin-story.md) — How this feature was born
- [Cost Attribution Analysis](cost-attribution.md) — Customer profiles,
  proxy metrics, data source landscape
- [Perplexity Data Sources](Claude%20Code%20Observability%20%20Token%20%26%20Cost%20Data%20Sources%20(perplexity).md) — Full technical reference
