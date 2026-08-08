# Per-sparkline toggles in thermo

## Goal
Let users hide/show each sparkline in thermo via single-key toggles, with the
keys self-documenting in the help overlay. Bundled with this: add a **token
sparkline** as a new graph (data path already shipped via the token-counter
plan; no widget exists yet — only inline text on the rates line). Sparklines
are the only widgets subject to the toggle system; gauges, headline, agent
dots, heat bloom/rails, alerts, battery are always-on chrome. The sparkline
row holds at most **3 visible sparklines** at any time; toggling on a
sparkline when 3 are already shown is a **silent no-op** — the user must
toggle one off to free a slot first.

Final result is an extensible system — adding a new sparkline takes one
registration call in `internal/keys/`, and the help overlay auto-documents it
because Phase 0 rewrites `helpView()` to consume the registry.

## Naming convention
- **"sparkline row"** for the user-facing concept (matches existing source:
  `horizontal.go:35` comment "sparkline gauges", `:263` help header
  literal "sparklines").
- **`visibleMask` / `Visible[id]`** for the internal state.
- No "stage" terminology — it was draft-internal; not in the codebase.

## Non-goals
- No persistence of visibility across thermo restarts in v1 (parking lot).
- No reordering or resizing — toggles are visibility only.
- No mouse / click-to-hide. Keyboard only.
- No statusline changes (per root CLAUDE.md "Surfaces": system-wide signals
  stay in thermo).
- No auto-eviction / LRU when full — toggle-on is a silent no-op until a slot
  is freed.
- No fix for the literal "SWAP" label (which actually shows decompressions —
  see Parking lot). Out of scope; that's a CLAUDE.md + label rename of its
  own.
- No changes to event-bus / collector layer beyond the additive token ring
  buffer. Token *data* already lands via `collector/tokens.go`.
- No new widget structs beyond extending `Gauges` (per reuse research,
  `RenderSparkline()` is already parameterized; no `TokenSparkline` struct).

## Diagnosis
N/A — this is a feature plan, not a perf/bug plan.

## Files to touch
**Phase 0 — `helpView()` registry refactor**
- `thermal/internal/layout/horizontal.go` — rewrite `helpView()` (lines
  254-280) to generate help text from `keys.KeyMap` instead of hardcoded
  strings. Add a min-height guard so the overlay doesn't overflow small
  terminals.
- `thermal/internal/keys/keys.go` — extend each `key.Binding` with any
  metadata `helpView()` needs that isn't already on `key.WithHelp` (likely
  none — verify during phase 0).
- `thermal/internal/layout/horizontal_help_test.go` — refresh the test that
  iterates `km.FullHelp()` against `helpView()` output.

**Phase 1 — visibility mask + token data ring buffer**
- `thermal/internal/widgets/gauges.go` — extend `[3]springState` /
  `[3][]float64` to `[4]` for token; add `visibleMask` field; skip
  `RenderSparkline()` calls for hidden slots (don't post-filter — skip
  upstream to avoid any future zone-marker leak per CLAUDE.md bubblezone
  policy).
- `thermal/internal/widgets/sparkline.go:103-117` — add `TokenSparkThresh()`
  alongside the existing `CPUSparkThresh` family (~3 lines).
- `thermal/internal/collector/types.go` and/or `thermal/internal/model/` —
  add a ring buffer for `TokensPerSec` history (parallel to how
  `Decompressions` feeds the current third sparkline).
- `thermal/internal/config/tuning.go` — add token threshold constants
  (warn/crit) consistent with the existing sparkline threshold style.

**Phase 2 — keymap + dispatch + default visible set**
- `thermal/internal/keys/keys.go` — add 4 toggle bindings: `CPU` / `MEM` /
  `Decomp` / `Token`. Picked from free letters (e.g. `1 2 3 4`, or letter
  choices made during phase). Group these under a new `FullHelp()` row.
- `thermal/cmd/thermal/main.go:243-265` — add `case key.Matches(msg,
  m.keys.ToggleCPU):` etc. that call `m.layout.ToggleSparkline(id)`.
- `thermal/internal/layout/horizontal.go` — add `ToggleSparkline(id)`
  method that flips the mask on `Gauges`, enforcing the 3-visible
  invariant centrally.
- Default visible set: **CPU + MEM + Token.** Wired in `Gauges.New()` init.

**Phase 3 — tests**
- `thermal/internal/widgets/gauges_test.go` — new
  `TestSparklineVisibilityInvariant` covering: (a) default visible-set is 3;
  (b) toggle-off frees a slot; (c) toggle-on when 3 visible is a no-op;
  (d) toggling off then on round-trips.
- `thermal/internal/widgets/golden_test.go` — refresh `sparkline_classic`
  golden; add `sparkline_default_visible` (CPU+MEM+Token) and
  `sparkline_alt_visible` (CPU+MEM+Decomp) fixtures.
- `thermal/internal/layout/horizontal_help_test.go` — refresh to assert
  every toggle key from `KeyMap` appears in `helpView()` (existing test
  pattern at `horizontal_help_test.go:88-98` already does this for the
  base keymap; adding to `KeyMap` will naturally exercise it).
- `thermal/internal/widgets/help_smoke_test.go` — refresh smoke fixture.

## Failure modes to anticipate
- **`helpView()` drift bug.** Current `layout/horizontal.go:254-280`
  hardcodes the help strings; the test `TestHelpViewCoversAllBindings`
  (`horizontal_help_test.go:88`) iterates `km.FullHelp()` and asserts every
  key appears in the rendered text — so adding bindings without phase 0's
  refactor will turn the test red immediately. Phase 0 is therefore both
  the fix AND the safety net.
- **Help overlay overflow.** Current overlay is 6 lines. Adding 4 toggle
  entries → ~10 lines. No min-height guard exists. Add one in phase 0.
- **Skip `View()` for hidden sparklines, don't post-filter.** Sparklines
  don't currently call `zone.Mark` so there's no live leak, but the
  bubblezone policy in `thermal/CLAUDE.md` is clear: never post-filter
  marker-bearing strings. Toggle dispatch must short-circuit upstream.
- **Slot accounting drift.** Two code paths can change visibility: init-time
  defaults AND user toggles. The 3-visible invariant must hold across
  both. A 4-visible state must be impossible to reach, not just unlikely.
  Centralize via a single `Toggle(id)` method that enforces the cap.
- **Height-based truncation collision.** `gauges.go:152-172` already drops
  sparklines bottom-up when terminal height shrinks. The new visibility
  mask runs *before* this truncation — visibility filters first, then the
  height fallback runs over the visible-only set. Document this ordering
  in code comments.
- **"SWAP" label is a lie.** The third sparkline is labeled SWAP but
  renders `Decompressions`. Toggle key for it should be labeled DECOMP in
  the help overlay (since users see the new help text), even though the
  on-screen sparkline label still reads SWAP. Mismatch is intentional and
  documented in parking lot; the rename is a separate plan.
- **Key collisions.** Free letters from research: a, b, d, e, f, g, j, k,
  l, n, o, p, r, s, t, u, v, w, y, z (plus digits). Toggle keys must avoid
  the existing 9 bindings (h, q, c, x, [, ], \, m, i, ? as Help alt).
  Specific choices made during phase 2 with a one-liner rationale.
- **Bubbletea v2 key matching.** Uses `tea.KeyPressMsg`, not v1's `KeyMsg`.
  Cases use `key.Matches(msg, m.keys.X)` — follow existing pattern.

## Done criteria
- `helpView()` is generated from the registry; adding a `key.Binding` to
  `KeyMap` automatically surfaces in the overlay.
- Every sparkline (CPU, MEM, Decomp, Token) has a single-key toggle.
- At most 3 sparklines visible at once; toggle-on-when-full is a silent
  no-op — covered by unit test.
- Default visible set on cold start is **CPU + MEM + Token**.
- Token sparkline renders from a ring buffer that updates with each
  collector snapshot, with thresholds via `TokenSparkThresh()`.
- Pressing `h` shows the toggle keys alongside human labels AND indicates
  which sparklines are currently visible.
- `cd thermal && go test ./...` green. `bats tests/` green at repo root.
- Manual smoke: `./bin/thermo --demo` — toggle each sparkline, verify
  default visible set on launch, attempt a 4th-visible toggle and confirm
  silent no-op, verify help overlay matches reality and doesn't overflow.

## Parking lot
- Persist visibility across thermo restarts (config TOML or `~/.coolant/`
  state file). v1 is intentionally session-scoped.
- Rename literal "SWAP" label → "DECOMP" in `gauges.go:24-29` and update
  `thermal/CLAUDE.md` to reflect that the third sparkline shows
  decompressions, not swap bytes. Separate plan — touches user-visible
  labels and docs.
- Clamp `helpView()` to terminal height with a "... more, scroll" hint if
  the overlay exceeds available rows.
- LRU auto-eviction if silent no-op feels clunky after dogfooding.
