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
const (
	SwapWarmBytes = 2 << 30  // 2GB — baseline noise
	SwapHotBytes  = 8 << 30  // 8GB — real pressure
	SwapCritBytes = 20 << 30 // 20GB — meltdown territory
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
	CountSmoothAlpha = 0.15 // type/category count EMA
	RateSmoothAlpha  = 0.3  // spawn/death rate EMA
)

// ── Idle personality ───────────────────────────────────────

const IdleTickerModulo = 8 // ticks between idle message rotation

// ── Category thermal thresholds ────────────────────────────
// [warm, hot] — how many procs before a category gets warm/hot.

var CatThresholds = map[string][2]int{
	"test":   {2, 4},   // each ~1GB
	"build":  {3, 6},   // ~300MB each
	"run":    {4, 8},   // variable weight
	"search": {10, 25}, // lightweight
	"shell":  {15, 40}, // ephemeral
}

// Default category thresholds when category name is unknown.
var CatThresholdDefault = [2]int{10, 25}

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
	HeadroomCritBytes = 2 // multiplied by GB in model package
	HeadroomWarnBytes = 4 // multiplied by GB in model package
)
