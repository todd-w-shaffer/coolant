# PR 2 — OTEL token fan-in (stacked on PR 1) + two HIGH bug fixes

## Goal
Re-apply the 6 dropped otel-fanin commits onto PR 1 (branch `pr1-no-otel`), then fix the two HIGH correctness bugs the maintainer found and close the test-harness gap that hid them. Land a live cross-midnight + source-flip verification. Stacked PR — hard-depends on PR 1's `TokenCollector`.

## Diagnosis  *(both bugs verified statically via `git show` of the dropped commits)*

**Bug 1 — UTC midnight burns the one-shot drift gate.**
- **Hypothesis:** `adapter.OTELTokens` keys cumulative reads on `now.UTC()` date; at 00:00 UTC the bucket rolls empty → `ok=false` → collector's `otelEverProduced && !driftFired` branch fires `token_lookup_miss` and burns the one-shot `driftFired`, so a real later attribute rename never fires for the rest of the process.
- **Test result (static): CONFIRMED.** `dayKey := now.UTC().Format("2006-01-02")` at de2f19d:`adapter.go:277` (returns `ok=false` on empty bucket); tailer buckets on the data-point's `ts.UTC()` at 84ab8f2:`tailer.go:184` (≠ collector's wall-clock query → widens the dead window); one-shot gate `case tc.otelEverProduced && !tc.driftFired: … tc.driftFired = true` (no reset path) in d17ecc2:`tokens.go`.
- **Runtime test lands phase 1** (red first): a UTC date-roll with the same source still producing must fire NO `token_lookup_miss` and leave `driftFired` available.

**Bug 2 — billable readout steps backward on every source flip.**
- **Hypothesis:** the rate/sparkline path is clamped on `sourceFlipped`, but the cumulative `tok N · bill N` readout reads raw totals unconditionally, so at midnight and on every OTEL↔transcript flip it steps to the other source's absolute total.
- **Test result (static): CONFIRMED.** Rate clamp `sourceFlipped := useOTEL != tc.lastSourceOTEL; if !tc.lastTick.IsZero() && !sourceFlipped {…}` (d17ecc2:`tokens.go`); readout `tokWork := tok.InputTotal+tok.OutputTotal+tok.CacheCreateTotal; tokBill := tokWork+tok.CacheReadTotal` at `rates.go:156-161` — unclamped, reads `snap.Tokens` totals directly.
- **Runtime test lands phase 2** (red first): cumulative readout across OTEL→transcript→OTEL must never decrease.

**Unifying fix principle (OTEL data-model + Prometheus staleness):** an empty cumulative lookup is a *gap* (no-data), a third state distinct from "reset" and "valid lower value." Encode empty → carry last-good; neither the drift alarm (Bug 1) nor the backward step (Bug 2) can then fire. Cited: opentelemetry.io/docs/specs/otel/metrics/data-model (resets-and-gaps); robustperception.io/staleness-and-promql.

## Non-goals
- **Receiver temporality threading (cumulative-vs-delta).** `receiver.go:325-340` drops `AggregationTemporality`/`IsMonotonic`; tailer folds additively (`tailer.go:188`). *Confirmed safe to exclude:* CC exports metrics as **DELTA by default** (`OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` defaults to `delta`; metric `claude_code.token.usage`; verified via claude-code-guide → code.claude.com/docs/en/monitoring-usage), so the additive fold is **correct** — no double-count in the default config, and Bug 2's last-good carry produces a correct monotonic value. Only an explicit cumulative opt-in would double-count, which is optional defensive hardening, not a defect. → Parking lot.
- Not changing the cc-otel receiver beyond what the 6 commits already did.
- Not touching statusline or any non-token surface.
- Not re-deriving PR 1's work — this branch stacks on it unchanged.

## Files to touch
- `thermal/internal/otel/cc/adapter.go` — re-applied (de2f19d, 5819145); Bug 1 day-awareness contract (empty same-day bucket vs genuine miss).
- `thermal/internal/otel/cc/tailer.go` — re-applied (84ab8f2); optional phase 3: align day-key query window to shrink the midnight dead window.
- `thermal/internal/collector/tokens.go` — re-derive the fan-in onto PR 1's restructured `Tick` (either-or source select, source-flip clamp, drift gate); Bug 1 fix (date-roll empty = baseline reset, not drift); Bug 2 fix (carry last-good cumulative across flip so emitted totals never regress).
- `thermal/internal/collector/tokens_test.go` — re-applied fan-in tests; make `fakeOTELView.OTELTokens` honor its `now` arg; add Bug 1 + Bug 2 regression tests.
- `thermal/internal/collector/collector.go` — re-applied `RunConfig.OTELView` + nil-safe wiring (cf00552).
- `thermal/cmd/thermal/main.go` — re-applied `startCcOtel` → adapter → `WithOTELView` startup wiring (cf00552).
- `plans/otel-fanin.md` (+ archive move) — re-applied 2f50151; restore the `+ OTEL fanin` line in `thermal/CLAUDE.md` (scrubbed in PR 1).

## Failure modes to anticipate
- **Re-applying d17ecc2 onto restructured tokens.go.** The fan-in was authored against the OTEL-flavored `Tick`; PR 1's `Tick` now computes `IOTokensPerSec` + `PrettyTokensPerSec` + `lastActiveBytes` inline and writes to `s.*` directly. The either-or source merge, the `sourceFlipped` clamp, and the EMA/rate carry must be re-threaded onto that shape, not pasted. (adapter/tailer commits land on the untouched `otel/cc` package and should cherry-pick cleanly; only d17ecc2 + cf00552 conflict.)
- **Day-aware fix vs source-flip clamp interaction.** Both touch `lastSourceOTEL`/the gate; a date-roll must be distinguishable from a source flip so one doesn't mask the other.
- **max-hold carry pinning a stale value** if a source dies (research gotcha). Needs a staleness bound or "hold until next real sample exceeds it," not "hold forever."
- **No host-clock change (operator decision).** The host clock will NOT be touched. faketime on thermo alone doesn't work for the midnight case either: a live CC session stamps metrics at real time, so a fake-now thermo queries an empty fake-now bucket while real data sits in the real-now bucket (producer/consumer desync). → The midnight verification is a deterministic **integration test** driving the real adapter + tailer + collector across a synthetic UTC midnight via injected timestamps (no real clock). This also closes the harness gap that the Phase-1/2 unit tests leave (they mock the adapter via fakeOTELView, so never exercise the real `dayKey := now.UTC()` against the tailer's `ts.UTC()` bucketing).
- **Drift-gate one-shot semantics.** The fix must reserve `driftFired` for a *genuine* rename — over-correcting (never firing) is as wrong as the current over-firing.
- **Stacked-branch base movement.** If PR 1 is force-pushed again, this branch must rebase onto the new PR 1 head.

## Done criteria
- 6 otel commits re-applied on a new `pr2-otel-fanin` branch stacked on `pr1-no-otel`; `go build`/`vet` clean; **18 Go pkgs + bats green**; `gofmt -l` clean.
- `fakeOTELView.OTELTokens` honors `now`.
- **Bug 1 test (red→green):** a UTC date-roll with the same source producing fires no `token_lookup_miss` and leaves `driftFired` available for a real rename.
- **Bug 2 test (red→green):** cumulative `tok/bill` readout across an OTEL→transcript→OTEL flip never decreases (asserts the cumulative, not just `rate >= 0`).
- (Optional phase 3) tailer day-key query aligned with collector `now.UTC()` to shrink the dead window.
- Midnight verification: a deterministic integration test through the real adapter + tailer + collector across a synthetic 23:59→00:01 UTC boundary — asserts no spurious `token_lookup_miss`/drift, no backward `tok`/`bill`, and that a 23:59 data point doesn't vanish at 00:01. (No host-clock change — operator decision.) Source-flip half (kill exporter → readout holds, rate doesn't spike) covered by the same integration test; optionally confirmed live by hand (needs no clock change). Results/test output referenced in the PR.
- PR opened stacked on PR 1, description reflects scope; `[skip-review]` / commit-gate handled per maintainer.

## Parking lot
- **Adapter partial-cache-miss false-positive at each UTC day start (follow-up issue).** `/code-review` + a verified CC contract (CC OMITS zero-value cache points — code.claude.com/docs/en/monitoring-usage) showed 5819145's process-lifetime sticky bits (`adapter.go:328-350`) fire a false `schema_drift` at the start of each UTC day: day-N input/cacheCreation can land before the day's first `cache_read`, and the lifetime sticky bit reads that as "previously-observed cacheRead now missing." But day-scoping the bits (the obvious fix) would silently miss a genuine cross-day rename — the tradeoff is unclear and it's 5819145's deliberate, tested design (`TestOTELTokens_PartialCacheMiss_FiresDriftAndFallsBack` encodes cross-day cache-vanish = rename). Out of PR 2's named scope (the maintainer's Bug 1 was the *collector's* token_lookup_miss, fixed). → Needs its own design pass; filed as a follow-up, flagged to the maintainer.
- **Optional defensive hardening for the cumulative opt-in.** CC defaults to delta (verified), so the additive fold is correct as-is. If we ever want to support users who set `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative`, thread `AggregationTemporality`/`IsMonotonic` through receiver→JSONL→tailer (last-write-wins for cumulative vs additive for delta). Separate PR; not a defect.
