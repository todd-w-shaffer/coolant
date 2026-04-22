# Spec: clickable zones via `bubblezone v2` — headline category filter

**Status:** spec / ready for `/spec-to-ship`
**Date:** 2026-04-11
**Companion to:** `bubblezone-click-regions.md` (stub), `bubblezone-click-regions.brainstorm.md` (design)

---

## 0. Compat check result

`github.com/lrstanley/bubblezone/v2` go.mod requires `charm.land/bubbletea/v2 v2.0.0`, `charm.land/lipgloss/v2 v2.0.0`, `charmbracelet/x/ansi v0.11.6`, go 1.24.2. Coolant runs bubbletea v2.0.2, lipgloss v2.0.2, ansi v0.11.6, go 1.25.0 — same majors, ansi matches exactly, go newer is fine. **Compatible. No remediation needed.**

---

## 1. Goal

Land a single mouse-driven interaction — clicking a headline category cell toggles a one-category filter on the headline's session diamonds and category cells — and ship the architectural plumbing (bubblezone init, root `zone.Scan`, click router, mouse-mode toggle) needed to grow more zones later. Keyboard parity (`[` `]` `\`) is mandatory and shares the same state mutator. The `m` key toggles mouse capture so terminal text selection is never permanently broken.

---

## 2. Non-goals (v1)

- **No agent-pin click.** Detail panel placement is unsolved on a 2–3 row strip; deferred to its own spec.
- **No hover tooltips.** 150 ms repaint cadence makes follow-the-cursor jitter; would need a separate hover ticker.
- **No stat-swap** (clicking CPU/MEM in rates to swap sparkline focus). Deferred until the headline path proves out.
- **No right-click semantics.** Reserved for future. Right clicks are ignored, not bound.
- **No wheel-event handling.** Forwarded by tmux but unused in v1.
- **No filter persistence to TOML config.** Filter is ephemeral inspection state; resets each launch.
- **No widget-level mouse routing.** All `tea.MouseClickMsg` messages stay in the root `Update`. Widgets remain pure renderers in v1.
- **No `lipgloss.Canvas` / compositor adoption.** See §10 — bubblezone v2 is incompatible with it.

---

## 3. Scope

One vertical slice:

1. **State** — `AppState.CategoryFilter string` plus `ToggleCategoryFilter(name string)` and `SetCategoryFilter(name string)` methods.
2. **Keyboard** — `[` previous category, `]` next category, `\` clear filter. `m` toggles mouse capture.
3. **Visual feedback** — when a filter is active, non-matching session diamonds in the headline render dim; matching diamonds render at full intensity. The active headline category cell renders unchanged (already vivid); the *inactive* headline cells dim by one shade.
4. **Mouse** — left-click on a headline category cell calls the same toggle path the keyboard uses.
5. **Mouse toggle** — `m` flips `mouseEnabled` on the root model. `View()` emits `tea.MouseModeNone` when off, `tea.MouseModeCellMotion` when on. Subtle dim `m` glyph appears in the help row alongside `[h]` indicating current state.
6. **First-run hint** — when `$TMUX` is set, push a one-shot alert on first snapshot: "mouse mode active; if clicks don't register, run `tmux set -g mouse on`".

Anything outside that list is out of scope for this ship.

---

## 4. Architecture

### 4.1 Library + init

- Add `github.com/lrstanley/bubblezone/v2` to `thermal/go.mod`.
- `cmd/thermal/main.go` `main()` calls `zone.NewGlobal()` once before `tea.NewProgram(m).Run()`. No `defer zone.Close()` needed (process exits on Run return).
- Root `View()` wraps the composed layout string in `zone.Scan(...)` before assigning to `tea.View.Content`. Scan happens *every* frame whenever mouse capture is enabled; when `mouseEnabled == false`, skip Scan entirely (no markers needed; cheaper render).

### 4.2 Zone ID convention

`<widget>:<entity-id>` — e.g. `cat:build`, `cat:test`, `cat:shell`, `cat:run`. Avoids cross-widget collisions without `zone.NewPrefix()`. Document in `CLAUDE.md` Conventions section as part of the implementation commit.

### 4.3 Zone marking — `headline.go`

- `renderCatCell` wraps its already-styled return string in `zone.Mark("cat:"+cat.Name, styled)`. The marker is zero-width to lipgloss but inflates `len()`. Width math in `headline.go` (notably `len(quip)`, padding calcs) must stay on the *unmarked* content; only the final styled cell is marked. Already correct in current shape — `renderCatCell` builds padded content first, styles, then returns. The Mark wraps *that*. No width drift.
- Audit: `buildOverallCell` uses `len(quip)` after truncation. The quip is not marked. Fine. Document the rule: "wrap only the final styled output; never compute width on a Mark-wrapped string".

### 4.4 Filter state — where it lives

`internal/model/state.go` (`AppState`). Field: `CategoryFilter string`. Methods:

- `SetCategoryFilter(name string)` — sets exactly. Empty string clears.
- `ToggleCategoryFilter(name string)` — if `name` matches current, clears; else sets to `name`.
- `CycleCategoryFilter(direction int)` — `+1` next, `-1` previous, ordered by `collector.Categories`. From empty, `+1` selects the first; `-1` selects the last. From the last with `+1`, wraps to empty (not back to first), so the cycle is `none → A → B → … → Z → none`. Symmetric for `-1`. `direction == 0` is a no-op (defensive guard).

Cycling order is stable: defined by `collector.Categories` slice order. Only categories actually visible participate; cycle skips invisible ones so `]` never lands on a hidden filter the user can't see.

**Import-cycle note:** `visibleCategories` currently lives in `widgets/headline.go` (unexported), which imports `model`. To avoid a cycle, extract a `VisibleCategoryNames(smoothed map[string]float64) []string` helper into `model` (it only needs `collector.Categories`, `collector.FixedCategories`, and the 0.5 threshold — no widgets dependency). The widgets-side `visibleCategories` can then delegate to it or be inlined.

### 4.5 Filter propagation

- `widgets.Headline` consults `state.CategoryFilter` in `renderCatCell` (signature gains a `filter string` param, or method becomes a method on `*Headline` to access state). When filter is set and `cat.Name != filter`, render fg/bg with one gradient step lower (use `theme.CategoryGradient[max(0, level-1)]`). Active cell unchanged.
- **Session diamonds** (`renderSessionDiamonds` in `headline.go`) also consult the filter: when a filter is active, sessions whose dominant category doesn't match render with `ui.DimText`-equivalent dimming on the `⌬` glyph. Matching sessions unchanged.
- **Note:** `widgets.Rates` has no session glyphs — session diamonds and category cells both render in `headline.go`. No changes to `rates.go` for filter propagation. If `renderCatCell`'s signature changes, update `captureThermalLevels` in `golden_test.go` to match (see Cycle 4).

### 4.6 Click routing — root `Update`

Add a case in `cmd/thermal/main.go` `Update`:

- On `tea.MouseClickMsg` with `Button == tea.MouseLeft` (release, not press — bubblezone idiom): iterate `collector.Categories`, call `zone.Get("cat:"+cat.Name).InBounds(msg)`, dispatch to `m.layout.State().ToggleCategoryFilter(cat.Name)` on first hit, return.
- All other mouse messages are dropped silently in v1.
- When `mouseEnabled == false`, no `MouseClickMsg` will arrive (mouse mode is off at the terminal level), but defensively guard the case anyway.

### 4.7 Mouse capture toggle

Root `model` gains `mouseEnabled bool` (default `true`). `m` keypress flips it. `View()`:

- `mouseEnabled == true` → `MouseMode = tea.MouseModeCellMotion`, `Content = zone.Scan(layout)`.
- `mouseEnabled == false` → `MouseMode = tea.MouseModeNone`, `Content = layout` (no Scan).

The `[m]` indicator appears in both surfaces automatically: the short help on the rates row renders via `r.help.Short(r.keys)` which reads from `KeyMap` (once the binding is registered in `internal/keys`, both the full-help overlay in `layout/horizontal.go` and the short-help on the rates row pick it up).

### 4.8 First-run tmux hint

In root `Update`, on the first `snapshotMsg` only (gate with a `bool` field on the model), if `os.Getenv("TMUX") != ""`, push a one-shot alert. **Note:** `AppState.addAlert` is currently unexported — add a public `PushAlert(AlertEntry)` wrapper to `state.go` as part of this implementation (Cycle 6). Call via `m.layout.State().PushAlert(...)`. Hint text: `mouse mode active; if clicks don't register run: tmux set -g mouse on`.

---

## 5. Keyboard parity & `internal/keys` integration

The bubbles/help spec (`docs/backlog/bubbles-help.spec.md`, parallel agent) introduces `internal/keys` as a single source of truth for key bindings. **Assume that package exists.** This spec extends it; does not duplicate.

### 5.1 Bindings to register in `internal/keys`

| Key | Action ID | Mouse equivalent | Description |
|---|---|---|---|
| `[` | `category.prev` | — | Previous category filter |
| `]` | `category.next` | — | Next category filter |
| `\` | `category.clear` | — | Clear category filter |
| `m` | `mouse.toggle` | — | Toggle mouse capture |
| (left-click) | `category.toggle` | click headline cell | Toggle filter for clicked cat |

Each entry in `internal/keys` carries an optional `Mouse string` field (the human-readable annotation, e.g. `"click category"`). The bubbles/help spec's help renderer is expected to display both the key and `(click ...)` annotation on the same line. If the bubbles/help spec lands without the `Mouse` field, this spec's implementation adds it as a struct extension and updates the renderer.

### 5.2 Conflict policy

`[`, `]`, `\`, `m` are not currently bound. No conflict. The implementation must run a quick grep across `cmd/thermal/main.go` and any `internal/keys` definitions before claiming green.

### 5.3 Help row text

`helpView` in `layout/horizontal.go` gains a row (or extends the last row) listing the new bindings using the same `[k] desc` style as existing entries. Rendered through `internal/keys` once available.

---

## 6. Mouse release (`m`) — exact behavior

- Default state: `mouseEnabled = true` at startup.
- Pressing `m` toggles. No persistence between runs.
- When toggled **off**: `View()` returns `MouseMode: tea.MouseModeNone`. Terminal regains native shift-drag selection. `zone.Scan` is skipped (cheaper render). Existing filter state is preserved — keyboard cycling still works.
- When toggled **on**: `MouseMode: tea.MouseModeCellMotion`. `zone.Scan` re-engages. Click routing resumes immediately on next frame.
- Indicator: a single dim glyph in the help row. `[m] mouse on` (default) or `[m] mouse off` (toggled). Always shown so the state is discoverable.
- macOS escape hatch (Option-drag in iTerm2/Ghostty/Terminal.app) is documented in the README as the preferred path for one-off copies — no toggle needed.

---

## 7. tmux + altscreen considerations

### 7.1 What the user must configure

- `set -g mouse on` in `~/.tmux.conf`. Without it, tmux swallows mouse events and never forwards them. README must call this out in the install/usage section.
- AltScreen is already enabled (`tea.View.AltScreen = true` in `main.go:147`); no change.
- For users running coolant as a bottom strip via tmux pane split, mouse events forward to whichever pane has focus. Clicking the strip focuses it (tmux behavior with `mouse on`), then the click registers.

### 7.2 What we test

- `headline_test.go` — rendered output contains bubblezone marker bytes (zero-width sentinel chars) around each visible category cell. Detect with a string scan for the marker prefix bubblezone uses (vendor the constant or use `zone.Scan` round-trip).
- Synthetic `tea.MouseClickMsg` tests in root `Update` (Cycle 5) — no real terminal needed. Coords are constructed to fall within a known cell after a known `WindowSizeMsg`.
- `mouseEnabled = false` path tested by asserting `View().MouseMode == tea.MouseModeNone` and that `Content` does not include marker bytes.

### 7.3 What we do **not** test

- Real tmux forwarding behavior (manual smoke only).
- Real terminal text-selection behavior (manual).
- macOS Option-drag (manual).

---

## 8. TDD plan — six red-green-refactor cycles

Strict per `feedback_tdd_security.md`. One feature per cycle. Failing test first, minimum implementation, then refactor.

### Cycle 1 — bubblezone smoke + zone roundtrip
- **Red:** new `internal/widgets/zone_smoke_test.go` — `TestBubblezoneRoundtrip` calls `zone.NewGlobal()`, marks a sample string with `zone.Mark("smoke:1", "hello")`, asserts the result differs from input (markers added) and that `zone.Scan(marked)` returns a string of equal `lipgloss.Width` to the original. Verifies the dependency imports cleanly under `go 1.25` and the v2 API surface matches expectations.
- **Green:** add `github.com/lrstanley/bubblezone/v2` to go.mod (`go get`), run `go mod tidy`. Test passes once the import resolves.
- **Refactor:** if go.sum churn pulls in unexpected indirect deps, document in commit body.

### Cycle 2 — state plumbing
- **Red:** `internal/model/state_test.go` — `TestCategoryFilter` table-drives `SetCategoryFilter`, `ToggleCategoryFilter` (twice with same name → empty; once with new name → set), and `CycleCategoryFilter` (forward from empty hits first visible cat; forward past last wraps to empty; backward from empty hits last; direction=0 is a no-op). Visible-cat filtering uses a fixture `SmoothedCats` map. Also test `PushAlert` — single alert appears in `Alerts` ring buffer.
- **Green:** add `CategoryFilter string` field; implement the three methods plus `PushAlert(AlertEntry)` (public wrapper around `addAlert`). `VisibleCategoryNames(smoothed map[string]float64) []string` lives in `model` — iterates `collector.Categories`, includes fixed categories always and dynamic when `smoothed[name] >= 0.5`. `CycleCategoryFilter` calls it.
- **Refactor:** verify `widgets/headline.go`'s `visibleCategories` can delegate to the new `model.VisibleCategoryNames` or be inlined; remove duplication if so.

### Cycle 3 — keyboard cycle wired to state + keys registry
- **Red:** `cmd/thermal/main_test.go` (existing file — extend it) — `TestCategoryKeybindings` builds a model, sends `tea.KeyPressMsg{Code: '['}`, asserts state's `CategoryFilter` mutates per cycle rules. Same for `]` and `\`. Reuse the existing `newTestModel` and `pressKey` helpers.
- **Green:** add `CategoryPrev`, `CategoryNext`, `CategoryClear`, `MouseToggle` bindings to `internal/keys/keys.go` KeyMap + Default(). Add cases to the `KeyPressMsg` switch in `Update`. Routes to `m.layout.State().CycleCategoryFilter(±1)` and `SetCategoryFilter("")`. Update `ShortHelp()` and `FullHelp()` to include the new bindings.
- **Green companion:** update `internal/keys/keys_test.go` — `TestDefaultKeyMapBindings`, `TestShortHelpOrder`, and `TestFullHelpGrouping` all assert exact binding counts and positional content. Add the 4 new bindings to each test's expectations.
- **Refactor:** if the switch grows past ~10 cases, extract a `handleKey` method on the model.

### Cycle 4 — visual feedback: dimmed inactive headline cells + session diamonds
- **Red:** `headline_test.go` — `TestInactiveCategoryCellsDim` — when filter is set, non-matching dynamic category cells render with a downshifted gradient. `TestSessionDiamondsDimWhenFiltered` — when filter is set, session diamonds whose dominant category doesn't match render dimmed.
- **Green:** thread `state.CategoryFilter` into `Headline.ViewLines`/`renderCatCell` and `renderSessionDiamonds`. Apply dim styling on the non-matching path. If `renderCatCell` signature changes, update `captureThermalLevels` in `golden_test.go` to match the new signature (pass empty filter to preserve existing golden output).
- **Refactor:** if the dim transform appears in multiple render paths, factor a `theme.DimmedCategory(level int) Thermal` helper.

### Cycle 5 — bubblezone marks + click routing
- **Red:** `headline_test.go` — `TestCategoryCellsAreMarked` calls `zone.NewGlobal()` in `TestMain`, builds a headline at a known width with a known state, asserts the output contains a marker for every visible `cat.Name`. Cross-check via `zone.Get("cat:build").InBounds(...)` after a `zone.Scan(headline.View())` with a synthetic mouse coord.
- **Red companion:** `cmd/thermal/main_test.go` — `TestMouseClickRoutesToToggle` — sets a known window size, runs one Update with a fake snapshot to populate state, calls `View()` and `zone.Scan()` it (the test mimics what bubbletea does), then sends a `tea.MouseClickMsg{X, Y, Button: tea.MouseLeft}` whose coords land in the build cell, asserts state filter == "build".
- **Green:** wrap each `renderCatCell` return in `zone.Mark`; add `zone.NewGlobal()` to `main()`; wrap root view content in `zone.Scan`; add `MouseClickMsg` case in `Update`.
- **Refactor:** if the click router for-loop is non-trivial, factor `dispatchHeadlineClick(msg) bool` returning whether handled. Audit `widgets/headline.go` for `len()` calls on potentially-marked strings; swap for `lipgloss.Width` only if necessary (the marking happens at the outermost wrap, so width math on inner strings is safe).

### Cycle 6 — mouse-mode toggle (`m`) + first-run tmux hint + golden test fix
- **Red:** `cmd/thermal/main_test.go` — `TestMouseToggle` — default `View().MouseMode == tea.MouseModeCellMotion`; after `m` keypress, `MouseModeNone` and `Content` does not contain marker bytes. After second `m`, back to enabled with markers present.
- **Red companion:** `TestTmuxHintFiresOnce` — sets `TMUX=foo` env, sends a snapshot, asserts an alert containing "tmux set -g mouse on" was pushed; sends a second snapshot, asserts no duplicate.
- **Red companion:** `widgets/golden_test.go` — existing goldens diff because of marker injection. Update test harness to run captured output through `zone.Scan` *before* compare. Existing golden files unchanged; only the harness changes.
- **Green:** add `mouseEnabled` field, toggle, `View()` branching, indicator in `helpView`, gate first-run alert on a `tmuxHintShown bool` field, update golden harness.
- **Refactor:** ensure `mouseEnabled = false` path skips `zone.Scan` (perf). Confirm goldens cover both Classic theme and (where applicable) the dim-on-inactive-cell branch.

After Cycle 6: ship via `/commit`. No agent-pin, hover, or stat-swap work — those are separate specs.

---

## 9. File touchlist

### Created

- `thermal/internal/widgets/zone_smoke_test.go` — Cycle 1 smoke test, kept as a regression for v2 compat.

### Modified

- `thermal/go.mod` / `thermal/go.sum` — add `bubblezone/v2`.
- `thermal/internal/model/state.go` — add `CategoryFilter` field, `SetCategoryFilter`, `ToggleCategoryFilter`, `CycleCategoryFilter`, `VisibleCategoryNames`, public `PushAlert` wrapper.
- `thermal/internal/model/state_test.go` — Cycle 2 tests.
- `thermal/cmd/thermal/main.go` — `zone.NewGlobal()`, `mouseEnabled` field, `m` key, `[` `]` `\` keys, `MouseClickMsg` case, `tmuxHintShown` gate, `View()` branching on Scan + MouseMode, first-run hint logic.
- `thermal/cmd/thermal/main_test.go` — Cycles 3, 5, 6 keyboard/mouse tests on root model (file already exists with help-mode tests; extend it).
- `thermal/internal/widgets/headline.go` — `zone.Mark` around each `renderCatCell` return; consult `state.CategoryFilter` for dim styling on non-matching cells and session diamonds.
- `thermal/internal/widgets/headline_test.go` — Cycle 4/5 tests.
- `thermal/internal/widgets/golden_test.go` — pipe captured output through `zone.Scan` before compare; update `captureThermalLevels` if `renderCatCell` signature changes.
- `thermal/internal/layout/horizontal.go` — extend `helpView` with `[m]`, `[ ]`, `[\]` entries; thread keys via `internal/keys` once available.
- `thermal/internal/keys/keys.go` — register `category.prev`, `category.next`, `category.clear`, `mouse.toggle` bindings.
- `thermal/internal/keys/keys_test.go` — update `TestDefaultKeyMapBindings`, `TestShortHelpOrder`, and `TestFullHelpGrouping` to include the 4 new bindings.
- `thermal/internal/theme/theme.go` — optional helper `DimmedCategory(level int) Thermal` if Cycle 4 refactor calls for it.
- `CLAUDE.md` — add the lipgloss canvas warning (see §10) and the zone-ID naming convention.
- `README.md` — short note on `tmux set -g mouse on` and macOS Option-drag selection.
- `docs/backlog/bubblezone-click-regions.md` — flip status to "shipped" with link to commit, after merge.

---

## 10. Warning to add to CLAUDE.md

Append to the **Go (API gotchas)** section:

> **bubblezone v2 is incompatible with `lipgloss.Canvas` / the v2 compositor.** We register clickable zones via `github.com/lrstanley/bubblezone/v2`. The maintainer warns that bubblezone may not work when using the lipgloss v2 canvas/compositor — the marker-scan model assumes a flat string. Coolant composes layouts via string concatenation in `layout/horizontal.go`; do not introduce `lipgloss.Canvas` without re-evaluating the zone story (and likely swapping libraries — the bubblezone maintainer hints a successor is coming).
>
> **Zone ID naming convention:** `<widget>:<entity-id>` — e.g. `cat:build`, `agent:abc123`, `stat:cpu`. Avoids cross-widget collisions without `zone.NewPrefix()` overhead.
>
> **Width math near marks:** bubblezone markers are zero-width to `lipgloss.Width` but inflate `len()`. Wrap only the final styled output in `zone.Mark`; never compute width on a Mark-wrapped string. Prefer `lipgloss.Width` over `len` anywhere a string may be marked.

---

## 11. Definition of done

All true:

- [ ] All six TDD cycles green; each cycle has a corresponding commit (or one squash commit with cycle markers in the body's `Changes:` block per coolant commit style).
- [ ] `cd thermal && go test ./...` passes.
- [ ] `bats tests/` passes (no regressions in bash layer; no new bats tests expected since this is Go-only).
- [ ] `cd thermal && go build -o ../bin/thermo ./cmd/thermal/` succeeds.
- [ ] `./bin/thermo --demo` boots, headline cells are clickable, `[` `]` `\` `m` keys work, dim feedback visible on non-matching headline cells and session diamonds.
- [ ] `./bin/thermo` (live mode) same behavior on a real machine.
- [ ] Inside tmux with `set -g mouse on`: clicks register; first-run hint fires once.
- [ ] With `m` toggled off: terminal text selection works normally; help row shows `[m] mouse off`.
- [ ] Existing golden tests for Classic theme still pass after harness update.
- [ ] `CLAUDE.md` updated with §10 warning + naming convention.
- [ ] `README.md` updated with tmux mouse note.
- [ ] Stub spec `docs/backlog/bubblezone-click-regions.md` flipped to "shipped" with commit link.
- [ ] No Co-Authored-By lines in any commit; commit messages follow coolant style (imperative subject, `Recipe:` + `Changes:` body).

---

## 12. Open items intentionally deferred

Carried forward from the brainstorm; do **not** address in this ship:

- Filter persistence to TOML (decision: ephemeral; may revisit).
- Right-click semantics (decision: unbound for now).
- Click-through on offline overlay (decision: zones suppressed when offline; the offline path replaces category cells with a quip already, so no marks emitted).
- Wheel-event scroll for alert log (separate spec).
- bubblezone Scan CPU profile under 150 ms cadence (manual profile after Cycle 6; if >1% CPU, gate Scan on `mouseEnabled` — already done as a side effect).
- Agent-pin click + detail panel layout (separate spec; blocked on layout story for the strip).
- Hover tooltips (separate spec; needs decoupled hover ticker).
- Stat-swap on rates click (separate spec).

---
