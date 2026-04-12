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

// Agent icon breathing animation.
const (
	BreatheMinBright = 0.25  // dimmest point of breathing cycle
	BreatheMaxBright = 1.0   // brightest point
	BreathePhaseStep = 0.105 // radians per AnimTick (~2s cycle at 30fps: 2π/60)
	BreatheFadeEps   = 0.01  // spring position below which a dying icon is removed
	BreatheStaleRate = 0.3   // phase advance multiplier for stale (orphaned) dots
	BreatheStaleDim  = 0.35  // brightness multiplier for stale dots
)

// KITT scanner (stale ghost dots / highscore completed dots).
const (
	KITTSweepRate    = 0.04 // sweep position advance per AnimTick (~3s per full sweep)
	KITTAmbient      = 0.15 // floor brightness at sweep edges
	KITTPeak         = 0.85 // peak contribution above ambient
	KITTSigmaSq      = 0.8  // gaussian width (sigma²) — tighter = sharper spotlight
	KITTSingleBright = 0.8  // brightness multiplier when only one dot (no sweep)
)

// Tidal wave (active agent dots).
const (
	TidalPhaseStep    = 0.025 // phase advance per AnimTick (~8s per full wave)
	TidalWaveMix      = 0.85  // tidal wave weight in brightness blend
	TidalBreathMix    = 0.15  // individual breath weight in brightness blend
	TidalBrightFloor  = 0.5   // minimum brightness for active dots
	TidalPhaseSpread  = 1.5   // radians between adjacent dots (wider = clearer wave direction)
	GlyphFilledThresh = 0.66  // wave value above which glyph shows ⬢ (filled)
	GlyphMidThresh    = 0.33  // wave value above which glyph shows ⏣ (benzene)
)

// ── History / buffer sizes ─────────────────────────────────

const (
	MaxHistory     = 600 // ~90s at 150ms — fills sparklines on wide terminals
	MaxAlerts      = 100 // scrolling alert log cap
	RateWindowSize = 10  // rolling window for spawn/death rate
)

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
)

// ── Headroom warnings ──────────────────────────────────────

const (
	HeadroomCritBytes int64 = 2 << 30 // 2GB
	HeadroomWarnBytes int64 = 4 << 30 // 4GB
)

// ── Help panel ─────────────────────────────────────────────

const (
	// HelpAutoDismiss is how long full-help stays on screen before auto-collapsing.
	HelpAutoDismiss = 5 * time.Second

	// HelpShortMinWidth is the minimum strip width before the short help hint
	// degrades from a full key·desc list to a compact "[?]" token.
	HelpShortMinWidth = 80
)
