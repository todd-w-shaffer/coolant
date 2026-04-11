# OTEL Export Path — Platform Engineer Review

## 1. Go OTEL SDK Assessment

We want **metrics only** — no traces, no logs. The minimum package set:

- `go.opentelemetry.io/otel/sdk/metric` — `MeterProvider`, `PeriodicReader`
- `go.opentelemetry.io/otel/metric` — instrument types (`Float64Gauge`, `Int64Counter`, etc.)
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` — HTTP/protobuf exporter (recommended default)
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` — gRPC exporter (optional, behind build tag)

Skip the tracer provider entirely. Skip `go.opentelemetry.io/otel/log`. The meter provider with a `PeriodicReader` is the whole integration surface.

Use `PeriodicReader` (not manual flush). Set the export interval to 10s — an order of magnitude slower than our fast loop (150ms), well within OTLP collector norms (Datadog agent default is 10s, Prometheus scrape default is 15s). This reader accumulates delta temporality metrics and flushes asynchronously on its own goroutine. It never touches the fast loop.

## 2. Integration Point

The cleanest hook is in `model.AppState.Update()` — the single function that processes every Snapshot and recomputes all derived state. After `s.updateThreatAndAlerts(&snap)` returns, all computed fields (`ThreatLevel`, `SpawnRate`, `DeathRate`, `CategoryCounts`, `Headroom`, etc.) are populated. This is where we record OTEL observations.

Do NOT hook into the collector loops directly. The collector produces raw `Snapshot` structs on a channel; the model is where semantic meaning gets attached. An OTEL exporter cares about threat levels, category rollups, and headroom — all model-layer concepts.

Implementation shape: add an optional `MetricSink` interface to the model package:

```go
type MetricSink interface {
    Record(s *AppState, snap *collector.Snapshot)
}
```

`AppState` gets a `MetricSink` field (nil in free tier). At the end of `Update()`, if non-nil, call `sink.Record(s, &snap)`. The sink implementation lives in a separate `internal/otel/` package. `Record` calls `gauge.Record()` / `counter.Add()` — these are non-blocking in the OTEL SDK; they write to in-memory aggregation state and return immediately.

For gate events: `HandleEvent()` already switches on event type. Add `sink.RecordEvent(ev)` at the end. Gate suppress and cap events become counter increments.

## 3. Performance Budget

The Go OTEL SDK is designed exactly for this use case. Key guarantees:

**Recording is non-blocking.** `gauge.Record()` and `counter.Add()` write to lock-free (or fine-grained-locked) aggregation buckets. Measured overhead is ~100-200ns per instrument per call. With ~15 instruments recorded per tick, that is roughly 2-3 microseconds — invisible against a 150ms budget.

**Export is fully async.** `PeriodicReader` runs its own goroutine with its own timer. When it fires (every 10s), it snapshots the aggregation state, serializes to protobuf, and sends over the network. This happens entirely off the hot path. If serialization + network takes 500ms, the fast loop never knows.

**Unreachable endpoint.** The OTLP exporter has a configurable timeout (default 10s) and retry with exponential backoff. When the endpoint is down, the periodic reader's export call fails, metrics for that interval are dropped, and the next interval starts fresh. There is no unbounded buffering — failed exports do not accumulate memory. The meter provider's internal aggregation is bounded (fixed number of instruments x attribute sets).

**Memory overhead.** The meter provider allocates per-instrument-per-attribute-set aggregation state. With ~15 instruments and 1-2 attribute dimensions (threat level, category name), expect roughly 50-100 aggregation cells. Each cell is ~200 bytes. Total steady-state overhead: ~20KB. Negligible.

**Worst case.** If `Record()` ever blocks (it shouldn't, but hypothetically due to a buggy exporter), it would delay `AppState.Update()` by that duration. Mitigation: wrap the sink call in a select with a 1ms deadline, but this is likely unnecessary given the SDK's design.

## 4. Metric Catalog

### System metrics

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.cpu.utilization` | Float64Gauge | `percent` | — |
| `thermal.memory.used` | Int64Gauge | `bytes` | — |
| `thermal.memory.total` | Int64Gauge | `bytes` | — |
| `thermal.memory.utilization` | Float64Gauge | `percent` | — |
| `thermal.swap.used` | Int64Gauge | `bytes` | — |
| `thermal.swap.total` | Int64Gauge | `bytes` | — |
| `thermal.gpu.utilization` | Float64Gauge | `percent` | — |
| `thermal.memory.decompressions` | Int64Gauge | `count` | — |
| `thermal.memory.headroom` | Int64Gauge | `bytes` | — |

### Agent lifecycle

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.agents.active` | Int64Gauge | `count` | `state={fresh,stale}` |
| `thermal.agents.completed` | Int64Counter | `count` | — |
| `thermal.agents.spawn_rate` | Float64Gauge | `per_second` | — |
| `thermal.agents.death_rate` | Float64Gauge | `per_second` | — |
| `thermal.sessions.active` | Int64Gauge | `count` | — |

### Threat level

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.threat.level` | Int64Gauge | `level` | `name={COOL,WARM,HOT,MELTDOWN}` |

Encoding threat as both a numeric gauge (0-3) and a name attribute lets Grafana dashboards do numeric thresholds and label-based annotations.

### Process categories

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.processes.count` | Int64Gauge | `count` | `category={build,shell,node,go,python,rust,swift}` |
| `thermal.processes.total` | Int64Gauge | `count` | — |

### Gate events (counters — monotonically increasing)

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.gate.suppressions` | Int64Counter | `count` | `command` |
| `thermal.gate.caps` | Int64Counter | `count` | `command` |

### Network

| Metric name | Type | Unit | Attributes |
|---|---|---|---|
| `thermal.api.online` | Int64Gauge | `boolean` | — |
| `thermal.api.offline_duration` | Float64Gauge | `seconds` | — |

### Cost attribution (GAP)

Token-level cost metrics (`thermal.tokens.input`, `thermal.tokens.output`, `thermal.tokens.cache_read`, `thermal.cost.usd`) would be the highest-value enterprise metrics — the reason a platform team deploys this. **However, the JSONL event bus does not currently carry token counts.** The `GateEvent` struct has no token fields. The bash hooks that write events (`agent-start.sh`, `agent-stop.sh`, `gate.sh`) don't have access to API response bodies where token usage lives.

Closing this gap requires either: (a) a new hook type that fires post-API-response (Claude Code doesn't currently expose one), or (b) parsing Claude Code's own session logs. This is a prerequisite for enterprise value and should be scoped as a separate workstream — the OTEL plumbing can ship without it and the metrics get added when the data source exists.

## 5. Protocol and Transport

**Default: HTTP/protobuf (`otlpmetrichttp`).** Reasons:

- Works through corporate proxies and load balancers that often block HTTP/2 / gRPC.
- Every OTEL collector (the reference collector, Datadog Agent, Grafana Agent, Vector) accepts `http/protobuf` on `/v1/metrics`.
- No dependency on `google.golang.org/grpc` (~8MB binary overhead).
- Simpler TLS configuration (standard HTTPS, no mTLS channel credentials).

**Optional: gRPC (`otlpmetricgrpc`) behind a `//go:build otel_grpc` tag.** Some enterprise stacks (New Relic, hosted Grafana Cloud) prefer gRPC for streaming efficiency. Ship it as a secondary binary or let customers build from source with the tag.

**Do not support HTTP/JSON.** It is the slowest OTLP encoding (3-5x larger payloads than protobuf) and no production collector prefers it. If a customer needs JSON, they should run the OTEL collector as a local sidecar to transcode.

Configuration surface: `OTEL_EXPORTER_OTLP_ENDPOINT` (the standard env var, which the Go SDK reads automatically), plus `OTEL_EXPORTER_OTLP_HEADERS` for auth tokens. No custom config needed — follow the OTEL env var spec and customers' existing tooling Just Works.

## 6. Dependency Impact

New direct dependencies for HTTP/protobuf only:

- `go.opentelemetry.io/otel` (core) — ~2MB
- `go.opentelemetry.io/otel/sdk/metric` — ~1.5MB
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` — ~1MB
- Transitive: `google.golang.org/protobuf`, `go.opentelemetry.io/proto/otlp`

Estimated binary size increase: **3-5MB** (current thermo binary is likely ~15MB with bubbletea + cgo). Not trivial.

**Build tag isolation: yes, mandatory.** The OTEL code should live behind `//go:build enterprise`. The free-tier binary (`go build ./cmd/thermal/`) carries zero OTEL imports. The enterprise binary (`go build -tags enterprise ./cmd/thermal/`) pulls in the meter provider. The `MetricSink` interface in the model package is always present (it is just an interface), but the implementation that satisfies it only compiles with the tag. This keeps the free binary lean and avoids shipping protobuf/OTLP dependencies to individual users.

In `main.go`, the wiring looks like:

```go
//go:build enterprise

func initOTEL() model.MetricSink { ... }
```

With a no-op stub in the default build:

```go
//go:build !enterprise

func initOTEL() model.MetricSink { return nil }
```

## 7. Failure Modes

| Scenario | SDK behavior | Dashboard impact |
|---|---|---|
| **Endpoint down** | `PeriodicReader` export fails, logs warning, retries next interval. No buffering. | None. `Record()` calls succeed into local aggregation; only export is lost. |
| **Endpoint slow** (>10s response) | Export times out (configurable). That batch is dropped. Next interval proceeds independently. | None. Export runs on its own goroutine. |
| **Wrong credentials** | HTTP 401/403. Exporter logs error, drops batch, retries next interval. | None. Same as endpoint down from the SDK's perspective. |
| **Network partition mid-batch** | TCP timeout or reset. Exporter treats as transient failure. | None. Partial writes are idempotent (OTLP is designed for this). |
| **DNS resolution failure** | Exporter cannot connect. Same as endpoint down. | None. |
| **Memory pressure from high-cardinality attributes** | If `command` attribute on gate counters has unbounded cardinality, aggregation cells grow. | Mitigate by capping `command` to a known set (top-20 commands, bucket rest as "other"). The category and threat attributes are already bounded. |

The critical invariant: **the `PeriodicReader` never blocks the goroutine that calls `Record()`**. Recording writes to in-memory aggregation. Exporting reads from that aggregation on a separate goroutine. These are decoupled by design in the OTEL SDK. The dashboard cannot degrade from OTEL failures — the worst case is silent metric loss, which is the correct tradeoff for an observability sidecar.

One additional safeguard: wrap `MeterProvider.Shutdown()` in the existing `close(m.done)` path so the final batch flushes on clean exit. Use a 5-second context deadline on shutdown to avoid hanging if the endpoint is unreachable at quit time.
