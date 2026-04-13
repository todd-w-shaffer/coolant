# Thermographic Accent Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a soft, left-flowing thermographic bloom behind the headline strip's left zone, breathing slowly at baseline and intensifying with composite system heat. The right cluster is inviolable — no visual bleed under any condition.

**Architecture:** New `HeatBloom` widget under `internal/widgets/` exposes a per-cell background color grid for the headline's left zone. The headline consumes `bloom.BgAt(col, row)` in place of its current single `iconBg` value when rendering left-zone cells. Motion is spring-driven (harmonica) off a new `CompositeHeat()` scalar exposed on `model.AppState`. Colors flow through a new `BloomRamp` LUT on `theme.Theme`, pre-computed in `Init()` following the existing `overallTempLUT` pattern. Rendering is ANSI truecolor — no raster, no terminal-capability branching.

**Tech Stack:** Go 1.25+, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/harmonica`, `github.com/lucasb-eyer/go-colorful`.

**Spec:** `docs/superpowers/specs/2026-04-12-thermographic-accent-design.md`

---

## File Structure

**Create:**
- `thermal/internal/widgets/heatbloom.go` — the widget (struct, constructor, SetSize, Update, AnimTick, BgAt)
- `thermal/internal/widgets/heatbloom_test.go` — unit + golden tests
- `thermal/internal/widgets/testdata/heatbloom_*.golden` — frozen frames

**Modify:**
- `thermal/internal/model/threat.go` — extract `compositeHeatScore` helper, preserve `Classify` behavior
- `thermal/internal/model/state.go` — add `CompositeHeat() float64` method
- `thermal/internal/model/threat_test.go` — new test asserting `CompositeHeat` buckets match `Classify` at thresholds
- `thermal/internal/theme/theme.go` — add `BloomRamp` field + `BloomColor(heat, radial)` method + LUT precompute in `Init`
- `thermal/internal/theme/frappe.go` — populate bloom ramp (primary target)
- `thermal/internal/theme/classic.go` — populate bloom ramp
- `thermal/internal/theme/iron.go` — populate bloom ramp
- `thermal/internal/theme/mono.go` — populate bloom ramp
- `thermal/internal/theme/theme_test.go` — assert all registered themes have a non-nil bloom ramp
- `thermal/internal/anim/profile.go` — new bloom motion fields
- `thermal/internal/anim/default.go` — wire new fields
- `thermal/internal/anim/calm.go` — wire new fields
- `thermal/internal/anim/intense.go` — wire new fields
- `thermal/internal/config/tuning.go` — bloom tunable constants
- `thermal/internal/widgets/headline.go` — construct bloom, forward Update/AnimTick, replace left-zone iconBg with bloom.BgAt, remove quip rendering
- `thermal/internal/widgets/headline_test.go` — update any golden captures affected by quip removal

---

## Task 1: Expose `CompositeHeat` scalar on AppState

**Files:**
- Modify: `thermal/internal/model/threat.go`
- Modify: `thermal/internal/model/state.go`
- Test: `thermal/internal/model/threat_test.go`

- [ ] **Step 1: Write the failing test**

Add to `thermal/internal/model/threat_test.go`:

```go
func TestCompositeHeatMatchesClassify(t *testing.T) {
	cases := []struct {
		name     string
		snap     collector.Snapshot
		spawn    float64
		wantBand ThreatLevel
	}{
		{"idle", collector.Snapshot{System: collector.SystemStats{CPUPercent: 5, MemUsedBytes: 1, MemTotalBytes: 100}}, 0, ThreatCool},
		{"warm", collector.Snapshot{System: collector.SystemStats{CPUPercent: 55, MemUsedBytes: 70, MemTotalBytes: 100}}, 0, ThreatWarm},
		{"hot", collector.Snapshot{System: collector.SystemStats{CPUPercent: 85, MemUsedBytes: 85, MemTotalBytes: 100}}, 0, ThreatHot},
		{"meltdown", collector.Snapshot{System: collector.SystemStats{CPUPercent: 95, MemUsedBytes: 95, MemTotalBytes: 100, SwapUsedBytes: config.C.Swap.CritBytes + 1}}, 5, ThreatMeltdown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scalar := CompositeHeatFor(&tc.snap, tc.spawn)
			if scalar < 0 || scalar > 1 {
				t.Fatalf("scalar %v out of [0,1]", scalar)
			}
			got := Classify(&tc.snap, tc.spawn)
			if got != tc.wantBand {
				t.Fatalf("Classify=%v want %v", got, tc.wantBand)
			}
		})
	}
}

func TestCompositeHeatClamp(t *testing.T) {
	snap := collector.Snapshot{System: collector.SystemStats{CPUPercent: -100, MemUsedBytes: 0, MemTotalBytes: 0}}
	v := CompositeHeatFor(&snap, 0)
	if v < 0 || v > 1 {
		t.Fatalf("scalar %v not clamped to [0,1]", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd thermal && go test ./internal/model/ -run TestCompositeHeat -v
```

Expected: FAIL with `undefined: CompositeHeatFor`

- [ ] **Step 3: Refactor Classify to expose scalar**

Replace the body of `thermal/internal/model/threat.go` with:

```go
package model

import (
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

type ThreatLevel int

const (
	ThreatCool ThreatLevel = iota
	ThreatWarm
	ThreatHot
	ThreatMeltdown
)

func (t ThreatLevel) String() string {
	switch t {
	case ThreatCool:
		return "COOL"
	case ThreatWarm:
		return "WARM"
	case ThreatHot:
		return "HOT"
	case ThreatMeltdown:
		return "MELTDOWN"
	default:
		return "UNKNOWN"
	}
}

// compositeHeatRaw returns the unbounded integer score used by Classify's
// bucketing. Kept private — callers wanting a continuous value use
// CompositeHeatFor (scalar) or Classify (bucketed).
func compositeHeatRaw(snap *collector.Snapshot, spawnRate float64) int {
	if snap == nil {
		return 0
	}
	mem := snap.System.MemPercent()
	cpu := snap.System.CPUPercent
	swapUsed := snap.System.SwapUsedBytes

	score := 0
	switch {
	case mem > float64(config.C.Memory.CritPct):
		score += 3
	case mem > float64(config.C.Memory.HotPct):
		score += 2
	case mem > float64(config.C.Memory.WarmPct):
		score += 1
	}
	switch {
	case cpu > float64(config.C.CPU.CritPct):
		score += 2
	case cpu > float64(config.C.CPU.WarmPct):
		score += 1
	}
	switch {
	case swapUsed > config.C.Swap.CritBytes:
		score += 3
	case swapUsed > config.C.Swap.HotBytes:
		score += 2
	case swapUsed > config.C.Swap.WarmBytes:
		score += 1
	}
	if spawnRate > config.C.Spawn.RateEscalation {
		score += 1
	}
	return score
}

// CompositeHeatFor returns the composite pressure scalar in [0.0, 1.0]
// derived from the same signals Classify uses. 0 = fully idle,
// 1 = score >= Meltdown threshold.
func CompositeHeatFor(snap *collector.Snapshot, spawnRate float64) float64 {
	score := compositeHeatRaw(snap, spawnRate)
	scalar := float64(score) / float64(config.C.Score.Meltdown)
	if scalar < 0 {
		return 0
	}
	if scalar > 1 {
		return 1
	}
	return scalar
}

// Classify determines threat level from a snapshot and spawn rate.
func Classify(snap *collector.Snapshot, spawnRate float64) ThreatLevel {
	score := compositeHeatRaw(snap, spawnRate)
	switch {
	case score >= config.C.Score.Meltdown:
		return ThreatMeltdown
	case score >= config.C.Score.Hot:
		return ThreatHot
	case score >= config.C.Score.Warm:
		return ThreatWarm
	default:
		return ThreatCool
	}
}
```

Add to `thermal/internal/model/state.go` (after the existing `LastDeaths` method):

```go
// CompositeHeat returns the weighted CPU/MEM/SWAP+spawn pressure scalar in
// [0.0, 1.0]. The HeatBloom widget consumes this as its heat target.
func (s *AppState) CompositeHeat() float64 {
	if s.Current == nil {
		return 0
	}
	return CompositeHeatFor(s.Current, s.SpawnRate)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd thermal && go test ./internal/model/ -run TestCompositeHeat -v && go test ./internal/model/ -run TestClassify -v
```

Expected: PASS for both (TestClassify must still pass — bucketing behavior is unchanged).

- [ ] **Step 5: Commit**

```bash
git add thermal/internal/model/threat.go thermal/internal/model/state.go thermal/internal/model/threat_test.go
git commit -m "Expose CompositeHeat scalar alongside Classify"
```

---

## Task 2: Add bloom tuning constants

**Files:**
- Modify: `thermal/internal/config/tuning.go`

- [ ] **Step 1: Add bloom constants**

Append to `thermal/internal/config/tuning.go`:

```go
// ── Heat bloom ───────────────────────────────────────────
// Breathe period at heat=0 (idle) and heat=1 (meltdown). Linear-interp in widget.
const (
	BloomBreatheSecCool = 4.5
	BloomBreatheSecHot  = 1.6
)

// Scale amplitude — the ± fractional ellipse radius expansion as breath swells.
const (
	BloomScaleAmpCool = 0.02
	BloomScaleAmpHot  = 0.08
)

// Opacity range — (min, max) of the alpha oscillation across a breath.
const (
	BloomOpacityMinCool = 0.72
	BloomOpacityMaxCool = 0.88
	BloomOpacityMinHot  = 0.85
	BloomOpacityMaxHot  = 1.00
)

// Spring physics governing heat + breathe-phase-rate transitions. Values
// chosen to feel "organic" — ~400ms settle, minor overshoot.
const (
	BloomSpringFreq    = 3.0
	BloomSpringDamping = 0.9
)

// Geometry — fractions of the left-zone width.
const (
	BloomAnchorX       = 0.12 // bloom center, as fraction of left-zone width
	BloomAnchorY       = 0.5  // vertical center (always mid-height)
	BloomRadiusX       = 0.55 // horizontal ellipse radius (fraction of left-zone width)
	BloomRadiusY       = 1.1  // vertical ellipse radius (fraction of left-zone height, oversize)
	BloomFalloffExp    = 1.8  // gaussian-ish falloff exponent
	BloomRightBoundary = 0.65 // bloom alpha must be 0 past this fraction of left-zone
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd thermal && go build ./...
```

Expected: exit 0, no errors.

- [ ] **Step 3: Commit**

```bash
git add thermal/internal/config/tuning.go
git commit -m "Add heat-bloom tuning constants"
```

---

## Task 3: Extend `anim.Profile` with bloom motion fields

**Files:**
- Modify: `thermal/internal/anim/profile.go`
- Modify: `thermal/internal/anim/default.go`
- Modify: `thermal/internal/anim/calm.go`
- Modify: `thermal/internal/anim/intense.go`

- [ ] **Step 1: Write the failing test**

Create `thermal/internal/anim/profile_test.go`:

```go
package anim

import "testing"

func TestAllProfilesHaveBloomFields(t *testing.T) {
	profiles := []*Profile{Default(), Calm(), Intense()}
	for _, p := range profiles {
		if p.BloomBreatheSecCool <= 0 || p.BloomBreatheSecHot <= 0 {
			t.Errorf("profile %q missing breathe seconds: cool=%v hot=%v", p.Name, p.BloomBreatheSecCool, p.BloomBreatheSecHot)
		}
		if p.BloomScaleAmpCool < 0 || p.BloomScaleAmpHot < 0 {
			t.Errorf("profile %q has negative scale amp", p.Name)
		}
		if p.BloomSpringFreq <= 0 || p.BloomSpringDamping <= 0 {
			t.Errorf("profile %q has non-positive spring params", p.Name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd thermal && go test ./internal/anim/ -run TestAllProfilesHaveBloomFields -v
```

Expected: FAIL with `p.BloomBreatheSecCool undefined`.

- [ ] **Step 3: Extend the Profile struct**

Append to `thermal/internal/anim/profile.go`:

```go
	// -- Heat bloom --
	BloomBreatheSecCool float64 // breathe period seconds at heat=0
	BloomBreatheSecHot  float64 // breathe period seconds at heat=1
	BloomScaleAmpCool   float64 // scale amplitude at heat=0
	BloomScaleAmpHot    float64 // scale amplitude at heat=1
	BloomOpacityMinCool float64
	BloomOpacityMaxCool float64
	BloomOpacityMinHot  float64
	BloomOpacityMaxHot  float64
	BloomSpringFreq     float64 // harmonica spring frequency for heat + params
	BloomSpringDamping  float64 // harmonica spring damping
```

- [ ] **Step 4: Wire the fields in Default, Calm, Intense**

Edit `thermal/internal/anim/default.go` — add inside the returned `&Profile{...}`:

```go
		BloomBreatheSecCool: config.BloomBreatheSecCool,
		BloomBreatheSecHot:  config.BloomBreatheSecHot,
		BloomScaleAmpCool:   config.BloomScaleAmpCool,
		BloomScaleAmpHot:    config.BloomScaleAmpHot,
		BloomOpacityMinCool: config.BloomOpacityMinCool,
		BloomOpacityMaxCool: config.BloomOpacityMaxCool,
		BloomOpacityMinHot:  config.BloomOpacityMinHot,
		BloomOpacityMaxHot:  config.BloomOpacityMaxHot,
		BloomSpringFreq:     config.BloomSpringFreq,
		BloomSpringDamping:  config.BloomSpringDamping,
```

Edit `thermal/internal/anim/calm.go` — inside the returned profile, set the same fields but multiply rates by ~0.75 (slower, softer):

```go
		BloomBreatheSecCool: config.BloomBreatheSecCool * 1.25,
		BloomBreatheSecHot:  config.BloomBreatheSecHot * 1.25,
		BloomScaleAmpCool:   config.BloomScaleAmpCool * 0.75,
		BloomScaleAmpHot:    config.BloomScaleAmpHot * 0.75,
		BloomOpacityMinCool: config.BloomOpacityMinCool,
		BloomOpacityMaxCool: config.BloomOpacityMaxCool * 0.9,
		BloomOpacityMinHot:  config.BloomOpacityMinHot,
		BloomOpacityMaxHot:  config.BloomOpacityMaxHot * 0.9,
		BloomSpringFreq:     config.BloomSpringFreq * 0.7,
		BloomSpringDamping:  config.BloomSpringDamping,
```

Edit `thermal/internal/anim/intense.go` — faster, sharper:

```go
		BloomBreatheSecCool: config.BloomBreatheSecCool * 0.75,
		BloomBreatheSecHot:  config.BloomBreatheSecHot * 0.6,
		BloomScaleAmpCool:   config.BloomScaleAmpCool * 1.3,
		BloomScaleAmpHot:    config.BloomScaleAmpHot * 1.3,
		BloomOpacityMinCool: config.BloomOpacityMinCool,
		BloomOpacityMaxCool: config.BloomOpacityMaxCool,
		BloomOpacityMinHot:  config.BloomOpacityMinHot,
		BloomOpacityMaxHot:  config.BloomOpacityMaxHot,
		BloomSpringFreq:     config.BloomSpringFreq * 1.4,
		BloomSpringDamping:  config.BloomSpringDamping,
```

- [ ] **Step 5: Run test to verify it passes**

```bash
cd thermal && go test ./internal/anim/ -v
```

Expected: PASS for `TestAllProfilesHaveBloomFields` and all pre-existing tests.

- [ ] **Step 6: Commit**

```bash
git add thermal/internal/anim/profile.go thermal/internal/anim/default.go thermal/internal/anim/calm.go thermal/internal/anim/intense.go thermal/internal/anim/profile_test.go
git commit -m "Add bloom motion fields to anim.Profile"
```

---

## Task 4: Add `BloomRamp` + LUT to `theme.Theme`

**Files:**
- Modify: `thermal/internal/theme/theme.go`
- Modify: `thermal/internal/theme/theme_test.go`

- [ ] **Step 1: Write the failing test**

Add to `thermal/internal/theme/theme_test.go`:

```go
func TestAllThemesProvideBloomRamp(t *testing.T) {
	for _, name := range Names() {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		if th.BloomRamp == ([4]BloomRampStop{}) {
			t.Errorf("theme %q has empty BloomRamp", name)
		}
		bg := th.BloomColor(0.0, 0.0)
		if bg == nil {
			t.Errorf("theme %q BloomColor(0,0) returned nil", name)
		}
		bg = th.BloomColor(1.0, 0.0)
		if bg == nil {
			t.Errorf("theme %q BloomColor(1,0) returned nil", name)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd thermal && go test ./internal/theme/ -run TestAllThemesProvideBloomRamp -v
```

Expected: FAIL with `BloomRamp undefined`.

- [ ] **Step 3: Add BloomRamp struct + Theme field + LUT + BloomColor method**

In `thermal/internal/theme/theme.go`, add near the other supporting types:

```go
// BloomRampStop defines one heat-level stop in the bloom color gradient.
// Core is the center color (peak alpha); Edge is the falloff color (alpha→0).
type BloomRampStop struct {
	Core colorful.Color
	Edge colorful.Color
}
```

Add to the `Theme` struct (after `CategoryGradient`):

```go
	// -- Heat bloom --
	// BloomRamp defines four heat stops (COOL/WARM/HOT/MELTDOWN), each a
	// Core→Edge gradient. The bloom widget blends between stops based on
	// a continuous heat scalar.
	BloomRamp [4]BloomRampStop

	// Pre-computed bloom color LUT. First dimension is heat (0..99),
	// second is radial distance (0..99, 0=center, 99=edge).
	bloomCoreLUT [100][100]colorful.Color
```

Add the `BloomColor` method:

```go
// BloomColor returns the blended bloom color for a given heat scalar
// in [0,1] and radial distance in [0,1] (0=core, 1=edge). Lookup is
// O(1) via pre-computed LUT.
func (t *Theme) BloomColor(heat, radial float64) colorful.Color {
	hi := blendIndex(heat)
	ri := blendIndex(radial)
	return t.bloomCoreLUT[hi][ri]
}
```

Extend `Init()` — add at the end, before `}`:

```go
	// Heat-bloom LUT: 100 heat rows × 100 radial cols.
	for hi := 0; hi < 100; hi++ {
		heat := float64(hi) / 99.0
		// Pick the two adjacent stops in BloomRamp (4 stops → 3 segments).
		pos := heat * 3.0
		seg := int(pos)
		if seg >= 3 {
			seg = 2
		}
		segRatio := pos - float64(seg)
		coreA := t.BloomRamp[seg].Core
		coreB := t.BloomRamp[seg+1].Core
		edgeA := t.BloomRamp[seg].Edge
		edgeB := t.BloomRamp[seg+1].Edge
		core := coreA.BlendHcl(coreB, segRatio).Clamped()
		edge := edgeA.BlendHcl(edgeB, segRatio).Clamped()
		for ri := 0; ri < 100; ri++ {
			radial := float64(ri) / 99.0
			t.bloomCoreLUT[hi][ri] = core.BlendHcl(edge, radial).Clamped()
		}
	}
```

- [ ] **Step 4: Verify compile (test still fails because themes don't populate ramp)**

```bash
cd thermal && go build ./...
```

Expected: exit 0. Test `TestAllThemesProvideBloomRamp` will still fail until Tasks 5-8 fill ramp values.

- [ ] **Step 5: Commit**

```bash
git add thermal/internal/theme/theme.go thermal/internal/theme/theme_test.go
git commit -m "Add BloomRamp + LUT infrastructure to Theme"
```

---

## Task 5: Populate Frappé bloom ramp

**Files:**
- Modify: `thermal/internal/theme/frappe.go`

- [ ] **Step 1: Add ramp values**

In `thermal/internal/theme/frappe.go`, inside the `&Theme{...}` constructor, add (Frappé palette hex values — Catppuccin reference):

```go
		BloomRamp: [4]BloomRampStop{
			// COOL — sky / blue core, lavender-ish edge
			{Core: mustHex("#8caaee"), Edge: mustHex("#babbf1")},
			// WARM — yellow core, peach edge
			{Core: mustHex("#e5c890"), Edge: mustHex("#ef9f76")},
			// HOT — peach core, red edge
			{Core: mustHex("#ef9f76"), Edge: mustHex("#e78284")},
			// MELTDOWN — saturated red core, maroon edge
			{Core: mustHex("#e78284"), Edge: mustHex("#ea999c")},
		},
```

- [ ] **Step 2: Verify Frappé passes the bloom test (others still fail)**

```bash
cd thermal && go test ./internal/theme/ -run TestAllThemesProvideBloomRamp -v
```

Expected: still fails overall (other themes), but inspect output — Frappé should not be in the failure list for the non-zero-ramp check.

- [ ] **Step 3: Commit**

```bash
git add thermal/internal/theme/frappe.go
git commit -m "Populate Frappé bloom ramp"
```

---

## Task 6: Populate Classic, Iron, Mono bloom ramps

**Files:**
- Modify: `thermal/internal/theme/classic.go`
- Modify: `thermal/internal/theme/iron.go`
- Modify: `thermal/internal/theme/mono.go`

- [ ] **Step 1: Classic ramp (traffic-light aesthetic)**

In `thermal/internal/theme/classic.go`, add inside the `&Theme{...}` constructor:

```go
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#3b82f6"), Edge: mustHex("#1e3a8a")},  // COOL — blue
			{Core: mustHex("#eab308"), Edge: mustHex("#92400e")},  // WARM — yellow/amber
			{Core: mustHex("#f97316"), Edge: mustHex("#b45309")},  // HOT — orange
			{Core: mustHex("#ef4444"), Edge: mustHex("#7f1d1d")},  // MELTDOWN — red
		},
```

- [ ] **Step 2: Iron ramp (FLIR blackbody aesthetic)**

In `thermal/internal/theme/iron.go`:

```go
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#4c1d95"), Edge: mustHex("#1e1b4b")},  // COOL — deep violet core, near-black edge
			{Core: mustHex("#c026d3"), Edge: mustHex("#6b21a8")},  // WARM — magenta
			{Core: mustHex("#f97316"), Edge: mustHex("#9a3412")},  // HOT — amber
			{Core: mustHex("#fcd34d"), Edge: mustHex("#d97706")},  // MELTDOWN — near-white-hot yellow
		},
```

- [ ] **Step 3: Mono ramp (single-hue, brightness-only)**

In `thermal/internal/theme/mono.go`:

```go
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#404040"), Edge: mustHex("#1a1a1a")},  // COOL — dim
			{Core: mustHex("#a16207"), Edge: mustHex("#451a03")},  // WARM — faint amber
			{Core: mustHex("#d97706"), Edge: mustHex("#7c2d12")},  // HOT — amber
			{Core: mustHex("#fbbf24"), Edge: mustHex("#b45309")},  // MELTDOWN — bright amber
		},
```

- [ ] **Step 4: Run test to verify all themes pass**

```bash
cd thermal && go test ./internal/theme/ -v
```

Expected: PASS on `TestAllThemesProvideBloomRamp`.

- [ ] **Step 5: Commit**

```bash
git add thermal/internal/theme/classic.go thermal/internal/theme/iron.go thermal/internal/theme/mono.go
git commit -m "Populate Classic/Iron/Mono bloom ramps"
```

---

## Task 7: HeatBloom widget skeleton

**Files:**
- Create: `thermal/internal/widgets/heatbloom.go`
- Create: `thermal/internal/widgets/heatbloom_test.go`

- [ ] **Step 1: Write the failing test**

Create `thermal/internal/widgets/heatbloom_test.go`:

```go
package widgets

import (
	"image/color"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func newTestBloom(t *testing.T) *HeatBloom {
	t.Helper()
	th, err := theme.Get("frappe")
	if err != nil {
		t.Fatalf("theme: %v", err)
	}
	return NewHeatBloom(th, anim.Default())
}

func TestHeatBloomConstruct(t *testing.T) {
	b := newTestBloom(t)
	if b == nil {
		t.Fatal("NewHeatBloom returned nil")
	}
}

func TestHeatBloomSetSizeZero(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(0, 2)
	// Must not panic on BgAt with zero width.
	got := b.BgAt(0, 0, nil)
	if got == nil {
		t.Fatal("BgAt returned nil at zero size")
	}
}

func TestHeatBloomBgAtFallback(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	// Past the right-boundary (col >= 0.65*40 = 26), bg must equal the fallback.
	got := b.BgAt(35, 0, fallback)
	if got != fallback {
		t.Errorf("BgAt past boundary = %v, want fallback %v", got, fallback)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloom -v
```

Expected: FAIL with `undefined: HeatBloom`.

- [ ] **Step 3: Create the widget skeleton**

Create `thermal/internal/widgets/heatbloom.go`:

```go
// Package widgets — HeatBloom renders a left-anchored thermographic
// accent layer behind the headline strip. It exposes per-cell background
// colors (BgAt) that headline.go consumes instead of its fixed iconBg
// when rendering the left zone.
package widgets

import (
	"image/color"
	"math"

	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// HeatBloom renders a gaussian-falloff thermal accent over the headline's
// left zone. Motion: breath oscillation + heat-driven spring tracking.
type HeatBloom struct {
	theme   *theme.Theme
	profile *anim.Profile

	// Dimensions (set by SetSize).
	width  int
	height int

	// Heat target + current spring-interpolated value, both in [0,1].
	heatTarget float64
	heat       float64
	heatVel    float64
	spring     harmonica.FixedDeltaSpring

	// Breath oscillator (radians).
	breathePhase float64
}

// NewHeatBloom constructs the widget with the given theme and profile.
func NewHeatBloom(th *theme.Theme, ap *anim.Profile) *HeatBloom {
	return &HeatBloom{
		theme:   th,
		profile: ap,
		spring:  harmonica.NewSpring(1.0/float64(config.AnimFPS), ap.BloomSpringFreq, ap.BloomSpringDamping),
	}
}

// SetSize sets the left-zone dimensions the bloom renders into.
func (b *HeatBloom) SetSize(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	b.width = w
	b.height = h
}

// Update reads the latest heat scalar from AppState. Safe with nil state.
func (b *HeatBloom) Update(s *model.AppState) {
	if s == nil {
		b.heatTarget = 0
		return
	}
	b.heatTarget = s.CompositeHeat()
}

// AnimTick advances the spring toward the heat target and steps the
// breathe oscillator. Called at AnimFPS cadence by Headline.AnimTick.
func (b *HeatBloom) AnimTick() {
	b.heat, b.heatVel = b.spring.Update(b.heat, b.heatVel, b.heatTarget)

	// Breathe period linearly interpolates from cool (slow) to hot (fast).
	periodSec := b.profile.BloomBreatheSecCool + (b.profile.BloomBreatheSecHot-b.profile.BloomBreatheSecCool)*b.heat
	if periodSec <= 0 {
		periodSec = b.profile.BloomBreatheSecCool
	}
	step := 2 * math.Pi / (periodSec * float64(config.AnimFPS))
	b.breathePhase += step
	if b.breathePhase > 2*math.Pi {
		b.breathePhase -= 2 * math.Pi
	}
}

// BgAt returns the bloom's background color at the given cell. Beyond
// the right-boundary (col >= BloomRightBoundary*width) returns fallback
// unchanged. Fallback is also returned for out-of-bounds cells.
func (b *HeatBloom) BgAt(col, row int, fallback color.Color) color.Color {
	if b.width == 0 || b.height == 0 || col < 0 || row < 0 || row >= b.height {
		return fallback
	}
	if float64(col) >= config.BloomRightBoundary*float64(b.width) {
		return fallback
	}
	// TODO(next task): compute alpha via gaussian + return blended color.
	// Skeleton returns fallback so tests pass until geometry lands.
	return fallback
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloom -v
```

Expected: PASS on all three skeleton tests.

- [ ] **Step 5: Commit**

```bash
git add thermal/internal/widgets/heatbloom.go thermal/internal/widgets/heatbloom_test.go
git commit -m "Add HeatBloom widget skeleton with BgAt right-boundary guard"
```

---

## Task 8: Gaussian geometry + alpha falloff

**Files:**
- Modify: `thermal/internal/widgets/heatbloom.go`
- Modify: `thermal/internal/widgets/heatbloom_test.go`

- [ ] **Step 1: Write the failing test**

Add to `thermal/internal/widgets/heatbloom_test.go`:

```go
func TestHeatBloomAlphaFalloff(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heatTarget = 1.0
	b.heat = 1.0
	b.breathePhase = 0

	// Center column (col ≈ BloomAnchorX * width = 0.12*40 ≈ 5), center row → high alpha.
	centerA := b.alphaAt(5, 1)
	// Far-left edge (col=0) → low alpha.
	edgeA := b.alphaAt(0, 1)
	// Just-inside right boundary (col=25) → low alpha.
	nearBoundary := b.alphaAt(25, 1)

	if centerA <= edgeA {
		t.Errorf("center alpha %v should exceed edge alpha %v", centerA, edgeA)
	}
	if centerA <= nearBoundary {
		t.Errorf("center alpha %v should exceed near-boundary alpha %v", centerA, nearBoundary)
	}
	if centerA < 0.5 {
		t.Errorf("center alpha %v should be >= 0.5 at peak heat", centerA)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloomAlphaFalloff -v
```

Expected: FAIL with `b.alphaAt undefined`.

- [ ] **Step 3: Implement alphaAt + update BgAt**

Add to `thermal/internal/widgets/heatbloom.go`:

```go
// alphaAt returns the bloom's alpha [0,1] at (col, row) factoring in
// gaussian falloff, breathe scale/opacity modulation, and heat.
func (b *HeatBloom) alphaAt(col, row int) float64 {
	if b.width == 0 || b.height == 0 {
		return 0
	}
	w := float64(b.width)
	h := float64(b.height)

	// Breath modulation: 0 at phase=0 rising through 1 at phase=pi.
	breath := (math.Sin(b.breathePhase) + 1) * 0.5

	// Scale amplitude interpolates cool→hot.
	ampCool := b.profile.BloomScaleAmpCool
	ampHot := b.profile.BloomScaleAmpHot
	amp := ampCool + (ampHot-ampCool)*b.heat
	scale := 1.0 + amp*(2*breath-1) // oscillates in [1-amp, 1+amp]

	// Opacity interpolates cool→hot, oscillates within the breath band.
	oMinCool := b.profile.BloomOpacityMinCool
	oMaxCool := b.profile.BloomOpacityMaxCool
	oMinHot := b.profile.BloomOpacityMinHot
	oMaxHot := b.profile.BloomOpacityMaxHot
	oMin := oMinCool + (oMinHot-oMinCool)*b.heat
	oMax := oMaxCool + (oMaxHot-oMaxCool)*b.heat
	opacityEnvelope := oMin + (oMax-oMin)*breath

	// Normalized cell center in [0,1] space.
	nx := (float64(col) + 0.5) / w
	ny := (float64(row) + 0.5) / h

	// Distance from anchor, normalized by ellipse radii (scaled).
	dx := (nx - config.BloomAnchorX) / (config.BloomRadiusX * scale)
	dy := (ny - config.BloomAnchorY) / (config.BloomRadiusY * scale)
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist >= 1.0 {
		return 0
	}
	// Gaussian-ish falloff: (1 - dist^exp).
	return opacityEnvelope * math.Pow(1-dist, config.BloomFalloffExp)
}

// radialAt returns the radial distance [0,1] from the bloom anchor,
// used to look up the color ramp edge position. Independent of breath
// scale so color doesn't jitter — only alpha does.
func (b *HeatBloom) radialAt(col, row int) float64 {
	if b.width == 0 || b.height == 0 {
		return 1
	}
	nx := (float64(col) + 0.5) / float64(b.width)
	ny := (float64(row) + 0.5) / float64(b.height)
	dx := (nx - config.BloomAnchorX) / config.BloomRadiusX
	dy := (ny - config.BloomAnchorY) / config.BloomRadiusY
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > 1 {
		return 1
	}
	return dist
}
```

Replace the body of `BgAt` (remove the TODO skeleton):

```go
func (b *HeatBloom) BgAt(col, row int, fallback color.Color) color.Color {
	if b.width == 0 || b.height == 0 || col < 0 || row < 0 || row >= b.height {
		return fallback
	}
	if float64(col) >= config.BloomRightBoundary*float64(b.width) {
		return fallback
	}
	alpha := b.alphaAt(col, row)
	if alpha <= 0 {
		return fallback
	}
	bloomC := b.theme.BloomColor(b.heat, b.radialAt(col, row))
	return blendColors(bloomC, fallback, alpha)
}

// blendColors returns fg composited over bg with the given alpha.
// All inputs/outputs in color.Color; internally converts through RGB.
func blendColors(fg colorful.Color, bg color.Color, alpha float64) color.Color {
	if alpha >= 1 {
		r, g, bl := fg.RGB255()
		return color.RGBA{R: r, G: g, B: bl, A: 255}
	}
	if alpha <= 0 {
		return bg
	}
	br, bgr, bb, _ := bg.RGBA()
	fr, fg8, fb := fg.RGB255()
	outR := uint8(float64(fr)*alpha + float64(br>>8)*(1-alpha))
	outG := uint8(float64(fg8)*alpha + float64(bgr>>8)*(1-alpha))
	outB := uint8(float64(fb)*alpha + float64(bb>>8)*(1-alpha))
	return color.RGBA{R: outR, G: outG, B: outB, A: 255}
}
```

Add the `colorful` import at the top of the file (if not already present):

```go
import (
	// ... existing imports ...
	colorful "github.com/lucasb-eyer/go-colorful"
)
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloom -v
```

Expected: PASS on all HeatBloom tests including new alpha falloff test.

- [ ] **Step 5: Commit**

```bash
git add thermal/internal/widgets/heatbloom.go thermal/internal/widgets/heatbloom_test.go
git commit -m "Implement HeatBloom gaussian falloff + color compositing"
```

---

## Task 9: Golden-capture tests for bloom rendering

**Files:**
- Modify: `thermal/internal/widgets/heatbloom_test.go`
- Create: `thermal/internal/widgets/testdata/heatbloom_heat_0.golden`
- Create: `thermal/internal/widgets/testdata/heatbloom_heat_half.golden`
- Create: `thermal/internal/widgets/testdata/heatbloom_heat_1.golden`

- [ ] **Step 1: Write the golden test infrastructure**

Add to `thermal/internal/widgets/heatbloom_test.go`:

```go
import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	// existing imports
)

var updateGoldenBloom = flag.Bool("update-bloom-golden", false, "update heatbloom golden files")

func renderBloomRow(b *HeatBloom, row int, fallback color.Color) string {
	var sb strings.Builder
	for col := 0; col < b.width; col++ {
		c := b.BgAt(col, row, fallback)
		r, g, bl, _ := c.RGBA()
		fmt.Fprintf(&sb, "[%03d,%03d,%03d]", r>>8, g>>8, bl>>8)
	}
	return sb.String()
}

func bloomGoldenCheck(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGoldenBloom {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update-bloom-golden to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch: %s\nWANT:\n%s\nGOT:\n%s", path, string(want), got)
	}
}

func TestHeatBloomGolden_Heat0(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 0
	b.breathePhase = 0
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255} // Frappé mantle
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_0.golden", got)
}

func TestHeatBloomGolden_HeatHalf(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 0.5
	b.breathePhase = math.Pi / 2
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_half.golden", got)
}

func TestHeatBloomGolden_Heat1(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 1.0
	b.breathePhase = 0
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_1.golden", got)
}
```

Add `"math"` to the test file imports if not already there.

- [ ] **Step 2: Run test to verify it fails (missing golden files)**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloomGolden -v
```

Expected: FAIL with "no such file or directory" for each golden.

- [ ] **Step 3: Generate golden files**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloomGolden -update-bloom-golden
```

Expected: PASS. Three files created in `internal/widgets/testdata/`.

- [ ] **Step 4: Verify goldens are non-trivial**

Read each generated golden file and confirm the color strings vary across columns (not all identical). If all columns are the fallback color, the alpha falloff isn't engaging — investigate Task 8.

```bash
cd thermal && head -c 400 internal/widgets/testdata/heatbloom_heat_1.golden
```

Expected: output contains varying `[R,G,B]` triples, with hotter colors (red-ish) near left columns (col 5-10) and the fallback `[041,044,060]` appearing past col 25.

- [ ] **Step 5: Re-run without update flag to confirm match**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloomGolden -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add thermal/internal/widgets/heatbloom_test.go thermal/internal/widgets/testdata/heatbloom_heat_0.golden thermal/internal/widgets/testdata/heatbloom_heat_half.golden thermal/internal/widgets/testdata/heatbloom_heat_1.golden
git commit -m "Add HeatBloom golden-capture tests at three heat levels"
```

---

## Task 10: Right-boundary bleed guard test

**Files:**
- Modify: `thermal/internal/widgets/heatbloom_test.go`

- [ ] **Step 1: Add explicit bleed guard test**

Append to `thermal/internal/widgets/heatbloom_test.go`:

```go
func TestHeatBloomNoRightBleed(t *testing.T) {
	b := newTestBloom(t)
	widths := []int{40, 80, 120, 200}
	for _, w := range widths {
		b.SetSize(w, 2)
		for _, heat := range []float64{0, 0.5, 1.0} {
			b.heat = heat
			for _, phase := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
				b.breathePhase = phase
				boundary := int(config.BloomRightBoundary * float64(w))
				for col := boundary; col < w; col++ {
					for row := 0; row < 2; row++ {
						fallback := color.RGBA{R: 99, G: 99, B: 99, A: 255}
						got := b.BgAt(col, row, fallback)
						if got != fallback {
							t.Errorf("bleed at w=%d heat=%v phase=%v col=%d row=%d: got %v want fallback", w, heat, phase, col, row, got)
						}
					}
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeatBloomNoRightBleed -v
```

Expected: PASS (the guard is already in `BgAt` from Task 7/8).

- [ ] **Step 3: Commit**

```bash
git add thermal/internal/widgets/heatbloom_test.go
git commit -m "Add exhaustive no-right-bleed test across widths/heats/phases"
```

---

## Task 11: Remove quip rendering from headline

**Files:**
- Modify: `thermal/internal/widgets/headline.go`

- [ ] **Step 1: Inspect existing headline golden tests**

```bash
cd thermal && ls internal/widgets/testdata/ | grep -i headline
```

If golden files exist for the headline, they'll need regenerating after quip removal. Capture the list.

- [ ] **Step 2: Edit headline.go — remove quip logic from ViewLines**

In `thermal/internal/widgets/headline.go`, locate the online `ViewLines` branch (starting around line 245). Remove quip-related lines. Specifically:

Delete:
```go
	quip := h.state.StableQuip()
```

Delete:
```go
	maxQuip := leftWidth - 1
	if maxQuip < 0 {
		maxQuip = 0
	}
	if utf8.RuneCountInString(quip) > maxQuip {
		quip = truncRunes(quip, maxQuip)
	}
	quipWidth := utf8.RuneCountInString(quip)

	quipStyle := lipgloss.NewStyle().Foreground(fg).Background(iconBg)
	leftTop := ""
	if leftWidth > 0 {
		padAfterQuip := leftWidth - 1 - quipWidth
		if padAfterQuip < 0 {
			padAfterQuip = 0
		}
		leftTop = quipStyle.Render(" "+quip) + bgPad(iconBg, padAfterQuip)
	}
```

Replace with:
```go
	// Top-row-left is intentionally blank (see docs/backlog/headline-top-row-content.md).
	// The bloom renders behind this zone; future content additions will compose over it.
	leftTop := ""
	if leftWidth > 0 {
		leftTop = bgPad(iconBg, leftWidth)
	}
```

Also remove the unused `fg` variable assignment if it's only referenced by the deleted `quipStyle` line. Grep after editing:

```bash
cd thermal && grep -n " fg " internal/widgets/headline.go
```

If `fg` is unused, delete its declaration (`fg := h.theme.OverallGradient[level].Fg`).

Similarly check `utf8` — remove from imports if no longer referenced. Run `goimports` or `go build` to surface unused imports.

- [ ] **Step 3: Build + run all tests**

```bash
cd thermal && go build ./... && go test ./...
```

Expected: build passes. If any headline golden tests fail, regenerate them (next step).

- [ ] **Step 4: Regenerate headline golden tests if present**

If `TestHeadline*` golden tests fail due to quip removal:

```bash
cd thermal && go test ./internal/widgets/ -run TestHeadline -update-golden 2>&1 | head -20
```

(Use whatever update flag the existing test harness uses — check `internal/widgets/golden_test.go` for the flag name if different.)

Inspect diffs manually to confirm ONLY the quip area changed.

- [ ] **Step 5: Run all tests clean**

```bash
cd thermal && go test ./...
```

Expected: PASS on all.

- [ ] **Step 6: Commit**

```bash
git add thermal/internal/widgets/headline.go thermal/internal/widgets/testdata/
git commit -m "Remove quip rendering from headline top row"
```

---

## Task 12: Integrate HeatBloom into headline

**Files:**
- Modify: `thermal/internal/widgets/headline.go`

- [ ] **Step 1: Add bloom field + constructor wiring**

In `thermal/internal/widgets/headline.go`, add to the `Headline` struct:

```go
	bloom *HeatBloom
```

Edit `NewHeadline`:

```go
func NewHeadline(th *theme.Theme, ap *anim.Profile) *Headline {
	return &Headline{
		agents: NewBreatheDots(th, ap),
		temp:   NewSegmentReadout(th, ap),
		bloom:  NewHeatBloom(th, ap),
		theme:  th,
	}
}
```

- [ ] **Step 2: Forward SetSize, Update, AnimTick**

In `SetSize`:

```go
func (h *Headline) SetSize(w, height int) {
	h.width = w
	// Bloom is confined to the left zone; its width is computed inside
	// ViewLines after right-cluster measurements. We pass a placeholder
	// here; the actual resize happens per-frame.
}
```

In `Update`, before the final line:

```go
	h.bloom.Update(state)
```

In `AnimTick`, before the final closing brace:

```go
	h.bloom.AnimTick()
```

- [ ] **Step 3: Swap iconBg → bloom.BgAt in left-zone rendering**

Find the spot in `ViewLines` where `leftTop` and `botLeft` are constructed. Replace the single-value `iconBg` usage with per-cell bloom lookup via a new helper:

After `leftCombined := h.width - rightVis - headlineRightMargin` (and the clamping), add:

```go
	// Configure bloom for this frame's left-zone dimensions.
	h.bloom.SetSize(leftCombined, 2)
```

Add a helper method on Headline for bg-pad with bloom:

```go
// bloomedBgPad renders n cells starting at startCol on the given row,
// using the bloom's BgAt for each column (falling back to iconBg past
// the bloom's right-boundary).
func (h *Headline) bloomedBgPad(iconBg color.Color, startCol, n, row int) string {
	if n <= 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		c := h.bloom.BgAt(startCol+i, row, iconBg)
		sb.WriteString(lipgloss.NewStyle().Background(c).Render(" "))
	}
	return sb.String()
}
```

Replace the top-left `leftTop` construction (from Task 11) with:

```go
	leftTop := ""
	if leftWidth > 0 {
		leftTop = h.bloomedBgPad(iconBg, 0, leftWidth, 0)
	}
```

Replace the bottom-left ghost-pad construction. Find:

```go
	botLeft := bgPad(iconBg, ghostPadLeft) + ghostStr
```

Replace with:

```go
	botLeft := h.bloomedBgPad(iconBg, 0, ghostPadLeft, 1) + ghostStr
```

Also the runtime cells on the top row render WITH `iconBg` via `renderCatCell`. For v1, leave these alone — the bloom shows through the space AROUND the runtime cells but the cells themselves keep their categorical colors. Do not modify `renderCatCell`.

- [ ] **Step 4: Build + run tests**

```bash
cd thermal && go build ./... && go test ./...
```

Expected: build passes. Headline golden tests likely need re-generation because per-cell bg colors now differ.

- [ ] **Step 5: Regenerate affected goldens**

```bash
cd thermal && go test ./internal/widgets/ -run TestHeadline -update-golden
```

Inspect the diff carefully. The changes should be confined to the left zone's background ANSI codes, not text content or the right cluster.

- [ ] **Step 6: Run full test suite**

```bash
cd thermal && go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add thermal/internal/widgets/headline.go thermal/internal/widgets/testdata/
git commit -m "Integrate HeatBloom as headline left-zone background"
```

---

## Task 13: Build binary and manual QA

**Files:**
- Build artifact: `bin/thermo`

- [ ] **Step 1: Build the binary**

```bash
cd thermal && go build -o ../bin/thermo ./cmd/thermal/
```

Expected: exit 0, binary present at `bin/thermo`.

- [ ] **Step 2: Launch demo mode**

```bash
./bin/thermo --demo
```

- [ ] **Step 3: Visual verification checklist**

With the demo running, confirm each:

- [ ] Bloom visible in headline left zone at idle (cool-colored, slow breathing).
- [ ] Bloom fades out BEFORE the right cluster begins (no color bleed into SESS/BLD/agents/LCD).
- [ ] As the demo ramps load, bloom color shifts through the ramp (cool→warm→hot→meltdown).
- [ ] Bloom breathing speeds up at higher heat (faster oscillation visible).
- [ ] Bloom breathing amplitude grows at higher heat (wider brightness swings).
- [ ] Transitions between heat levels are smooth (no snaps, no jerky jumps).
- [ ] Ghost ribbon on bottom row still renders correctly over the bloom.
- [ ] Sparklines below the headline are visually unchanged.
- [ ] No tearing, flicker, or render artifacts at 150ms tick cadence.
- [ ] Test with `--theme iron`, `--theme mono`, `--theme classic` — each produces a coherent bloom.
- [ ] Test with `--animation calm` and `--animation intense` — motion visibly differs.

Press `q` to exit.

- [ ] **Step 4: Launch live mode briefly**

```bash
./bin/thermo
```

Confirm live system data drives the bloom plausibly (bloom intensity correlates with actual CPU/MEM pressure). Press `q` to exit.

- [ ] **Step 5: If all checks pass, commit binary rebuild if artifacts are tracked**

```bash
git status bin/
```

If `bin/thermo` is tracked, commit. If gitignored, skip.

---

## Task 14: Final full-suite verification

**Files:** none (verification only)

- [ ] **Step 1: Run every test**

```bash
cd thermal && go test ./... && cd .. && bats tests/
```

Expected: both pass.

- [ ] **Step 2: Run go vet**

```bash
cd thermal && go vet ./...
```

Expected: no output, exit 0.

- [ ] **Step 3: Run gofmt check**

```bash
cd thermal && gofmt -l ./...
```

Expected: no output (all files formatted).

If any of 1–3 fail, diagnose and fix before declaring complete.

---

## Self-Review Notes

### Spec coverage

- Goal "soft, left-flowing thermographic bloom" — Tasks 7-10 implement the widget.
- Architecture: new `widgets/heatbloom.go`, composed with `theme.Theme.BloomColor`, `anim.Profile` motion fields, `model.CompositeHeat` — Tasks 1-8 + 12.
- Visual spec color ramp via palette-aware LUT — Tasks 4-6.
- Visual spec motion (breathe period, scale amplitude, opacity range, spring easing) — Tasks 2, 3, 8.
- Right-boundary guard (alpha=0 past 0.65×width) — Tasks 7 (initial), 10 (exhaustive).
- Golden tests at heat=0 / 0.5 / 1.0 — Task 9.
- `CompositeHeat` refactor preserving `Classify` bucketing — Task 1.
- Quip removal — Task 11.
- Headline integration keeping right cluster unchanged — Task 12.

### Consistency

Method names used consistently: `CompositeHeat()` on `AppState`, `CompositeHeatFor(...)` on package, `BloomColor(heat, radial)` on Theme, `BgAt(col, row, fallback)` on HeatBloom. Spring field names (`BloomSpring*`) match across profile/config/widget.

### Worktree note

User explicitly requested this be implemented in a worktree. Before the executor begins Task 1, invoke `superpowers:using-git-worktrees` to set up `worktree-thermographic-accent` off the current main HEAD.
