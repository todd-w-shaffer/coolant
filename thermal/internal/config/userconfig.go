// userconfig.go — user-configurable thresholds loaded from TOML.
//
// Coolant ships with sane defaults tuned for a 16GB M-series Mac (see
// tuning.go). This file lets users override any threshold without
// touching Go source. Drop a file at ~/.config/coolant/config.toml
// (or set COOLANT_CONFIG), restart thermo, done.
//
// The parser is intentionally lenient. All of these are equivalent:
//
//	warm_pct = 65        # plain integer
//	warm_pct = 65.0      # float — we'll round it
//	warm_pct = "65"      # string — we'll parse it
//	warm_pct = 65%       # bare unit — we'll strip it
//	warm_pct = "65%"     # quoted unit — also fine
//	warm_gb  = 4GB       # bare unit on byte fields
//	warm_gb  = "4 gb"    # quoted, spaced, lowercase — sure
//
// Missing keys keep the compiled default. Partial configs are
// first-class — set only what you care about.
package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// sanitizeRE catches bare units that would make the TOML parser choke.
// It runs on the raw file text *before* parsing, turning lines like
// `warm_pct = 65%` into `warm_pct = 65` so TOML sees a valid integer.
// Quoted values ("65%") survive TOML parsing on their own and get
// cleaned up later by cleanNumeric during coercion.
var sanitizeRE = regexp.MustCompile(
	`(?m)^(\s*\w+\s*=\s*)` + // key = (with optional whitespace)
		`(-?[0-9]+(?:\.[0-9]+)?)` + // numeric value
		`\s*(%|[Gg][Bb]|[Mm][Bb])\s*$`, // trailing unit: %, GB, gb, MB, mb
)

// ── Runtime config ────────────────────────────────────────────
// These structs hold concrete values — no pointers, no TOML tags.
// Consumers read them as config.C.Memory.WarmPct, etc.

// UserConfig is the top-level container for every tunable threshold.
type UserConfig struct {
	Memory     MemoryConfig
	CPU        CPUConfig
	Swap       SwapConfig
	Headroom   HeadroomConfig
	Spawn      SpawnConfig
	Score      ScoreConfig
	Sparklines SparklineConfig
	Categories CategoryConfig
	Updates    UpdatesConfig
}

// MemoryConfig controls when RAM usage triggers threat escalation.
// Values are percentages of total physical RAM (0–100).
type MemoryConfig struct {
	WarmPct int // start watching — default 65
	HotPct  int // getting uncomfortable — default 80
	CritPct int // swap is imminent — default 90
}

// CPUConfig controls when CPU utilization triggers threat escalation.
// Values are percentages of total CPU (0–100).
type CPUConfig struct {
	WarmPct int // elevated — default 75
	CritPct int // sustained ceiling — default 90
}

// SwapConfig controls when swap usage triggers threat escalation.
// Values are stored as bytes internally; users write gigabytes in TOML.
type SwapConfig struct {
	WarmBytes int64 // baseline noise — default 2 GB
	HotBytes  int64 // real pressure — default 8 GB
	CritBytes int64 // meltdown territory — default 20 GB
}

// HeadroomConfig controls when projected free memory triggers warnings.
// "Headroom" = available RAM minus estimated memory commitment from
// running Claude processes. Values are bytes; users write gigabytes.
type HeadroomConfig struct {
	WarnBytes int64 // heads-up — default 4 GB
	CritBytes int64 // critical — default 2 GB
}

// SpawnConfig controls process-spawn alerting.
type SpawnConfig struct {
	BurstThreshold int     // procs appearing in one tick to trigger an alert — default 8
	RateEscalation float64 // EMA spawn rate that bumps the threat score — default 10.0
}

// ScoreConfig maps composite threat scores to named levels.
// The classifier sums individual signals (mem, cpu, swap, spawn);
// these boundaries decide when the total flips to the next level.
// Must be warm < hot < meltdown.
type ScoreConfig struct {
	Warm     int // ≥ this → WARM — default 1
	Hot      int // ≥ this → HOT — default 3
	Meltdown int // ≥ this → MELTDOWN — default 5
}

// SparklineConfig controls the color breakpoints on gauge sparklines.
// When a gauge value exceeds Warn it turns yellow; past Crit, red.
// Percent fields are 0–100; swap fields are gigabytes.
type SparklineConfig struct {
	CPUWarn    float64 // default 70
	CPUCrit    float64 // default 90
	MemWarn    float64 // default 60
	MemCrit    float64 // default 80
	DecompWarn float64 // decompressions/tick — default 5000
	DecompCrit float64 // default 20000
	SwapWarnGB float64 // default 2.0
	SwapCritGB float64 // default 8.0
	GPUWarn    float64 // default 60
	GPUCrit    float64 // default 85
}

// UpdatesConfig controls automatic update checking behavior.
type UpdatesConfig struct {
	CheckIntervalSec int  // TTL between remote checks — default 86400 (24h)
	Disabled         bool // opt out of update checks entirely — default false
}

// CategoryConfig controls per-category process count thresholds.
// Each category maps to [warm, hot] — how many processes before the
// category row turns amber or red in the headline widget.
type CategoryConfig struct {
	Thresholds     map[string][2]int // e.g. "node" → [1, 8]
	Default        [2]int            // fallback for unknown categories — default [10, 25]
	ShellExplosion int               // shell count that marks a "shell explosion" — default 30
}

// CatThreshold returns the [warm, hot] pair for a category,
// falling back to Default for categories not in the map.
func (uc *UserConfig) CatThreshold(cat string) [2]int {
	if t, ok := uc.Categories.Thresholds[cat]; ok {
		return t
	}
	return uc.Categories.Default
}

// ── Package-level config ──────────────────────────────────────

// C is the active configuration, readable from any package as
// config.C.Memory.WarmPct, etc. Initialized to compiled defaults
// at import time; overwritten by Load() if a config file exists.
var C = Defaults()

const gb = 1 << 30

// Defaults returns a fresh UserConfig populated from the compiled
// constants in tuning.go. This is what you get if no config file
// exists — identical behavior to before this feature was added.
func Defaults() *UserConfig {
	cats := make(map[string][2]int, len(CatThresholds))
	for k, v := range CatThresholds {
		cats[k] = v
	}
	return &UserConfig{
		Memory: MemoryConfig{
			WarmPct: MemWarmPct,
			HotPct:  MemHotPct,
			CritPct: MemCritPct,
		},
		CPU: CPUConfig{
			WarmPct: CPUWarmPct,
			CritPct: CPUCritPct,
		},
		Swap: SwapConfig{
			WarmBytes: SwapWarmBytes,
			HotBytes:  SwapHotBytes,
			CritBytes: SwapCritBytes,
		},
		Headroom: HeadroomConfig{
			WarnBytes: HeadroomWarnBytes,
			CritBytes: HeadroomCritBytes,
		},
		Spawn: SpawnConfig{
			BurstThreshold: SpawnBurstThreshold,
			RateEscalation: SpawnRateEscalation,
		},
		Score: ScoreConfig{
			Warm:     ScoreWarm,
			Hot:      ScoreHot,
			Meltdown: ScoreMeltdown,
		},
		Sparklines: SparklineConfig{
			CPUWarn:    CPUSparkWarn,
			CPUCrit:    CPUSparkCrit,
			MemWarn:    MemSparkWarn,
			MemCrit:    MemSparkCrit,
			DecompWarn: DecompSparkWarn,
			DecompCrit: DecompSparkCrit,
			SwapWarnGB: SwapSparkWarn,
			SwapCritGB: SwapSparkCrit,
			GPUWarn:    GPUSparkWarn,
			GPUCrit:    GPUSparkCrit,
		},
		Categories: CategoryConfig{
			Thresholds:     cats,
			Default:        CatThresholdDefault,
			ShellExplosion: ShellExplosionThreshold,
		},
		Updates: UpdatesConfig{
			CheckIntervalSec: 86400,
			Disabled:         false,
		},
	}
}

// ── Loading ───────────────────────────────────────────────────

// Load reads a TOML config file, merges it onto compiled defaults,
// validates the result, and stores it in C. Call once from main()
// before constructing any widgets.
//
// Behavior:
//   - File missing → C keeps defaults, returns nil (not an error).
//   - File exists but can't be parsed → returns error.
//   - File parses but values are invalid → returns error.
func Load(path string) error {
	C = Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}

	// Pre-process: strip bare trailing units (65%, 4GB) that would
	// make the TOML parser reject the line. Quoted strings survive
	// parsing and get cleaned during coercion instead.
	data = sanitizeRE.ReplaceAll(data, []byte("${1}${2}"))

	// Decode into an untyped map. We do our own type coercion so
	// users don't have to care whether a value is int vs float vs
	// string — we accept all three and figure it out.
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	merge(C, raw)

	if err := validate(C); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	return nil
}

// ── Merge ─────────────────────────────────────────────────────
// Walk the raw TOML map and overlay present values onto the
// defaults. Absent keys are left alone — partial configs work.

func merge(dst *UserConfig, raw map[string]any) {
	if m, ok := section(raw, "memory"); ok {
		coerceInt(&dst.Memory.WarmPct, m, "warm_pct")
		coerceInt(&dst.Memory.HotPct, m, "hot_pct")
		coerceInt(&dst.Memory.CritPct, m, "crit_pct")
	}
	if c, ok := section(raw, "cpu"); ok {
		coerceInt(&dst.CPU.WarmPct, c, "warm_pct")
		coerceInt(&dst.CPU.CritPct, c, "crit_pct")
	}
	if s, ok := section(raw, "swap"); ok {
		coerceGBToBytes(&dst.Swap.WarmBytes, s, "warm_gb")
		coerceGBToBytes(&dst.Swap.HotBytes, s, "hot_gb")
		coerceGBToBytes(&dst.Swap.CritBytes, s, "crit_gb")
	}
	if h, ok := section(raw, "headroom"); ok {
		coerceGBToBytes(&dst.Headroom.WarnBytes, h, "warn_gb")
		coerceGBToBytes(&dst.Headroom.CritBytes, h, "crit_gb")
	}
	if sp, ok := section(raw, "spawn"); ok {
		coerceInt(&dst.Spawn.BurstThreshold, sp, "burst_threshold")
		coerceFloat(&dst.Spawn.RateEscalation, sp, "rate_escalation")
	}
	if sc, ok := section(raw, "score"); ok {
		coerceInt(&dst.Score.Warm, sc, "warm")
		coerceInt(&dst.Score.Hot, sc, "hot")
		coerceInt(&dst.Score.Meltdown, sc, "meltdown")
	}
	if sl, ok := section(raw, "sparklines"); ok {
		coerceFloat(&dst.Sparklines.CPUWarn, sl, "cpu_warn")
		coerceFloat(&dst.Sparklines.CPUCrit, sl, "cpu_crit")
		coerceFloat(&dst.Sparklines.MemWarn, sl, "mem_warn")
		coerceFloat(&dst.Sparklines.MemCrit, sl, "mem_crit")
		coerceFloat(&dst.Sparklines.DecompWarn, sl, "decomp_warn")
		coerceFloat(&dst.Sparklines.DecompCrit, sl, "decomp_crit")
		coerceFloat(&dst.Sparklines.SwapWarnGB, sl, "swap_warn_gb")
		coerceFloat(&dst.Sparklines.SwapCritGB, sl, "swap_crit_gb")
		coerceFloat(&dst.Sparklines.GPUWarn, sl, "gpu_warn")
		coerceFloat(&dst.Sparklines.GPUCrit, sl, "gpu_crit")
	}
	if u, ok := section(raw, "updates"); ok {
		coerceInt(&dst.Updates.CheckIntervalSec, u, "check_interval")
		coerceBool(&dst.Updates.Disabled, u, "disabled")
	}
	if cat, ok := section(raw, "categories"); ok {
		for name := range CatThresholds {
			if pair, err := coercePair(cat, name); err == nil {
				dst.Categories.Thresholds[name] = pair
			}
		}
		if pair, err := coercePair(cat, "default"); err == nil {
			dst.Categories.Default = pair
		}
		coerceInt(&dst.Categories.ShellExplosion, cat, "shell_explosion")
	}
}

// section pulls a [table] out of the raw map.
func section(raw map[string]any, key string) (map[string]any, bool) {
	v, ok := raw[key]
	if !ok {
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// ── Coercion helpers ──────────────────────────────────────────
// The whole point: users shouldn't have to know (or care) whether
// TOML considers their value an integer, a float, or a string.
// We accept all three and coerce to the target type.
//
// If a key is absent we leave the default. If a value is present
// but completely un-parseable we silently skip it (the validator
// will catch any resulting nonsense downstream).

// coerceInt reads a value as int — accepts int64, float64, or string.
func coerceInt(dst *int, m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	switch val := v.(type) {
	case int64:
		*dst = int(val)
	case float64:
		*dst = int(math.Round(val))
	case string:
		if f, err := strconv.ParseFloat(cleanNumeric(val), 64); err == nil {
			*dst = int(math.Round(f))
		}
	}
}

// coerceFloat reads a value as float64 — accepts float64, int64, or string.
func coerceFloat(dst *float64, m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	switch val := v.(type) {
	case float64:
		*dst = val
	case int64:
		*dst = float64(val)
	case string:
		if f, err := strconv.ParseFloat(cleanNumeric(val), 64); err == nil {
			*dst = f
		}
	}
}

// coerceBool reads a value as bool — accepts bool or string ("true"/"false").
func coerceBool(dst *bool, m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	switch val := v.(type) {
	case bool:
		*dst = val
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "true", "yes", "1":
			*dst = true
		case "false", "no", "0":
			*dst = false
		}
	}
}

// coerceGBToBytes reads a value in gigabytes and stores it as bytes.
func coerceGBToBytes(dst *int64, m map[string]any, key string) {
	v, ok := m[key]
	if !ok {
		return
	}
	var f float64
	switch val := v.(type) {
	case float64:
		f = val
	case int64:
		f = float64(val)
	case string:
		parsed, err := strconv.ParseFloat(cleanNumeric(val), 64)
		if err != nil {
			return
		}
		f = parsed
	default:
		return
	}
	*dst = int64(f * float64(gb))
}

// coercePair extracts a [warm, hot] two-element array from a TOML value.
func coercePair(m map[string]any, key string) ([2]int, error) {
	v, ok := m[key]
	if !ok {
		return [2]int{}, fmt.Errorf("missing")
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		return [2]int{}, fmt.Errorf("not a 2-element array")
	}
	a, aOK := toInt(arr[0])
	b, bOK := toInt(arr[1])
	if !aOK || !bOK {
		return [2]int{}, fmt.Errorf("non-numeric elements")
	}
	return [2]int{a, b}, nil
}

// cleanNumeric strips human-friendly suffixes so the string can be
// parsed as a number. Handles %, GB, gb, MB, mb with optional
// whitespace between the number and the unit.
func cleanNumeric(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, suffix := range []string{"gb", "mb"} {
		if strings.HasSuffix(lower, suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}
	return strings.TrimSpace(s)
}

// toInt coerces a single value to int (from int64, float64, or string).
func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int64:
		return int(val), true
	case float64:
		return int(math.Round(val)), true
	case string:
		if f, err := strconv.ParseFloat(cleanNumeric(val), 64); err == nil {
			return int(math.Round(f)), true
		}
	}
	return 0, false
}

// ── Validation ────────────────────────────────────────────────
// Runs after merge. Catches impossible combinations that would
// cause confusing dashboard behavior (e.g. warm > hot means
// you'd skip straight from cool to hot with no warning).

func validate(c *UserConfig) error {
	if err := orderedInt("memory", c.Memory.WarmPct, c.Memory.HotPct, c.Memory.CritPct); err != nil {
		return err
	}

	if c.CPU.WarmPct >= c.CPU.CritPct {
		return fmt.Errorf("cpu: warm_pct (%d) must be less than crit_pct (%d)", c.CPU.WarmPct, c.CPU.CritPct)
	}

	if c.Swap.WarmBytes < 0 || c.Swap.HotBytes < 0 || c.Swap.CritBytes < 0 {
		return fmt.Errorf("swap: thresholds must be non-negative")
	}
	if c.Swap.WarmBytes >= c.Swap.HotBytes || c.Swap.HotBytes >= c.Swap.CritBytes {
		return fmt.Errorf("swap: must be warm_gb < hot_gb < crit_gb")
	}

	if c.Headroom.CritBytes < 0 || c.Headroom.WarnBytes < 0 {
		return fmt.Errorf("headroom: thresholds must be non-negative")
	}
	if c.Headroom.WarnBytes > 0 && c.Headroom.CritBytes > 0 && c.Headroom.WarnBytes <= c.Headroom.CritBytes {
		return fmt.Errorf("headroom: warn_gb must be greater than crit_gb (warn triggers first)")
	}

	if err := orderedInt("score", c.Score.Warm, c.Score.Hot, c.Score.Meltdown); err != nil {
		return err
	}

	if c.Updates.CheckIntervalSec < 0 {
		return fmt.Errorf("updates: check_interval must be non-negative")
	}

	if c.Spawn.BurstThreshold < 0 {
		return fmt.Errorf("spawn: burst_threshold must be non-negative")
	}
	if c.Spawn.RateEscalation < 0 {
		return fmt.Errorf("spawn: rate_escalation must be non-negative")
	}

	return nil
}

func orderedInt(section string, low, mid, high int) error {
	if low >= mid {
		return fmt.Errorf("%s: warm (%d) must be less than hot (%d)", section, low, mid)
	}
	if mid >= high {
		return fmt.Errorf("%s: hot (%d) must be less than crit (%d)", section, mid, high)
	}
	return nil
}
