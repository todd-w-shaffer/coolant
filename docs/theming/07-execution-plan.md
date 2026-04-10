# Execution Plan — Phases, Dependencies, and Parallelization

## Phase diagram

```
Phase 0: Golden capture (pre-refactor snapshot)
    │
    ├─── Phase 1: Theme package ──────┐
    │    (struct, Classic, registry)    │
    │                                  │
    │    Phase 2: Swatch renderer ─────┤  ← can start once Theme struct exists
    │    (fitting room tool)           │
    │                                  │
    ├─── Phase 3: Consolidation ───────┤  ← depends on Phase 1
    │    (migrate all widgets)         │
    │                                  │
    │    Phase 4: Palette design ──────┤  ← uses swatch renderer (Phase 2)
    │    (iron, mono, frost)           │     can run parallel with Phase 3
    │                                  │
    └─── Phase 5: Wiring ─────────────┘  ← depends on Phase 3 + 4
         (--theme flag, docs)

    Phase 6: External themes (future)  ← deferred, not part of this work
```

## Parallelization opportunities

### Parallel pair 1: Phase 1 + Phase 0
- **Agent A:** Write golden capture tests, generate golden files
- **Agent B:** Scaffold `internal/theme/` package with Theme struct and Classic palette
- **No dependency** — Agent A captures from current code, Agent B creates new code

### Parallel pair 2: Phase 2 + Phase 3 (after Phase 1)
- **Agent A:** Build swatch renderer (`cmd/swatch/main.go`)
- **Agent B:** Start widget migration (sparkline.go, thermal.go, headline.go, ...)
- **Loose dependency** — Agent B needs the Theme struct from Phase 1, but doesn't need swatch

### Parallel pair 3: Phase 4 + Phase 3 (after Phase 2)
- **Agent A:** Design and tune Iron/Mono/Frost palettes using swatch renderer
- **Agent B:** Continue widget migration
- **No dependency** — palette data is separate from widget plumbing

### Sequential: Phase 5
- Depends on both Phase 3 (all widgets themed) and Phase 4 (palettes exist)
- Wire `--theme` flag in main.go, register palettes, update docs

---

## Phase 0: Golden Capture

**Goal:** Freeze current render output so we can prove the refactor is invisible.

**Tasks:**
1. Create `thermal/internal/widgets/testdata/` directory
2. Write `TestCaptureGoldenFiles` with fixture snapshot data
3. Generate golden files for: sparkline, severity gradient, thermal levels, overall levels, session diamonds, breathing dots, rates, alerts
4. Commit golden files as a standalone commit

**Output:** `testdata/*.golden` files, test code that compares against them

**Estimated size:** ~150 lines of test code + golden file data

**Agent brief:** "Write golden capture tests for all widget render outputs. Use a fixed fixture snapshot. Generate and commit the golden files. See docs/theming/06-test-strategy.md for the fixture data shape and file list."

---

## Phase 1: Theme Package

**Goal:** Create the `internal/theme/` package with the Theme struct, Classic palette, and registry.

**Tasks:**
1. Create `thermal/internal/theme/theme.go` — Theme struct, ThermalLevel, SessionPhaseColors, GaugeDotColor types
2. Create `thermal/internal/theme/classic.go` — `Classic()` function returning a Theme populated with all current color values
3. Create `thermal/internal/theme/registry.go` — `var Registry map[string]func() *Theme`
4. Implement `Theme.Init()` — LUT generation, GaugeDot formatted string computation
5. Implement `Theme.SeverityColor(v float64, thresh *SparkThresholds) string`
6. Write tests: `TestClassicInit`, `TestRegistryHasClassic`, `TestSeverityColor`

**Output:** Compilable `internal/theme/` package, fully tested, not yet used by anything

**Key decisions already made:**
- Theme struct fields per `docs/theming/02-schema.md`
- Classic palette values per `docs/theming/05-palettes.md` section 1
- LUT generation per schema Init() pseudocode

**Agent brief:** "Create the internal/theme/ package. The struct definition is in docs/theming/02-schema.md. The Classic palette values are in docs/theming/05-palettes.md. Write tests for Init(), SeverityColor(), and registry lookup. Don't wire it into any widgets yet."

**STUB: SparkThresholds type currently lives in widgets/sparkline.go. To avoid circular imports, either move SparkThresholds to theme/ or to a shared types package. Decision: move to theme/ since it's part of the severity color API.**

---

## Phase 2: Swatch Renderer

**Goal:** Build the visual palette testing tool.

**Tasks:**
1. Create `thermal/cmd/swatch/main.go`
2. Render sections: severity gradient ramp, overall gradient, category gradient, threat colors, session diamonds, gauge dots, agent icons, offline sparkline, rates, chrome
3. Support `--theme NAME` and `--all` flags
4. Test: `TestSwatchDoesNotPanic` for each registered theme

**Output:** `go run ./cmd/swatch --theme classic` produces terminal output showing all themed elements

**Dependencies:** Phase 1 (Theme struct must exist)

**Agent brief:** "Build cmd/swatch/main.go — a print-and-exit tool that renders all themed visual elements for a named theme. See docs/theming/03-swatch-renderer.md for the layout spec. Import from internal/theme/. No bubbletea, just direct stdout."

---

## Phase 3: Consolidation Refactor

**Goal:** Wire the Theme struct through all widgets. Classic theme produces identical output to pre-refactor code.

**Approach:** One widget at a time, run golden tests after each. Order by dependency depth (leaf widgets first).

**Sub-phases (sequential within Phase 3):**

### 3a: sparkline.go
- Move `SparkThresholds` to `theme/` package (or resolve import)
- Add `*Theme` field to Gauges (uses severityColor)
- Replace `gradGreen/Yellow/Red`, LUT globals with `theme.*` reads
- Replace `severityColor()` calls with `theme.SeverityColor()`
- Replace `rainbowColors` with `theme.OfflineSparkColors`
- Run golden tests ✓

### 3b: thermal.go + headline.go
- Add `*Theme` field to Headline
- Replace `thermalGradient` with `theme.CategoryGradient`
- Replace `overallGradient` with `theme.OverallGradient`
- Replace offline color literals with `theme.OfflineFg/Bg`
- Run golden tests ✓

### 3c: rates.go
- Add `*Theme` field to Rates
- Replace `phaseRed/Orange/Yellow/Green/Idle` with `theme.SessionPhase.*`
- Replace spawn/death/net color literals with `theme.SpawnColor/DeathColor/NetColor`
- Replace `severityColor()` calls with `theme.SeverityColor()`
- Run golden tests ✓

### 3d: breathedots.go
- Pass `*Theme` through Headline → BreatheDots
- Replace `config.BreatheBaseR/G/B` with `theme.AccentR/G/B`
- Run golden tests ✓

### 3e: alerts.go
- Add `*Theme` field to Alerts (or pass ThreatColors through)
- Replace `ui.ThreatColor[alert.Level]` with `theme.ThreatColors[alert.Level]`
- Run golden tests ✓

### 3f: layout/horizontal.go
- Add `*Theme` field to Horizontal
- Pass theme to all widget constructors
- Replace hardcoded help text colors with theme reads
- Run golden tests ✓

### 3g: ui/colors.go cleanup
- Remove `ThreatColor` map (moved to theme)
- Remove `GaugeDots` init (moved to theme)
- Keep `DimText()`, `ColorText()` helpers (utility, not theme-specific)
- Keep `TypeColor`, `CategoryColor` as defaults (theme can override)
- Keep `CategoryGlyph`, `AgentGlyphHollow/Filled` (structural, not themed)
- Regenerate `CategoryGlyphFormatted` from theme's CategoryColors

### 3h: config/tuning.go cleanup
- Remove `BreatheBaseR/G/B` (moved to theme)
- Keep all non-color constants

### 3i: main.go
- Create Classic theme, pass to `NewHorizontal(theme)`
- Golden tests should still pass ✓

**Agent brief:** "Migrate widgets to use the Theme struct. Follow docs/theming/04-migration-mapping.md line by line. Do one widget at a time. Run `go test ./...` after each sub-phase. The golden tests from Phase 0 must pass at every step. See the sub-phase ordering in docs/theming/07-execution-plan.md Phase 3."

---

## Phase 4: Palette Design

**Goal:** Create Iron, Mono, and Frost palettes with all field values tuned.

**Tasks:**
1. Create `thermal/internal/theme/iron.go` — `Iron()` function
2. Create `thermal/internal/theme/mono.go` — `Mono()` function
3. Create `thermal/internal/theme/frost.go` — `Frost()` function
4. Register all in `registry.go`
5. Use swatch renderer to iterate on values
6. Screenshot each with `freeze` for review
7. Resolve STUBs from `docs/theming/05-palettes.md` (gauge dots, offline colors)

**Output:** Four fully specified, visually validated themes

**Dependencies:** Phase 1 (struct) + Phase 2 (swatch renderer). Can run parallel with Phase 3.

**Agent brief:** "Create Iron, Mono, and Frost themes in internal/theme/. Starting values are in docs/theming/05-palettes.md. Use `go run ./cmd/swatch --theme <name>` to preview. Fill in all STUB fields. Each palette needs its own _test.go with TestXxxInit."

**STUB: This phase requires visual judgment. The agent should render swatches and present screenshots for review. Exact color values will be iterated — the palette doc gives starting points, not final values.**

---

## Phase 5: Wiring

**Goal:** Users can select a theme via `--theme NAME`.

**Tasks:**
1. Add `--theme` flag to `main.go` (default: "classic")
2. Add `COOLANT_THEME` env var as fallback
3. Look up theme from registry, error on unknown name
4. Pass resolved theme to layout constructor
5. Add `--list-themes` flag that prints available names
6. Update help text in horizontal.go helpView to mention theme switching
7. Update CLAUDE.md with `--theme` usage
8. Write test: `TestMainThemeFlag` (parse flag, verify correct theme loaded)

**Output:** `./bin/thermo --theme iron --demo` works

**Dependencies:** Phase 3 (widgets accept theme) + Phase 4 (palettes exist)

---

## Phase 6: External Theme Files (Future — Not This Work)

**Goal:** Users define custom themes in TOML files.

**Tasks (high-level, to be planned later):**
1. Define TOML schema mirroring Theme struct
2. Parse TOML into Theme, validate all required fields
3. Load from `~/.config/coolant/themes/*.toml` at startup
4. Register alongside built-in themes (same registry)
5. Error reporting for malformed themes (which field, what's wrong)

**STUB: Full planning deferred. The internal/theme/ package is designed to make this straightforward — external themes just call the same Init() and register in the same map. The TOML schema maps 1:1 to the Go struct.**

---

## Risk register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Circular import (theme↔widgets) | Blocks compilation | Move SparkThresholds to theme/; widgets import theme/, not vice versa |
| Golden files too brittle (whitespace, trailing chars) | False failures | Normalize output before comparison (trim trailing whitespace) |
| Palette looks bad in braille | Ship ugly theme | Swatch renderer catches this before merge — visual QA gate |
| Performance regression from theme indirection | Slower rendering | Theme fields are pointers/values, no interface dispatch. LUTs are pre-computed. Negligible. |
| Theme struct too large / too many fields | Maintenance burden | Fields are justified in schema doc. Each serves a specific callsite. No speculative fields. |
| CyanColor overloading (offline + death rate) | Semantic confusion | Split into OfflineFg and DeathColor in theme — explicit is better |

---

## Definition of done

- [ ] `./bin/thermo --demo` renders identically to pre-theme code (Classic)
- [ ] `./bin/thermo --theme iron --demo` renders with iron palette
- [ ] `./bin/thermo --theme mono --demo` renders with mono palette
- [ ] `./bin/thermo --theme frost --demo` renders with frost palette
- [ ] `./bin/thermo --list-themes` prints available theme names
- [ ] `go test ./...` passes, including golden file comparison
- [ ] `go run ./cmd/swatch --all` produces visual comparison
- [ ] No hardcoded colors remain outside `internal/theme/` (except structural glyphs)
- [ ] CLAUDE.md updated with `--theme` documentation
