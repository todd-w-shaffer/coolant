# Log-scale sparkline rendering

## Goal
Add a log-scale rendering mode for sparklines so values across multiple orders
of magnitude are distinguishable by visual height instead of all pinning to the
visual ceiling. Per-slot opt-in: Token + Pretty get log scale (signals span
~10 io/s idle to 10k+ io/s heavy use); CPU/MEM keep linear (bounded
percentages); Decomp keeps autoscale (current behavior — wide dynamic range is
already handled).

## Non-goals
- Not changing CPU/MEM rendering. Their natural 0-100% range is well-served by
  linear scale.
- Not changing Decomp. It already autoscales via `g.peaks[slot]`; the user
  hasn't complained about it and the autoscale is the right call there.
- Not changing how colors are computed. Severity thresholds (warn/crit) still
  fire on raw values — color of 1500 io/s is still yellow, 5000 is still red.
  Only HEIGHT changes; color logic is untouched.
- Not introducing a user-facing config knob for scale mode in v1 — per-slot
  scale strategy stays a code-level decision baked into `Gauges.View`. Parking
  lot if user demand surfaces.
- Not changing how `g.peaks[slot]` is computed. Token/Pretty don't use the
  autoscale peak today; that doesn't change.
- Not changing the visible-window length, scroll rate, or any other sparkline
  behavior. This is rendering-only.

## Files to touch
- `thermal/internal/widgets/sparkline.go` — extend `RenderSparkline` to accept
  a scale-mode parameter (linear vs log) OR add a sibling
  `RenderSparklineLog` that wraps the transform. The transform happens BEFORE
  the height normalization but AFTER (or alongside) the color computation —
  color must still derive from raw values so warn/crit thresholds keep their
  absolute meaning.
- `thermal/internal/widgets/gauges.go` — pass the log-scale flag for SlotToken
  and SlotPretty entries in the `allGauges` array; CPU/MEM/Decomp keep linear.
- `thermal/internal/widgets/sparkline_test.go` (or `golden_test.go`) — add
  test coverage for the log transform: known input series → expected braille
  output. Possibly a new golden fixture for log-rendered sparkline.

## Failure modes to anticipate
- **Log(0) is undefined**, log(values < 1) is negative. Token/Pretty values
  can be 0 (idle) or fractional (after decay rounds). Need `log1p(x)` style
  (log(1+x)) or floor-clamp to avoid NaN / negative heights. log1p is the
  classic fix and keeps log(0) = 0 → height 0.
- **Color/height divergence**: if the transform is applied to the data slice
  before passing to the render pipeline, the color computation
  (`theme.SeverityColor(value, thresh)`) will see the transformed value and
  fire warn/crit at the wrong inputs. Color must use raw values. The transform
  needs to happen INSIDE the height-computation path only.
- **Per-column height interpretation**: currently `height = value / max`. With
  log, it becomes `height = log1p(value) / log1p(max)`. The max input itself
  must come from raw-value land (still warn or crit threshold); we just
  transform both operands consistently.
- **Scale anchor choice**: if we keep `max = warn (1000)`, then `log1p(1000)
  / log1p(1000) = 1.0` — anything ≥ 1000 still clips. We probably want to
  anchor at `crit (4000)` for log mode, because the whole point is the dynamic
  range above warn. log1p(4000) = 8.3; a 1000 burst renders at log1p(1000) /
  log1p(4000) = 6.9 / 8.3 ≈ 83%. 10000 io/s reads as log1p(10000) /
  log1p(4000) = 9.2 / 8.3 ≈ 111% (clips but only barely). That's reasonable.
- **Test/golden fragility**: existing `gauges.golden` includes synthetic
  Token slot output at the current linear-max calibration. Switching Token to
  log will shift the rendered braille — golden must refresh.
- **Edge fade interaction**: the sparkline edge fade dims the outer 3 columns;
  unrelated to height computation, should pass through cleanly. Verify.
- **Performance**: log10 / log1p per column per render frame. At 30fps and ~80
  columns visible per sparkline × 2 slots using log = ~4800 log calls/sec.
  Math is cheap (sub-microsecond per call) but worth confirming we're not in
  a tight inner loop that could be vectorized or cached.
- **Width math near markers**: bubblezone markers don't touch sparkline cells
  (per prior research), but the width math conventions in sparkline.go use
  `lipgloss.Width` over `len()` — the transform shouldn't change rendered
  output width, but verify.

## Done criteria
- `RenderSparkline` (or a new sibling) supports log-scale mode without
  breaking existing linear-mode callers (CPU/MEM/Decomp render identically
  byte-for-byte).
- SlotToken and SlotPretty render in log mode: a 100 io/s burst is visibly
  shorter than a 1000 burst, which is visibly shorter than a 4000 burst.
- Colors still fire correctly: 500 io/s = green-ish, 1500 = yellow, 5000 = red.
  Threshold semantics preserved.
- `cd thermal && go test ./...` green; `bats tests/` green at repo root.
- New unit test pinning the log transform's expected per-column heights at
  representative inputs (1, 10, 100, 1000, 4000, 10000).
- Goldens (`gauges.golden`, possibly `gauges_alt_visible.golden`) refreshed
  to reflect the new log rendering for Token slot.
- Manual smoke: `./bin/thermo`, generate token bursts of varying magnitudes,
  visually confirm height differentiation up to crit and beyond.

## Parking lot
(empty)
