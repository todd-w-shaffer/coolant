// Package config holds named constants for timing, thresholds, EMA
// smoothing, animation parameters, and category heat levels.
package config

import "time"

// ── Collector timing ───────────────────────────────────────

const (
	FastInterval    = 150 * time.Millisecond // CPU/MEM/procs sample rate
	SlowInterval    = 1 * time.Second        // network reachability check
	NetDialTimeout  = 2 * time.Second        // TCP dial to api.anthropic.com
	NetCheckTimeout = 3 * time.Second        // context deadline for network check
	SysExecTimeout  = 2 * time.Second        // per-subprocess timeout (sysctl, vm_stat)
	SysInitTimeout  = 5 * time.Second        // one-shot static sysctl init
	ProcTimeout     = 3 * time.Second        // ps process tree collection
	CollectTimeout  = 5 * time.Second        // overall fast-loop context deadline
	EventInterval   = 500 * time.Millisecond // JSONL event log poll rate
)

// DefaultPageSize is the fallback macOS page size when hw.pagesize is unavailable.
const DefaultPageSize int64 = 16384

// ── Agent staleness ───────────────────────────────────────

// AgentStaleThreshold is how long after start an agent is considered
// "likely orphaned" if no stop event has arrived. Team agents that
// terminate via shutdown protocol don't fire SubagentStop hooks,
// so their dots dim after this duration.
const AgentStaleThreshold = 3 * time.Minute

// ── Animation ──────────────────────────────────────────────

const (
	AnimFPS          = 30                    // spring physics + sparkline scroll rate
	AnimInterval     = time.Second / AnimFPS // ~33ms per frame
	PeakDecayRate    = 0.982                 // per-frame decay (~1.3s half-life at 30fps)
	MaxRenderHistory = 600                   // ~20s at 30fps
)

// Spring physics parameters for gauge easing.
const (
	SpringFreq    = 5.0 // harmonica spring frequency
	SpringDamping = 1.0 // critically damped
)

// Spring physics for the LCD temperature readout. Kept separate from
// gauge spring so the focal readout can be tuned independently.
const (
	LCDSpringFreq    = 5.0  // harmonica frequency
	LCDSpringDamping = 1.0  // critically damped — no overshoot
	LCDBoundaryHyst  = 0.15 // dead-zone radius around each integer step
)

// Agent icon breathing animation.
const (
	BreatheMinBright = 0.25 // dimmest point of breathing cycle
	BreatheMaxBright = 1.0  // brightest point
	BreathePhaseStep = 0.14 // radians per AnimTick (~1.5s cycle at 30fps: 2π/45)
	BreatheFadeEps   = 0.01 // spring position below which a dying icon is removed
	BreatheStaleRate = 0.3  // phase advance multiplier for stale (orphaned) dots
	BreatheStaleDim  = 0.35 // brightness multiplier for stale dots
)

// KITT scanner (stale ghost dots / highscore completed dots).
const (
	KITTSweepRate    = 0.025 // sweep position advance per AnimTick
	KITTAmbient      = 0.15  // floor brightness at sweep edges
	KITTPeak         = 0.85  // peak contribution above ambient
	KITTSigmaSq      = 0.8   // gaussian width (sigma²) — tighter = sharper spotlight
	KITTSingleBright = 0.8   // brightness multiplier when only one dot (no sweep)
	KITTMaxDots      = 12    // cap rendered completed/ghost dots to prevent overflow
)

// Tidal wave (active agent dots).
const (
	TidalPhaseStep    = 0.015 // phase advance per AnimTick (~14s per full wave)
	TidalWaveMix      = 0.85  // tidal wave weight in brightness blend
	TidalBreathMix    = 0.15  // individual breath weight in brightness blend
	TidalBrightFloor  = 0.5   // minimum brightness for active dots
	TidalPhaseSpread  = 1.5   // radians between adjacent dots (wider = clearer wave direction)
	GlyphFilledThresh = 0.66  // wave value above which glyph shows ⬢ (filled)
	GlyphMidThresh    = 0.33  // wave value above which glyph shows ⏣ (benzene)
)

// ── History / buffer sizes ─────────────────────────────────

const (
	MaxHistory      = 600 // ~90s at 150ms — fills sparklines on wide terminals
	MaxAlerts       = 100 // scrolling alert log cap
	RateWindowSize  = 10  // rolling window for spawn/death rate
	MaxAgentRecords = 500 // ring buffer cap for completed agent records
)

// BurstGapThreshold is the minimum gap between agent starts that separates
// two burst groups. Agents starting within this window share a BurstID.
const BurstGapThreshold = 3 * time.Second

// MaxGateEvents is the ring buffer cap for gate.cap + gate.suppress events.
const MaxGateEvents = 64

// ── Threat classification ──────────────────────────────────

// Memory pressure thresholds (percent of total RAM).
const (
	MemWarmPct = 65
	MemHotPct  = 80
	MemCritPct = 90
)

// CPU pressure thresholds (percent).
const (
	CPUWarmPct = 75
	CPUCritPct = 90
)

// Swap thresholds (bytes). macOS proactively swaps; only escalate when large.
// Typed int64 to prevent accidental use in int contexts where they'd overflow on 32-bit.
const (
	SwapWarmBytes int64 = 2 << 30  // 2GB — baseline noise
	SwapHotBytes  int64 = 8 << 30  // 8GB — real pressure
	SwapCritBytes int64 = 20 << 30 // 20GB — meltdown territory
)

// SpawnBurstThreshold triggers an alert when this many procs appear in one tick.
const SpawnBurstThreshold = 8

// SpawnRateEscalation is the EMA spawn rate above which threat score increments.
const SpawnRateEscalation = 10.0

// Threat score → level boundaries.
const (
	ScoreMeltdown = 5
	ScoreHot      = 3
	ScoreWarm     = 1
)

// ── EMA smoothing ──────────────────────────────────────────

const (
	CountSmoothAlpha   = 0.15 // type/category count EMA — fixed categories (build, shell)
	RuntimeSmoothAlpha = 0.05 // slower decay for dynamic runtimes — linger ~6s after processes exit
	RateSmoothAlpha    = 0.3  // spawn/death rate EMA
)

// ── Idle personality ───────────────────────────────────────

const IdleTickerModulo = 8 // ticks between idle message rotation

// ── Category thermal thresholds ────────────────────────────
// [warm, hot] — how many procs before a category gets warm/hot.

var CatThresholds = map[string][2]int{
	"build":  {1, 3},   // few procs but each spawns heavy child trees
	"shell":  {15, 40}, // ephemeral
	"node":   {1, 8},   // warm on first appearance — each ~500MB-1.5GB
	"go":     {1, 4},   // warm on first appearance — heavier per-process
	"python": {1, 6},   // warm on first appearance — variable weight
	"rust":   {1, 4},   // warm on first appearance — heavy compilation
	"swift":  {1, 4},   // warm on first appearance — heavy compilation + linking
}

// Default category thresholds when category name is unknown.
var CatThresholdDefault = [2]int{10, 25}

// ShellExplosionThreshold is the shell count at which a session is considered
// in the "shell explosion" phase (language → build → shell dance).
const ShellExplosionThreshold = 30

// ── Gauge sparkline thresholds ─────────────────────────────

const (
	CPUSparkWarn    = 70.0
	CPUSparkCrit    = 90.0
	MemSparkWarn    = 60.0
	MemSparkCrit    = 80.0
	DecompSparkWarn = 5000.0 // decompressions/tick
	DecompSparkCrit = 20000.0
	SwapSparkWarn   = 2.0  // GB — aligns with SwapWarmBytes
	SwapSparkCrit   = 8.0  // GB — aligns with SwapHotBytes (half physical RAM)
	GPUSparkWarn    = 60.0 // GPU Device Utilization %
	GPUSparkCrit    = 85.0
	// Token throughput is EMA-smoothed across the four counter axes
	// (input/output/cache create/cache read). Defaults sized for typical
	// Claude Code usage: sustained heavy work ≈500 tok/s, parallel agents
	// pushing rate-limit pressure ≈2000 tok/s.
	TokenSparkWarn = 500.0
	TokenSparkCrit = 2000.0
)

// ── Battery ───────────────────────────────────────────────

// Battery severity thresholds (percent remaining).
const (
	BatteryMeltdownPct = 10.0 // below this, cell pulses locally (crit throb)
	BatteryCritPct     = 20.0 // red below this
	BatteryWarnPct     = 50.0 // amber below this, green above
)

// Battery cell rendering.
const (
	BatteryCellWidth           = 8    // 3 braille (battery shape) + space + text
	BatteryChargeBreathFloor   = 0.35 // brightness floor during charging breath
	BatteryChargeBreathCeil    = 0.80 // brightness ceiling during charging breath
	BatteryBreathRate          = 0.35 // multiplier on BreathePhaseStep — slower than agent dots
	BatteryMeltdownBreathFloor = 0.6  // aggressive brightness floor for <10% meltdown pulse
	BatteryMeltdownBreathCeil  = 1.0  // brightness ceiling for meltdown pulse
	BatteryWarnBreathFloor     = 0.85 // subtle brightness floor for 10-20% warning breath
	BatteryWarnBreathCeil      = 1.0  // brightness ceiling for warning breath
	BatteryWarnBreathRate      = 0.5  // 1 cycle per 2 seconds at AnimFPS=60
)

// ── Headroom warnings ──────────────────────────────────────

const (
	HeadroomCritBytes int64 = 2 << 30 // 2GB
	HeadroomWarnBytes int64 = 4 << 30 // 4GB
)

// ── Click-to-inspect ──────────────────────────────────────

// ClickDebounce is the cooldown after opening a focused-agent overlay.
// Agent-zone clicks within this window are swallowed to prevent
// double-click flicker from toggling the overlay open→closed.
const ClickDebounce = 300 * time.Millisecond

// ── Help panel ─────────────────────────────────────────────

// HelpShortMinWidth is the minimum strip width before the short help hint
// degrades from a full key·desc list to a compact "[?]" token.
const HelpShortMinWidth = 80

// ── Build/shell ember rails ───────────────────────────────
// BuildShellEmberDecay is the time over which the directional heat rail
// above `build:NNN` (and below `shell:NNN`) cools from its peak
// CategoryGradient fg back to the pinned headline bg after the smoothed
// count drops below the level that set the peak.
const BuildShellEmberDecay = 2 * time.Second

// ── Heat bloom ───────────────────────────────────────────
// Motion and geometry for the thermographic accent layer rendered behind
// the headline's left zone. Endpoints interpolate linearly across the
// composite heat scalar; anim.Profile variants scale these defaults.

// Breathe period (seconds) — slow at idle, fast at meltdown.
const (
	BloomBreatheSecCool = 6.0
	BloomBreatheSecHot  = 2.2
)

// Scale amplitude — the ± fractional ellipse-radius expansion per breath.
const (
	BloomScaleAmpCool = 0.02
	BloomScaleAmpHot  = 0.08
)

// Opacity envelope — (min, max) alpha range sampled across each breath.
const (
	BloomOpacityMinCool = 0.72
	BloomOpacityMaxCool = 0.88
	BloomOpacityMinHot  = 0.85
	BloomOpacityMaxHot  = 1.00
)

// Spring physics governing heat-target easing. ~400ms settle, minor overshoot.
const (
	BloomSpringFreq    = 3.0
	BloomSpringDamping = 0.9
)

// Geometry — all values are fractions of the left-zone's bounding box.
const (
	BloomAnchorX       = 0.12 // bloom center, left-anchored
	BloomAnchorY       = 0.5  // vertical center
	BloomRadiusX       = 0.55 // horizontal ellipse radius
	BloomRadiusY       = 1.1  // vertical ellipse radius (oversized — softens top/bottom)
	BloomFalloffExp    = 1.8  // gaussian-ish falloff exponent
	BloomRightBoundary = 1.00 // bloom alpha must be 0 past this fraction of left-zone
)
