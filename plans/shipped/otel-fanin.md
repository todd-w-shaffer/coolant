# OTEL fan-in

## Goal
Fan in Claude Code's OTEL metric stream alongside the transcript-tail token collector so the dashboard's `tok N/s · cache N%` reflects total Claude API traffic (background calls, compaction, title generation), not just per-message `assistant` rows. Single fragment, two sources, no double-counting.

## Non-goals
- Building a new widget — same single line on the rates strip.
- Reworking the OTEL adapter itself (`internal/otel/cc/`) — landed on main as `9d9b6d8`, treat as a given (we extend its `AggregatorView` interface; we do not rewire its internals).
- Per-session attribution. Still aggregate. (That's the OTHER deferred follow-up; tackle separately.)
- Per-agent / per-skill / per-plugin breakdown — receiver allowlist (`receiver.go:36-48`) drops `agent.name` / `skill.name` / `plugin.name` / `marketplace.name` today, so the data isn't available in-process. Separate plan if we want this.
- Cost math, model-pricing tables.
- Touching the `f2e2987` per-agent-stop telemetry path. That's a different axis (post-hoc attribution to `stats.Counters`); orthogonal to live throughput.
- Exposing or rendering the OTEL `cc-findings.jsonl` data — that's `thermo cc-findings`' job. We only consume the in-memory aggregator state.
- Making the OTEL receiver mandatory. If port 4318 fails to bind (already a documented degrade path), the dashboard falls back to transcript-tail-only and behaves exactly as it does today.
- Annotation glyph on the rates strip ("OTEL coverage vs transcript-only"). Defer; tackle separately if useful.

## Files to touch
- `thermal/internal/otel/cc/tailer.go` — add an accessor returning the **latest-cumulative** value per `(metric, attrs, dayKey)`, NOT the existing `SumDay()` which is sum-of-cumulatives. Coolant overrides CC to `cumulative` temporality (`scripts/enable-cc-otel.sh:77`), so the tailer's `Sum +=` (`tailer.go:189`) is sum-of-cumulatives and unsuitable for live throughput. Likely shape: `LatestCumulative(metric, attrs, dayKey) (value float64, ts time.Time, ok bool)`.
- `thermal/internal/otel/cc/adapter.go` — extend the `AggregatorView` interface (`adapter.go:215-219`) with **three** new methods:
  1. `OTELTokens(now time.Time) (input, output, cacheCreate, cacheRead int64, ok bool)` — sums latest-cumulative across all `query_source ∈ {main, subagent, auxiliary}` for each `type` value (note: OTEL emits CamelCase `cacheRead`/`cacheCreation`; transcript uses snake_case — fan-in normalises to a single int64 quadruple).
  2. `IsOTELLive(now time.Time) bool` — reuses the existing `cleanlyOfflineWindow=2min` debounce (`reconcile.go:24,211`). Adapter exposes the boolean; reconciler logic stays where it is.
  3. `ObserveTokenSchemaDrift(field, ccVersion string)` — thin pass-through to the adapter's existing `ObserveSchemaDrift` (`adapter.go:145-172`) so TokenCollector can fire the degraded-counter path on lookup miss without depending on the concrete adapter type.
- `thermal/internal/collector/tokens.go` — accept the extended `AggregatorView` via a variadic-option constructor (nil-safe; absent OTEL behaves identically to today). Compute per-tick deltas by subtracting last-seen cumulative. **Either-or merge**: when `IsOTELLive` is true, suppress transcript contribution entirely; when false, transcript is authoritative.
- `thermal/internal/collector/collector.go` and `thermal/cmd/thermal/main.go:359-408` — wire the existing adapter into the TokenCollector at the call site (the wire-up is in `main.go`, not `collector.go`; both files participate in the constructor change).
- `thermal/internal/collector/tokens_test.go` — new tests for: (a) OTEL-only path; (b) transcript-only path; (c) both-active reconciliation producing OTEL numbers, NOT sum; (d) live → offline (>2min stale) transition reverting to transcript; (e) schema-drift on missing attribute name fires `ObserveTokenSchemaDrift` and falls back to transcript for that tick.

## Failure modes to anticipate
- **Double-counting (the load-bearing one).** OTEL covers `query_source ∈ {main, subagent, auxiliary}`; transcript covers only `main + subagent`. During a normal turn BOTH sources report the same tokens — naive sum = exactly **2× inflation** on every foreground token. The cache-hit ratio doubles num and denom and looks roughly right, masking the bug. Merge is **either-or per-tick**, not gap-fill: OTEL authoritative when live, transcript-only when OTEL is stale.
- **Cumulative-vs-delta semantics.** Coolant overrides CC to `cumulative` temporality (`scripts/enable-cc-otel.sh:77`). The tailer's `Sum +=` aggregates every data point ever written; reading `SumDay()` yields **sum-of-cumulatives**, NOT the latest cumulative. Fan-in MUST read latest-data-point value per attrs and subtract its own prior-tick snapshot. Calling `SumDay()` here would produce exponentially growing numbers that look plausible at first but climb without bound.
- **First-10s buffered window.** OTEL meter export buffers 10s (`OTEL_METRIC_EXPORT_INTERVAL=10000` in `enable-cc-otel.sh:78`); transcript flushes synchronously at message-end. If we key "OTEL present this tick" we'd undercount the first ~10s of each session. Fix: reuse `cleanlyOfflineWindow=2min` debounce (`reconcile.go:24,211`) — once OTEL has been seen recently, trust it through brief silences.
- **Schema-drift silent zero.** Tailer key lookup returns 0 silently on missing key (`tailer.go:237-244`). A rename like `cacheRead` → `cache_read` would split one counter into two buckets — silent undercount on the original, silent new series. TokenCollector MUST call `ObserveTokenSchemaDrift("type"|"query_source", ccVersion)` on lookup miss → bumps `$COOLANT_DEGRADED_COUNT` visibly rather than emitting wrong numbers.
- **Attribute case mismatch.** OTEL emits `type=cacheRead`/`type=cacheCreation` (CamelCase per Anthropic docs); transcript JSONL emits `cache_read_input_tokens`/`cache_creation_input_tokens` (snake_case, `tokens.go:71-75`). Fan-in normalizes to a single int64 quadruple; tests must cover the canonicalisation.
- **`subagentInputTokens` filter is a red herring.** `reconcile.go:369-376` filters to `query_source=subagent` for cross-axis Counters comparison — that's a different job. Fan-in must sum ALL query_sources or it will silently exclude `auxiliary` (the whole motivating case).
- **OTEL not running.** Port-bind failure is a documented degrade in `internal/otel/cc/`; receiver writes one `receiver_bind_failed` finding and reconcile no-ops. TokenCollector must handle nil-aggregator-view gracefully (variadic-option pattern) and degrade to transcript-only without any user-visible disruption.
- **Window alignment.** Transcript-tail window is 30 ticks of 1s deltas. OTEL exports on its own cadence (~10s); reconciler tick may see different update frequencies. Mitigated by reading latest cumulative (not per-window join) — straddled exports just land in the next tick.
- **PII leakage.** Receiver already scrubs `user.email` at the boundary (`receiver.go:28`). The new `OTELTokens()` accessor must aggregate **before** returning (sum across `session.id` / `user.account_uuid` / `organization.id`); never expose those attribute values to the dashboard layer. Returning four int64s by `type` only — no session keys — is the trust-boundary discipline.
- **Slow-loop latency.** Slow loop runs 3 concurrent goroutines at 1s. `AggregatorView` reads are short-locked (verified: `Snapshot()` deep-copies under RLock; `SnapshotAggregate()` same pattern). New `OTELTokens()` must follow the same short-lock pattern — no blocking on the receiver's write path.
- **Test isolation.** OTEL package brings up an HTTP receiver in `receiver_test.go`. Collector-level fan-in tests MUST mock the extended `AggregatorView` interface to avoid booting the receiver.

## Done criteria
- `thermo` running against a live Claude Code session reflects token traffic that **exceeds** what transcript-tail-alone would report during `query_source=auxiliary` activity (compaction, title-gen, background haiku calls). Verify with a manual side-by-side: run transcript-only build vs OTEL-fanin build, observe the latter's `tok N/s` going higher during a compaction trigger.
- During a normal foreground turn (one `assistant` message), the displayed `tok N/s` matches today's transcript-only number — not 2× — proving the either-or merge holds.
- When OTEL receiver fails to bind (port 4318 in use), the dashboard renders identical numbers to today's transcript-tail-only behavior. No crash, no missing fragment.
- When OTEL has been silent for >2min, fan-in reverts to transcript-only without a visible discontinuity on the rates strip.
- `go test ./...` green; new tests cover: (a) OTEL-only path; (b) transcript-only path; (c) both-active reconciliation producing OTEL numbers, NOT sum; (d) live → offline (>2min stale) transition reverting to transcript; (e) schema-drift on missing attribute name fires `ObserveTokenSchemaDrift` and falls back for that tick.
- No PR introduces a new public surface that could regress the OTEL receiver's PII guarantees (no `user.email`, no raw attribute pass-through to the dashboard layer). `OTELTokens()` returns four int64s by `type` only — no session/account/org keys.
- Schema gate uses the existing `knownCCMetrics` + `IsKnownCCAttr` checks (`adapter.go:15-24,44-51`); lookup miss on `type` or `query_source` calls `ObserveTokenSchemaDrift` so the degraded counter bumps visibly rather than silently producing wrong numbers.

## Parking lot
- Per-agent / per-skill / per-plugin token breakdown — blocked on `receiver.go:36-48` allowlist dropping `agent.name`/`skill.name`/`plugin.name`/`marketplace.name`. Separate plan if we want this.
- Rates-strip annotation glyph distinguishing "OTEL coverage active" from "transcript-only" — defer.
- Per-session attribution path (the OTHER deferred follow-up from the token-counter ship).
