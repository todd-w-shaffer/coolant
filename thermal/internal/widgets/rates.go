package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// humanizeRate formats a per-second rate compactly: 47 → "47", 1234 → "1.2k",
// 1_500_000 → "1.5M". Negative or NaN values render as "0".
func humanizeRate(v float64) string {
	if v != v || v < 0 { // NaN or negative
		return "0"
	}
	switch {
	case v < 1000:
		return fmt.Sprintf("%d", int(v+0.5))
	case v < 1_000_000:
		return fmt.Sprintf("%.1fk", v/1000)
	case v < 1_000_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	default:
		return fmt.Sprintf("%.1fG", v/1_000_000_000)
	}
}


type Rates struct {
	width    int
	state    *model.AppState
	theme    *theme.Theme
	keys     keys.KeyMap
	help     *HelpRenderer
	ioPeak   float64   // last observed peak IOTokensPerSec, decays exponentially
	ioPeakAt time.Time // wall-clock timestamp of the last snap-up
	now      func() time.Time
}

func NewRates(th *theme.Theme, km keys.KeyMap) *Rates {
	return &Rates{theme: th, keys: km, help: NewHelpRenderer(th), now: time.Now}
}

func (r *Rates) SetSize(w, h int) {
	r.width = w
	r.help.SetWidth(w)
}

// ioPeakHalfLife derives the readout's decay rate from the sparkline's
// visible window so the number fades roughly in step with the bar scrolling
// off the left edge. sparkWidth = width - 7 (margin + value readout +
// trailing space) and the visible window holds (sparkWidth + 1) / 30
// seconds of samples. Half-life = window / 4 puts the tail at ~6% by the
// time the bar has fully scrolled.
func (r *Rates) ioPeakHalfLife() float64 {
	sparkSeconds := float64(r.width-6) / 30.0
	if sparkSeconds < 1 {
		sparkSeconds = 1 // very narrow terminal floor
	}
	return sparkSeconds / 4
}

// decayedIOPeak returns ioPeak after exponential decay since the last
// snap-up.
func (r *Rates) decayedIOPeak() float64 {
	if r.ioPeak <= 0 {
		return 0
	}
	elapsed := r.now().Sub(r.ioPeakAt).Seconds()
	if elapsed <= 0 {
		return r.ioPeak
	}
	return r.ioPeak * math.Exp2(-elapsed/r.ioPeakHalfLife())
}

func (r *Rates) Update(state *model.AppState) {
	r.state = state
	if state != nil && state.Current != nil {
		// Snap the peak up whenever the live rate exceeds the current
		// (decayed) peak. Otherwise the prior peak continues to fade.
		if rate := state.Current.Tokens.IOTokensPerSec; rate > r.decayedIOPeak() {
			r.ioPeak = rate
			r.ioPeakAt = r.now()
		}
	}
}

func (r *Rates) View() string {
	if r.state == nil || r.state.Current == nil {
		return ""
	}
	s := r.state
	snap := s.Current

	// Offline: show duration instead of rates
	if !s.Online {
		durStr := "just now"
		if s.OfflineDuration >= time.Minute {
			durStr = fmt.Sprintf("%dm %ds", int(s.OfflineDuration.Minutes()), int(s.OfflineDuration.Seconds())%60)
		} else if s.OfflineDuration > 0 {
			durStr = fmt.Sprintf("%ds", int(s.OfflineDuration.Seconds()))
		}
		return " " + ui.ColorText(ui.CyanColor, fmt.Sprintf("OFFLINE %s", durStr)) +
			ui.DimText("  —  no API connection, processes will wind down")
	}

	// Warm/cool/net — fixed width with sign
	spawnStr := fmt.Sprintf("warm:%+04d/s", s.LastSpawns())
	deathStr := fmt.Sprintf("cool:-%03d/s", s.LastDeaths())
	netVal := int(s.NetRate)
	netStr := fmt.Sprintf("net:%+04d/s", netVal)

	// System stats — fixed width
	cpuPct := int(snap.System.CPUPercent)
	memUsedGB := float64(snap.System.MemUsedBytes) / float64(model.GB)
	memTotalGB := snap.System.MemTotalBytes / model.GB
	memPct := snap.System.MemPercent()
	swapGB := float64(snap.System.SwapUsedBytes) / float64(model.GB)
	gpuPct := int(snap.System.GPUPercent)

	var sb strings.Builder
	sb.Grow(256)

	// Gauge stats: " ●CPU:NNN% ●MEM:NN.N/NNGB ●SWAP:NN.NGB ●GPU:NNN%"
	sb.WriteString(" ")
	sb.WriteString(r.theme.GaugeDots[0].Formatted)
	sb.WriteString("CPU:")
	cpuTh := CPUSparkThresh()
	sb.WriteString(r.theme.SeverityColor(snap.System.CPUPercent, &cpuTh))
	sb.WriteString(fmt.Sprintf("%03d%%", cpuPct))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(r.theme.GaugeDots[1].Formatted)
	sb.WriteString("MEM:")
	memTh := MemSparkThresh()
	sb.WriteString(r.theme.SeverityColor(memPct, &memTh))
	sb.WriteString(fmt.Sprintf("%04.1f/%02dGB", memUsedGB, memTotalGB))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(r.theme.GaugeDots[2].Formatted)
	sb.WriteString("SWAP:")
	swapTh := SwapSparkThresh()
	sb.WriteString(r.theme.SeverityColor(swapGB, &swapTh))
	sb.WriteString(fmt.Sprintf("%04.1fGB", swapGB))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(r.theme.GaugeDots[3].Formatted)
	sb.WriteString("GPU:")
	gpuTh := GPUSparkThresh()
	sb.WriteString(r.theme.SeverityColor(snap.System.GPUPercent, &gpuTh))
	sb.WriteString(fmt.Sprintf("%03d%%", gpuPct))
	sb.WriteString(sparkReset)

	// Separator
	sb.WriteString(ui.DimText("  |  "))

	// Rates: warm/cool/net
	sb.WriteString(ui.ColorText(r.theme.SpawnColor, spawnStr))
	sb.WriteString("  ")
	sb.WriteString(ui.ColorText(r.theme.DeathColor, deathStr))
	sb.WriteString("  ")
	sb.WriteString(ui.ColorText(r.theme.NetColor, netStr))

	// The displayed rate IS the decayed peak: snaps up on burst, eases
	// out over the sparkline window. Raw IOTokensPerSec would snap back
	// to 0 the tick after a burst — too fast for the eye to read.
	tok := snap.Tokens
	sb.WriteString("  ")
	sb.WriteString(ui.ColorText(r.theme.TokensColor,
		fmt.Sprintf("io %s/s · cache %.1f%%", humanizeRate(r.decayedIOPeak()), tok.CacheHitRatio*100)))

	// Static indicators: Desktop, Chrome
	if snap.DesktopRunning {
		sb.WriteString("  ")
		sb.WriteString(ui.DimText("⊞ Desktop"))
	}
	if snap.ChromeHostRunning {
		sb.WriteString("  ")
		sb.WriteString(ui.DimText("⊙ Chrome"))
	}

	sb.WriteString("  ")
	sb.WriteString(r.help.Short(r.keys))

	return sb.String()
}

// sessionGroup holds categorized process counts for one session.
// Uses a fixed array indexed by category order instead of a map to avoid
// per-frame map allocations in the hot render path.
type sessionGroup struct {
	cats [numCategories]int
}

// numCategories is len(collector.Categories), known at compile time.
const numCategories = 7

// catIndex maps category name → fixed array index. Built once at init.
var catIndex map[string]int

func init() {
	if numCategories != len(collector.Categories) {
		panic("numCategories drift: update const to match collector.Categories")
	}
	catIndex = make(map[string]int, numCategories)
	for i, cat := range collector.Categories {
		catIndex[cat.Name] = i
	}
}

func (g *sessionGroup) total() int {
	t := 0
	for _, c := range g.cats {
		t += c
	}
	return t
}

// sessionGroupCounts categorizes each session's descendants by activity category.
func sessionGroupCounts(sessions []collector.SessionTree) []sessionGroup {
	if len(sessions) == 0 {
		return nil
	}
	groups := make([]sessionGroup, len(sessions))
	for i, sess := range sessions {
		for _, p := range sess.Descendants {
			cat, ok := collector.TypeToCategory[p.TypeCode]
			if !ok {
				cat = "shell"
			}
			if idx, ok := catIndex[cat]; ok {
				groups[i].cats[idx]++
			}
		}
	}
	return groups
}

// sessionPhaseColor determines the escalation phase for a session based on which
// categories are active: idle → language (canary) → build → shell explosion.
func sessionPhaseColor(g *sessionGroup, th *theme.Theme) color.Color {
	// Shell explosion: highest phase
	shellIdx := catIndex["shell"]
	if g.cats[shellIdx] >= config.C.Categories.ShellExplosion {
		return th.SessionPhase.Explosion
	}

	// Build phase
	buildIdx := catIndex["build"]
	if g.cats[buildIdx] > 0 {
		return th.SessionPhase.Build
	}

	// Language phase (canary): any runtime category active
	for _, name := range collector.RuntimeCategories {
		if idx, ok := catIndex[name]; ok && g.cats[idx] > 0 {
			return th.SessionPhase.Language
		}
	}

	// Active but only shells (below explosion threshold)
	if g.total() > 0 {
		return th.SessionPhase.Active
	}

	// Idle — shouldn't reach here (caller checks total == 0)
	return th.SessionPhase.Idle
}
