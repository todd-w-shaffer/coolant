# Tokens widget → cumulative since launch

## Goal
Replace the rates-line token readout (currently `io N/s` — a decayed peak of
`IOTokensPerSec`) with a cumulative `tok N` total of input+output tokens since
thermo started. Cache hit % stays. The TKNS sparkline row is out of scope —
sparklines visualize rate, not cumulative.

## Diagnosis
- **Hypothesis:** The rates-line readout in `thermal/internal/widgets/rates.go`
  computes `humanizeRate(r.decayedIOPeak())` from
  `state.Current.Tokens.IOTokensPerSec`. Swapping that to
  `Tokens.InputTotal + Tokens.OutputTotal` gives "cumulative since launch"
  with no collector change — `TokenStats.{InputTotal,OutputTotal}` are
  already documented as cumulative since collector start, and they stay
  consistent across the transcript↔OTEL source flip (collector/tokens.go:60,
  collector/tokens.go:243-247).
- **Falsifiable test:** Read `TokenStats` doc comment + the OTEL fan-in
  branch in `tokens.go::Tick`; confirm `InputTotal/OutputTotal` are
  monotonically non-decreasing across a source flip (no synthetic delta /
  baseline reset zeros them).
- **Test result:** Confirmed. `types.go:59-67` describes the totals as
  cumulative; `tokens.go:249-303` resets per-tick *deltas* on source flip
  but leaves the totals unmodified — OTEL's cumulative replaces the
  transcript baseline directly on the `out` struct.

## Non-goals
- Touching the TKNS or PRTY sparklines. Sparkline visualization of a
  monotonically-growing counter is uninteresting; rate stays the right
  signal for the sparkline row.
- Touching cache hit ratio. Stays as-is on the rates line.
- Touching the io-peak decay machinery beyond removing it from `Rates`.
  Half-life math (`ioPeakHalfLife`, `decayedIOPeak`, `ioPeak`, `ioPeakAt`,
  the `Update` snap-up) can be deleted outright if no other widget calls it.
- Renaming `IOTokensPerSec` or removing it from `TokenStats`. The sparkline
  still consumes it.
- Adding a new field to `TokenStats`. Cumulative is already there as
  `InputTotal + OutputTotal`.
- Statusline changes. Per root CLAUDE.md, statusline owns per-session
  token usage; thermo owns system-wide. Cumulative-since-launch is
  thermo-shaped.

## Files to touch
- `thermal/internal/widgets/rates.go` — replace the `io N/s` component
  with `tok N` (cumulative `InputTotal+OutputTotal`); drop the peak-decay
  fields and `Update` snap-up logic (only consumer of `decayedIOPeak`).
  Format with the existing `humanizeRate` helper (the `N → 1.2k → 1.5M`
  ladder works for cumulative counts too; rename if the `Rate` suffix
  starts feeling wrong — defer unless it actually reads weird).
- `thermal/internal/widgets/rates_test.go` — update / replace the
  io-peak-decay tests to assert cumulative formatting instead, and that
  the value monotonically advances across snapshots.

## Failure modes to anticipate
- **OTEL fan-in source flip.** When OTEL goes live or drops silent, the
  cumulative totals on `out` change source. The diagnosis confirms they
  stay monotonic across the flip, but a regression here would show as
  the readout briefly snapping backward. Add a regression test that
  simulates a source flip and asserts the displayed value is
  non-decreasing across ticks.
- **Demo data path.** `internal/demo/demov2.go` synthesizes `TokenStats`
  for `--demo` mode. The scripted narrative drives IOTokensPerSec
  bursts; if it doesn't set `InputTotal/OutputTotal` (or sets them to
  fixed values), the readout will look dead. Check + extend the demo
  to keep the cumulative counter advancing.
- **Offline branch.** `Rates.View()` short-circuits on `!s.Online` to
  show `OFFLINE Xs`. New readout should also be suppressed in that
  branch (likely no change needed — the branch returns early before
  any rates render).
- **Removal of peak-decay state breaks something else.** Grep for any
  other caller of `ioPeak` / `ioPeakAt` / `decayedIOPeak` /
  `ioPeakHalfLife` before deletion. If `Rates` is the only consumer
  (expected), delete; otherwise scope carefully.
- **Visual readability of a monotonically-growing number.** A counter
  ticking up across 5-6 orders of magnitude over a long session might
  hit `1.5M` and stay there for ages, looking static. Acceptable for v1
  — if it feels dead in practice, a follow-up could add a subtle
  underline-color severity (cool→warm) keyed to total or per-minute
  derivative. Park, don't pre-build.
- **Label choice.** `io N/s` reads as a rate. `tok 1.5M` (no `/s`)
  conveys cumulative. Don't ship `tok N/s` — that would be a lie.

## Done criteria
- Rates line renders `tok <N>  ·  cache <X.X>%` where N is cumulative
  input+output since thermo launch, formatted via `humanizeRate`.
- Decay machinery in `Rates` is removed (or kept *only* if a non-rates
  consumer surfaces — verify by grep).
- `rates_test.go` covers: format on zero, format across the
  thousands/millions ladder, monotonicity across snapshots, and the
  OTEL source-flip case (no backward step).
- `go test ./...` in `thermal/` is green.
- Demo mode (`./bin/thermo --demo`) shows the counter advancing
  visibly during burst phases.

## Parking lot
(empty)
