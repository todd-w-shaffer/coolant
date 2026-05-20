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

// ioPeakHalfLife is how long it takes the io-rate readout's peak value to
// halve when no new sample exceeds it. Sized so the eye has time to catch the
// number after a burst — IO samples land at the collector tick rate (~1Hz)
// and the raw IOTokensPerSec snaps back to 0 the next tick, so the readout
// needed a slower-decaying display value to stay readable.
const ioPeakHalfLife = 2.0 // seconds

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

// decayedIOPeak returns ioPeak with exponential decay applied since the last
// snap-up. Half-life is ioPeakHalfLife seconds — a peak of 1000 reads as 500
// two seconds later. Returns 0 if no peak has ever been recorded.
func (r *Rates) decayedIOPeak() float64 {
	if r.ioPeak <= 0 {
		return 0
	}
	elapsed := r.now().Sub(r.ioPeakAt).Seconds()
	if elapsed <= 0 {
		return r.ioPeak
	}
	return r.ioPeak * math.Exp2(-elapsed/ioPeakHalfLife)
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

	// Token throughput + cache hit % — always rendered. The "io" segment
	// shows BOTH the live per-tick rate (snaps to 0 between bursts) and a
	// "(peak N)" companion that decays slowly so the eye has time to catch
	// the burst magnitude after the live value has already returned to 0.
	// The "(peak ...)" parenthetical is suppressed when the decayed peak is
	// below 1 — at that point the live value is the only useful number.
	// Pairs with the toggleable TOK sparkline whose amplitude tracks
	// current activity raw (Phase 5).
	tok := snap.Tokens
	sb.WriteString("  ")
	ioFragment := fmt.Sprintf("io %s/s", humanizeRate(tok.IOTokensPerSec))
	if peak := r.decayedIOPeak(); peak >= 1 {
		ioFragment += fmt.Sprintf(" (peak %s)", humanizeRate(peak))
	}
	ioFragment += fmt.Sprintf(" · cache %.1f%%", tok.CacheHitRatio*100)
	sb.WriteString(ui.ColorText(r.theme.TokensColor, ioFragment))

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
