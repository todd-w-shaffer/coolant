# `tok N` readout → billable total (include cache reads/creates)

## Goal
Make the rates-line `tok N` cumulative readout match what Claude Code reports
as "X tokens" for an agent — billable total including cache reads and cache
creates. Today the readout sums only `InputTotal + OutputTotal`, which is
~25–30% of the full billable figure on cache-heavy work and misleads the
operator (observed: 4 agents reported 91/79/89/82k = ~340k total, coolant
displayed `tok 92K`).

Phase 0 first tightens an OTEL fan-in gap found during research — the
adapter currently returns `ok=true` with cache fields silently zeroed
if CC emits `input`/`output` but the cache `type` attributes are
missing. Old 2-field sum masked it; new 4-field sum makes the failure
visible as a 3-4× downward step on source flip. Phase 1 then ships the
readout change.

## Diagnosis
- **Hypothesis:** `rates.go:143` sums only `InputTotal+OutputTotal` and excludes
  `CacheCreateTotal+CacheReadTotal`. Claude Code's per-agent "X tokens" string
  reports the all-in billable total. Cache hit ratios of ~70–75% on the
  observed session explain the ~3.7× undercount (92/340 ≈ 0.27).
- **Falsifiable test:** With thermo running against an active cache-heavy
  session, compare `tok N` against
  `(tok.InputTotal+tok.OutputTotal+tok.CacheCreateTotal+tok.CacheReadTotal)`
  using a one-off `thermo statsdump`-style probe or a temp log line. The
  ratio should match `(1 - CacheHitRatio)` roughly. Sum-of-four should match
  Claude Code's reported per-agent token totals within ~5%.
- **Test result:** Inferred from screenshot evidence — 4 agents reported
  ~340k tokens total in Claude Code's UI; coolant displayed 92K; cache % on
  screen confirms a high read ratio. Will re-verify quantitatively in
  Phase 1 against a live session before merging. (If the math doesn't line
  up at verification time, return to research with the negative result.)

## Non-goals
- Touching the TKNS sparkline. Sparklines visualize rate, not cumulative;
  `IOTokensPerSec` (excludes cache create/read) is still the right rate
  signal — it tracks "fresh model traffic", and that *should* drop to 0
  between bursts so the amplitude tracks current activity. Sparkline
  labeling stays `TKNS`.
- Touching the PRTY sparkline. Same reasoning — it's a rate, not a total.
- Renaming `IOTokensPerSec` or `TokenStats.{Input,Output,CacheCreate,CacheRead}Total`.
  No collector-side changes.
- Statusline. Per root CLAUDE.md, statusline owns per-session usage; thermo
  owns system-wide.
- Adding a separate "fresh I/O total" readout. The sparkline conveys fresh
  activity; the cumulative readout conveys billable total. One number per
  semantic on the rates line.
- Cache hit ratio formula. Stays as canonical Anthropic
  `cache_read / (input + cache_create + cache_read)` per `types.go:62`.

## Files to touch
**Phase 0 — close OTEL silent-zero gap:**
- `thermal/internal/otel/cc/adapter.go` — `OTELTokens()` at ~lines 270-298:
  add a partial-data guard. If `ok=true` is about to be returned but
  cache `type` values (`cacheCreation`, `cacheRead`) returned 0 while
  any other type returned > 0 AND we've previously observed those cache
  types (sticky bit), fire `ObserveTokenSchemaDrift("cache_field_missing", …)`
  and return `ok=false` so transcript stays authoritative. Sticky bit
  prevents false-positive on cold start before any cache traffic.
- `thermal/internal/otel/cc/adapter_test.go` — new test:
  `TestOTELTokens_PartialCacheMiss_FiresDrift` seeds input+output but no
  cache types after a prior successful read; asserts drift fires and
  `ok=false`.

**Phase 1 — billable-total readout:**
- `thermal/internal/widgets/rates.go` — line 143: change
  `format.FormatCount(tok.InputTotal+tok.OutputTotal)` to sum all four
  totals (Input + Output + CacheCreate + CacheRead). Update the
  comment block at lines 137–139 to reflect billable-total semantics
  and explain why this differs from the TKNS sparkline (rate excludes
  cache).
- `thermal/internal/widgets/rates_test.go` — update
  `TestTokReadoutShowsCumulative`, `TestTokReadoutMonotonic`, and
  `TestTokReadoutAcrossOTELFlip` to populate all four token fields and
  assert the readout matches the four-field sum. Add one new test
  that asserts cache-heavy workloads (`CacheReadTotal >>
  InputTotal+OutputTotal`) produce a readout ≫ the old two-field sum
  — locks in the semantic against future regression.
- `thermal/internal/widgets/testdata/rates_line.golden` — regenerate
  if the zero-state output changes (it shouldn't — `0+0+0+0 = 0` still
  formats as `tok 0` — but verify).

## Failure modes to anticipate
- **OTEL fan-in vs transcript parity.** Research confirmed OTEL adapter
  populates all four counters cumulatively. The asymmetric failure
  mode (partial-data silent-zero) is closed by Phase 0 — adapter
  fires drift and falls back to transcript if cache types vanish
  while input/output keep flowing.
- **Transcript scanner `lastMsgIDs` dedupe.** The cache-create / cache-read
  fields in the usage object are read once per `parseTranscriptLine`
  along with input/output (`tokens.go:88–93`). Dedupe is per-id at line
  361, so cache deltas are already counted at the same time as
  input/output deltas — no risk of asymmetric accumulation. Sanity-check
  in `tokens_test.go` to confirm `TokenAccumulator.Apply` already adds
  all four atomically.
- **Demo data path.** `internal/demo/demov2.go` synthesizes `TokenStats`;
  if it doesn't drive `CacheCreateTotal` / `CacheReadTotal` alongside
  Input/Output, `--demo` will look quieter than reality. Verify the
  demo numbers still look believable; bump cache totals proportionally
  if needed.
- **Backward jumps on source flip.** Even with all four counters
  populated on both sources, transcript baseline and OTEL baseline are
  independent — if OTEL goes live with `CacheRead=2M` while transcript
  shows `0`, then OTEL drops, the readout snaps back. Existing
  `TestTokReadoutAcrossOTELFlip` covers this for input/output; expand
  to cover cache fields too. (This was already noted as a known
  limitation in the prior plan; not regressing the four-field version.)
- **Display width.** Adding cache reads can push `tok N` from `tok 92K`
  to `tok 1.2M` or `tok 12M` on long sessions. `format.FormatCount`
  handles the K/M/B ladder, but rates.go does no width allocation —
  check the rates-line layout absorbs an extra character or two
  without wrapping. If it wraps, prefer truncating the help short
  string over rewriting the readout.
- **Semantic confusion with cache hit %.** Reading `tok 340K · cache 73%`
  means "340k billable, of which 73% came from cache". Make sure the
  comment in rates.go reflects this so future readers don't accidentally
  switch back to the two-field sum thinking it's "more honest". The
  Anthropic billing model charges for cache reads (at a discount) and
  cache writes (at a premium) — they are not free.

## Done criteria
- Phase 0: OTEL adapter `OTELTokens()` fires
  `ObserveTokenSchemaDrift("cache_field_missing", …)` and returns
  `ok=false` when input/output have data but cache types don't,
  provided cache types have been observed previously in the same
  process (sticky bit). New test in `adapter_test.go` exercises this.
- Phase 1: Rates line renders `tok <N>` where N =
  `InputTotal + OutputTotal + CacheCreateTotal + CacheReadTotal`,
  formatted via `format.FormatCount`.
- Live verification against a real cache-heavy session confirms `tok N`
  is within ~5% of summed per-agent token reports from Claude Code's UI.
- Tests in `rates_test.go` updated; new cache-heavy-cumulative test
  passes; `go test ./...` in `thermal/` is green.
- `bats tests/` still green (no bash-layer changes expected, but the
  full bash suite stays green to confirm no plumbing leak).
- Comment block at rates.go:137–139 explains billable-total semantics
  and why TKNS sparkline still excludes cache (rate vs cumulative
  distinction).
- Demo mode (`./bin/thermo --demo`) shows the counter advancing
  visibly and the numbers feel proportional to a real session.

## Parking lot
(empty)
