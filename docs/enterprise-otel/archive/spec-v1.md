# Thermal Enterprise: OTEL Self-Reporting Spec

**Status:** Draft — consensus from platform engineering, sales engineering,
product design, and security/compliance review panels.

**One-liner:** When configured, Thermal emits metrics to any OTLP-compatible
endpoint. Enterprise customers see per-machine, per-team, per-agent
observability in their existing Prometheus/Grafana/Datadog stack. Zero new
infrastructure.

---

## 1. Product Positioning

### Tier split

| | Thermal Free | Thermal Enterprise |
|---|---|---|
| Terminal dashboard | Yes | Yes |
| All themes + animations | Yes | Yes |
| Gate system (test caps, build suppression) | Yes | Yes |
| Local JSONL event logging | Yes | Yes |
| OTEL metric export | No | Yes |
| Cost attribution metrics | No | Yes (when data source exists) |
| Fleet-wide labels (team, project, env) | No | Yes |
| Pre-built Grafana dashboards (JSON) | No | Yes |
| Alerting rule templates (Prometheus YAML) | No | Yes |
| `--otel-status` diagnostic | No | Yes |

Enterprise is a **data export tier**, not a feature-gate tier. The local
experience is identical. Free stays free forever — it is the PLG engine.

### Pricing guidance

$8-15/developer/month, per-seat, annual. Below the AI coding tool itself,
above commodity monitoring. Flat per-seat (not per-machine, not usage-based).
Procurement teams know how to approve this shape.

### Competitive position

Nothing else exists at this layer. Generic APM tools see processes; Thermal
sees agent intent, gate suppressions, and threat semantics. Combined with
the advisor-channel GTM (Todd is in the room when the buying decision
happens), this is an unusually strong moat.

---

## 2. Architecture

### Integration point

Hook into `model.AppState.Update()` via an optional `MetricSink` interface.
After `updateThreatAndAlerts()` returns, all derived state is computed —
threat level, category counts, headroom, rates. This is where OTEL
observations are recorded. Gate events flow through `HandleEvent()`.

```go
// internal/model/sink.go
type MetricSink interface {
    Record(s *AppState, snap *collector.Snapshot)
    RecordEvent(ev collector.GateEvent)
    Shutdown(ctx context.Context) error
}
```

`AppState` gets a `sink MetricSink` field (nil in free tier). At the end of
`Update()`, if non-nil, call `sink.Record()`. The sink implementation lives
in `internal/otel/`. Recording calls are non-blocking — they write to
in-memory aggregation and return in ~2-3 microseconds for ~15 instruments.

### Export path (parallel, never blocking)

```
collector (fast 150ms / slow 1s loops)
    └── model.AppState.Update()
            ├── widgets → terminal render    [existing path]
            └── MetricSink.Record()          [new, enterprise only]
                    └── PeriodicReader (10s) → OTLP HTTP → customer collector
```

The `PeriodicReader` runs its own goroutine. Export never touches the render
hot path. If the endpoint is unreachable, the batch is dropped and the next
interval starts fresh. No unbounded buffering. Dashboard never degrades.

### Build tag isolation

### Repo split: open core + private enterprise

The OTEL implementation is **proprietary IP** in a separate private repo.
The open repo contains the interface seam and a nil stub — zero enterprise
code to lift.

**Open repo (coolant):**
- `internal/model/sink.go` — `MetricSink` interface (the public seam)
- `cmd/thermal/otel_stub.go` — nil stub, always compiled in free builds
- All dashboard, theme, animation, collector, gate code — MIT licensed

**Private repo (thermal-enterprise):**
- `internal/otel/` — full OTEL implementation (`MeterProvider`,
  `PeriodicReader`, HTTP exporter, metric instrument registration,
  closed attribute enforcement, transport security, config parsing)
- Compliance documentation package (data flow diagrams, PIA template,
  SIG Lite answers)
- Pre-built Grafana dashboard JSONs
- Alerting rule templates

Enterprise builds vendor the private repo as a Go module dependency.
The build tag `//go:build enterprise` controls which init path compiles.

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

The thin shim in the open repo imports the private module but contains no
implementation. `go build ./cmd/thermal/` succeeds without access to the
private repo (the `!enterprise` stub wins). `go build -tags enterprise`
requires module access (authenticated via `GOPRIVATE` + Git credentials).

**Binary builds:**
- Free: `go build ./cmd/thermal/` (~15MB, no protobuf deps, no private module)
- Enterprise: `go build -tags enterprise ./cmd/thermal/` (~18-20MB, requires private repo access)

**Distribution:** Enterprise binaries are pre-built and distributed via
private GitHub Releases or direct delivery. Customers never need source
access to the private repo unless they're building from source (uncommon).

### Go OTEL SDK packages

Metrics only. No traces, no logs.

- `go.opentelemetry.io/otel/sdk/metric` — MeterProvider, PeriodicReader
- `go.opentelemetry.io/otel/metric` — instrument types
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` — default
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` — optional, behind `otel_grpc` build tag

### Performance budget

| Concern | Impact | Detail |
|---------|--------|--------|
| `Record()` calls | ~2-3us/tick | 15 instruments, non-blocking writes to aggregation buckets |
| Memory | ~20KB steady-state | ~100 aggregation cells at ~200 bytes each |
| Export I/O | Off hot path | PeriodicReader goroutine, 10s interval, async |
| Endpoint down | Zero dashboard impact | Batch dropped, next interval fresh, no buffering |
| Shutdown | 5s deadline | `MeterProvider.Shutdown()` flushes final batch on clean exit |

---

## 3. Metric Catalog

### System metrics

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.cpu.utilization` | Float64Gauge | percent | — |
| `thermal.memory.used` | Int64Gauge | bytes | — |
| `thermal.memory.total` | Int64Gauge | bytes | — |
| `thermal.memory.utilization` | Float64Gauge | percent | — |
| `thermal.memory.headroom` | Int64Gauge | bytes | — |
| `thermal.memory.decompressions` | Int64Gauge | count | — |
| `thermal.swap.used` | Int64Gauge | bytes | — |
| `thermal.swap.total` | Int64Gauge | bytes | — |
| `thermal.gpu.utilization` | Float64Gauge | percent | — |

### Agent lifecycle

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.agents.active` | Int64Gauge | count | `state={fresh,stale}` |
| `thermal.agents.completed` | Int64Counter | count | — |
| `thermal.agents.spawn_rate` | Float64Gauge | per_second | — |
| `thermal.agents.death_rate` | Float64Gauge | per_second | — |
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

### Gate events (monotonic counters)

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.gate.suppressions` | Int64Counter | count | `tool={tsc,eslint,vitest,cargo-build,...}` |
| `thermal.gate.caps` | Int64Counter | count | `tool={vitest,jest,go-test,...}` |

The `tool` attribute is a **closed set** derived from gate.sh's known tool
list (not the raw command string — see Security section). Unknown tools
bucket to `other`.

### Network

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.api.online` | Int64Gauge | boolean | — |

### Cost signal (ships with v1)

Thermal cannot observe token counts today — the JSONL event bus and bash
hooks have no access to API response bodies. But cost *signal* is
derivable from data we already own. Full analysis in
[cost-attribution.md](cost-attribution.md).

| Metric | Type | Unit | Attributes | Source |
|--------|------|------|------------|--------|
| `thermal.agents.duration_seconds` | Float64Counter | seconds | `team`, `project` | agent start/stop timestamps |
| `thermal.gate.time_saved_seconds` | Float64Counter | seconds | `tool` | suppression count × configured avg duration |

**Agent-hours** = the primary cost proxy. Rankable across teams without
knowing token prices. "Team A burned 340 agent-hours, Team B burned 40."

**Gate suppression ROI** = the headline enterprise number. "Thermal
prevented 847 redundant tsc invocations this week, saving approximately
12 hours of CPU time." Concrete, defensible, unique to Thermal.

Two customer profiles (Enterprise orgs vs. stacked Max/Pro plans) may
have fundamentally different access to token data. For Max/Pro customers,
these proxy metrics may be the *only* fleet-level cost signal available —
permanently, not temporarily. See [cost-attribution.md](cost-attribution.md).

### Cost attribution (blocked on external data source)

Absolute token counts and dollar amounts. Requires a data source Thermal
doesn't have yet. Active research across Claude API, Bedrock, Vertex,
Foundry, and Claude Code local telemetry.

**Reserved metric names** (forward-compatible — dashboards reference these
now, they light up when the data source exists):

| Metric | Type | Unit | Attributes |
|--------|------|------|------------|
| `thermal.tokens.input` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.output` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.cache_read` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.tokens.cache_creation` | Int64Counter | tokens | `agent_id`, `session_id` |
| `thermal.cost.usd` | Float64Counter | dollars | `agent_id`, `session_id` |

---

## 4. Configuration

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
```

### Env var precedence

Standard OTEL env vars take priority over config file:

```
OTEL_EXPORTER_OTLP_ENDPOINT  >  config.toml [otel] endpoint  >  disabled
OTEL_EXPORTER_OTLP_HEADERS   >  (no config equivalent — secrets never in files)
OTEL_EXPORTER_OTLP_CERTIFICATE > config.toml [otel] ca
```

This means a platform team can set the endpoint via environment (MDM,
launchd plist, systemd unit) without touching config files. Developers who
already have OTEL env vars in their shell get Thermal export for free.

### Kill switch

`COOLANT_OTEL=0` immediately disables all export regardless of config.
Allows incident response to halt data flow fleet-wide via MDM push.

### Auth tokens

**Never in config files.** Auth credentials sourced exclusively from:
- `OTEL_EXPORTER_OTLP_HEADERS` env var (standard OTEL)
- `auth_header_env = "MY_TOKEN_VAR"` in config (reads named env var at runtime)

If a literal auth value is detected in the config file, log a warning at
startup.

### Protocol default: HTTP/protobuf

HTTP/protobuf over gRPC because:
- Works through corporate proxies and load balancers that block HTTP/2
- Every OTEL collector accepts `http/protobuf` on `/v1/metrics`
- No `google.golang.org/grpc` dependency (~8MB binary savings)
- Simpler TLS config (standard HTTPS)

gRPC available behind `otel_grpc` build tag for stacks that prefer it.
HTTP/JSON not supported (3-5x payload overhead, no production use case).

---

## 5. Security

### Transport security

**Resolved tension:** Security demanded TLS-only; Product wanted easy
localhost dev. Resolution: **localhost exception.**

- Remote endpoints: TLS required. `http://` scheme refused with clear error.
- Localhost (`127.0.0.1`, `::1`, `localhost`): plaintext allowed **automatically**.
  No config field needed — the endpoint hostname is sufficient. This
  covers `docker run otel/opentelemetry-collector` dev workflows without
  requiring the developer to also set `tls = false` or `insecure = true`.
- mTLS supported via `cert`/`key`/`ca` config fields.
- Certificate validation on by default. Custom CA via `ca` field for
  TLS-intercepting enterprise proxies.
- Private key files must be `0600`. Thermal refuses to start if
  group/world-readable, logging the exact permission bits.

### Closed attribute set

OTEL SDK auto-resource-detection **disabled**. Only explicitly listed
attributes are emitted:

**Default (always present):**
- `service.name` = `thermal`
- `service.version` = build version
- `os.type` = runtime.GOOS
- `host.arch` = runtime.GOARCH

**Opt-in (via config `[otel.labels]`):**
- `host.name` — opted in by setting `hostname = true` or explicit value
- `user` — opted in by setting `username = true` or explicit value
- `team`, `environment`, `cost_center` — from config labels

**Never emitted under any configuration:**
- `GateEvent.Command`, `Original`, `Rewritten` — Restricted data. Literal
  shell commands routinely contain secrets, file paths, connection strings.
  Architecturally excluded from OTEL serialization. Enforced by unit test.
- Process `Comm` raw values — only `basename()` derivative or category
  mapping used as attributes.
- `CollectErrs` strings — may embed file paths and system details.

### Attribute cardinality

- `tool` attribute on gate counters uses a **closed set** from gate.sh's
  known tool list. Unknown tools bucket to `other`.
- `category` attribute is already bounded (7 fixed categories).
- `threat.name` is bounded (4 levels).
- Max 20 custom labels, 256 char per value, 4KB combined.

### Config file security

- `config.toml` should be `0600`. OTEL config refused from files with
  group/world read.
- Endpoint allowlisting via `COOLANT_OTEL_ALLOWED_ENDPOINTS` env var (set
  by MDM, not the config file). Thermal refuses to export to unlisted
  endpoints.
- Optional `config_hash` validation for MDM-pushed configs.

### Compliance documentation (ships with enterprise binary)

1. Data flow diagram (collector sources → serialization → TLS → destination)
2. Attribute inventory with data classifications (from security review)
3. Privacy Impact Assessment template (GDPR, pre-filled for Thermal)
4. Security questionnaire answers (SIG Lite / CAIQ format)
5. Incident response runbook for credential compromise

### Threat model — top 3 risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Command string exfiltration via OTEL attributes | Critical | Architecturally excluded. Unit test enforces zero Restricted fields in export path. |
| Config file redirect to attacker endpoint | High | Endpoint allowlisting via MDM env, file permission enforcement, startup logging of active endpoint. |
| OTEL dependency supply chain attack | High | Pinned versions in go.sum, checksum verification, govulncheck in CI, consider vendoring. |

---

## 6. UX

### CLI

**No new flags for export config.** Flag namespace stays clean and visual
(`--theme`, `--animation`, `--demo`). OTEL is operational plumbing — it
lives in config/env.

**One diagnostic flag:** `--otel-status` (print-and-exit)

```
$ thermo --otel-status
otel: exporting to otel-collector.corp.example.com:4318 (http, tls)
      labels: team=platform, environment=staging
      last export: 3s ago (15 metrics, 0 errors)
      status: healthy
```

```
$ thermo --otel-status
otel: disabled (no endpoint configured)
      set [otel] endpoint in ~/.config/coolant/config.toml
      or export OTEL_EXPORTER_OTLP_ENDPOINT
```

### Dashboard indicator

Single dimmed `↑` glyph in the rates line when OTEL is active:

```
...  ⊙ Chrome  ↑ [h] help
```

- **Healthy:** dimmed white (same weight as `[h] help`)
- **Failing:** dimmed amber (theme warn color, never red)
- **Sustained failure (>60s):** amber with stale-breath pulse (reuses
  existing ghost-dot animation)
- **Recovery:** returns to steady dim white, no fanfare
- **OTEL disabled:** nothing rendered, zero visual cost

Alert log entries on failure/recovery:
```
otel: export failed (connection refused)
otel: export recovered
```

These scroll away naturally. Same visual weight as spawn-burst alerts.

### First-time enablement (3 steps, <2 minutes)

1. Start a collector: `docker run -p 4318:4318 otel/opentelemetry-collector`
2. Add to config: `echo '[otel]\nendpoint = "localhost:4318"' >> ~/.config/coolant/config.toml`
3. Restart thermo. See `↑` glyph. Open Grafana. Import dashboard JSON. Done.

Hot-reload deferred to v2. Ship restart-required first.

---

## 7. Grafana Dashboards

Ship as provisioned JSON in `thermal/grafana/`. Templated datasource
variables — customer points at their Prometheus instance, it works.

### Design philosophy

The Grafana dashboards are Thermal's web face. Same product, different
viewport.

- **Dark theme only.** Matches terminal dashboard.
- **Classic severity colors.** Exact hex values from `theme/classic.go` in
  Grafana threshold config. Amber on screen = amber in terminal.
- **Sparkline-heavy, not number-heavy.** Invert Grafana's default (big
  number, tiny sparkline). Use time-series panels with sparkline fill,
  minimal axis labels, severity-colored thresholds.
- **Layout mirrors terminal hierarchy.** Top: threat overview. Middle:
  system gauges. Bottom: process categories and agent activity.
- **No gratuitous panels.** Every panel answers a question the terminal
  strip already answers. Grafana adds history (24h vs 90s), not new data.

### Dashboard 1: Fleet Overview (platform team)

- **Row 1:** Stat panels — total active agents fleet-wide, machines in
  MELTDOWN (red), machines in HOT (amber), gate suppressions today
- **Row 2:** Heat map — machines as cells, colored by threat level, sorted
  by severity. Click-through to Machine Deep Dive.
- **Row 3:** Time series — fleet-wide agent count (24h), fleet CPU p50/p95,
  fleet memory p50/p95
- **Row 4:** Table — top 10 machines by agent count, labeled by team/project

### Dashboard 2: Cost & Efficiency (engineering managers)

- **Row 1:** Stat panels — agent-hours this week, estimated API cost
  (agent-hours × configurable rate), gate suppressions (count + time saved)
- **Row 2:** Bar chart — agent-hours by team (stacked by project),
  week-over-week
- **Row 3:** Time series — gate suppressions by tool type (tsc, eslint,
  vitest, cargo), showing which tools get suppressed most
- **Row 4:** Table — per-developer: agent count, agent-hours, suppressions,
  threat-level distribution (% time in each state)

### Dashboard 3: Machine Deep Dive (drill-down)

- **Row 1:** Current state — threat gauge, active agents, CPU/MEM/SWAP
  (mirrors terminal strip)
- **Row 2:** Time series — CPU, memory, swap, decompressions (24h) with
  threat-level bands as colored regions
- **Row 3:** Event log — scrolling table of gate events with timestamps
- **Row 4:** Agent timeline — Gantt-style agent lifecycles, colored by
  threat level during runtime

### README

Setup instructions for: Prometheus + Grafana, Grafana Cloud, Datadog with
OTLP intake. One-page each.

---

## 8. The Demo

### Beat 1: Peripheral vision (0:00–5:00)
Todd advises on Claude adoption. Thermal runs in tmux. He doesn't mention
it. The platform engineers notice the braille strip glowing.

### Beat 2: The question (5:00–5:30)
"What's that at the bottom?" — "Oh, Thermal. Watches what Claude's doing so
I don't have to." Brief pause. Back to the main topic.

### Beat 3: The casual demo (8:00–10:00)
Spawn three parallel agents. Dashboard lights up — dots appear, CPU
sparkline climbs, threat shifts to WARM. Point at gate suppression count.
"It just killed four redundant tsc invocations."

### Beat 4: The pivot (10:00–12:00)
"Imagine this across 200 developers. You have no idea which teams burn API
credits on cold cache, which projects spawn runaway agents, or whose
machines are melting." Pause. "Thermal has an enterprise mode. One config
line — it exports everything here to your existing Grafana."

### Beat 5: The dashboard (12:00–14:00)
Pull up Fleet Overview in Grafana. Three panels: fleet heat map, cost
attribution, gate ROI. "This is what it looks like when your platform team
can see what 200 Claude agents are doing to your infrastructure."

### Beat 6: The close (14:00–15:00)
"The dashboard is free. Open source. Your devs can install it today. The
fleet export is what you'd license." Hand them the repo URL.

---

## 9. Resolved Design Tensions

| Tension | Positions | Resolution |
|---------|-----------|------------|
| **Transport default** | Security: TLS-only, no exceptions. Product: insecure=true for dev ease. | Localhost exception: plaintext allowed for 127.0.0.1/::1/localhost. Remote requires TLS. |
| **Protocol** | Platform: HTTP/protobuf (proxy compat, smaller binary). Product: gRPC (standard OTEL). | HTTP/protobuf default. gRPC behind build tag. Corporate proxies win. |
| **Attribute emission** | Sales: emit hostname, user, team, project by default. Security: closed set, PII opt-in only. | Security wins. Default: service.name + version + os + arch. Hostname/username require explicit opt-in. |
| **Config vs flags** | Product: config-only. Platform: honor standard OTEL env vars. | Both. OTEL env vars > config file > disabled. No CLI flags for export config. |
| **Secrets in config** | Product: simple auth_header field. Security: never in files. | Secrets via env vars only. `auth_header_env` references a named env var. Literal tokens in config trigger a warning. |
| **Command strings in metrics** | Sales: command attribution is valuable. Security: commands contain secrets (Restricted). | Security wins absolutely. Command strings architecturally excluded. Gate tool attribution uses a closed set of tool names, not raw commands. |

---

## 10. Implementation Sequence

### Phase 1: Plumbing (no enterprise value yet, but shippable)
1. Define `MetricSink` interface in `internal/model/`
2. Wire nil-check call sites in `AppState.Update()` and `HandleEvent()`
3. Implement `internal/otel/` with MeterProvider + PeriodicReader + HTTP exporter
4. System metrics + agent lifecycle + threat level + gate counters
5. Build tag isolation (`enterprise` tag)
6. Config parsing for `[otel]` block
7. `--otel-status` diagnostic flag

### Phase 2: Security hardening
8. Closed attribute set with unit test enforcement
9. Transport security (TLS enforcement, localhost exception, mTLS)
10. Config file permission checks
11. Endpoint allowlisting via env
12. Kill switch (`COOLANT_OTEL=0`)

### Phase 3: Enterprise polish
13. `↑` glyph dashboard indicator with failure states
14. Grafana dashboard JSONs (Fleet Overview, Cost & Efficiency, Deep Dive)
15. Alerting rule templates
16. Compliance documentation package
17. README with setup instructions for common stacks

### Phase 4: Cost attribution (blocked on data source)
18. Investigate Claude Code hook/telemetry for token counts
19. Implement reserved token/cost metrics
20. Update Grafana dashboards with cost panels

---

## Appendix: Persona Findings

Individual review documents with full rationale:

- [Platform Engineer](platform-engineer.md) — SDK, integration point, perf budget, metric catalog
- [Sales Engineer](sales-engineer.md) — Enterprise needs, adoption blockers, pricing, demo flow
- [Product Designer](product-designer.md) — Config UX, CLI, dashboard indicator, Grafana philosophy
- [Security & Compliance](security-compliance.md) — Data classification, threat model, compliance gates
