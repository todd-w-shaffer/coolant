package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// springState tracks harmonica spring position and velocity for one gauge.
type springState struct {
	pos float64
	vel float64
}

// SparklineSlot identifies a gauge slot by stable index. Toggle keys and the
// visibility mask address slots by these constants so renaming a label or
// reordering the visual stack doesn't ripple through dispatch code.
type SparklineSlot int

const (
	SlotCPU SparklineSlot = iota
	SlotMEM
	SlotDecomp // labeled SWAP on-screen — actually compressor decompressions/tick, not swap bytes
	SlotToken
	SlotPretty // chars÷4 estimate from transcript byte growth — mimics CC's UI counter
	NumSparklineSlots = 5
)

// MaxVisibleSparklines is the cap enforced by ToggleVisible. Toggling on a
// hidden slot when this many are already visible is a silent no-op.
const MaxVisibleSparklines = 3

// slotLabels is the single source of truth for sparkline naming. The short
// form drives the braille label rendered on the sparkline itself; the long
// form drives the help-overlay description. Adding a slot means appending
// one entry here, not editing strings in two files.
var slotLabels = [NumSparklineSlots]struct{ short, long string }{
	SlotCPU:    {"CPU", "CPU cores"},
	SlotMEM:    {"MEM", "MEM app memory"},
	SlotDecomp: {"SWAP", "SWAP compressor pressure"},
	SlotToken:  {"TKNS", "TKNS tokens/sec"},
	SlotPretty: {"PRTY", "PRTY chars÷4"},
}

// SlotLongLabel returns the help-overlay description for a slot.
func SlotLongLabel(slot SparklineSlot) string {
	if slot < 0 || int(slot) >= NumSparklineSlots {
		return ""
	}
	return slotLabels[slot].long
}

// Gauge label words — pre-rendered once at init from slotLabels[*].short.
var gaugeLabels [NumSparklineSlots]BrailleWord

func init() {
	for slot, lbl := range slotLabels {
		gaugeLabels[slot] = RenderBrailleWord(lbl.short)
	}
}

// Gauges renders up to MaxVisibleSparklines of NumSparklineSlots possible
// sparklines. Hidden slots skip RenderSparkline upstream — never post-filter
// marker-bearing strings (bubblezone constraint). Springs and renderHistory
// keep updating for hidden slots so toggling on doesn't show an empty graph.
type Gauges struct {
	width         int
	state         *model.AppState
	tick          int
	spring        harmonica.Spring
	springs       [NumSparklineSlots]springState // one per slot
	targets       [NumSparklineSlots]float64     // snapshot target values
	renderHistory [NumSparklineSlots][]float64   // spring-interpolated samples pushed every AnimTick
	renderOnline  []bool                         // online/offline state pushed every AnimTick
	peaks         [NumSparklineSlots]float64     // decaying peak per slot — snaps up, fades slowly
	seeded        bool                           // true after first snapshot (skip spring on init)
	sparkBufs     [NumSparklineSlots]*SparkBufs  // reusable interpolation buffers, one per slot
	visible       [NumSparklineSlots]bool        // per-slot visibility; default set in NewGauges
	theme         *theme.Theme
	anim          *anim.Profile
	dimmed        bool // render via theme's dim LUTs (for behind-overlay mode)
}

// SetDimmed toggles dim rendering. Layout flips this on before rendering
// gauges behind an overlay, and off after.
func (g *Gauges) SetDimmed(d bool) { g.dimmed = d }

func NewGauges(th *theme.Theme, ap *anim.Profile) *Gauges {
	g := &Gauges{
		spring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), ap.SpringFreq, ap.SpringDamping),
		theme:  th,
		anim:   ap,
	}
	// Default visible set: CPU + MEM + Token. Decomp is toggleable but
	// hidden by default — token is more broadly useful to Claude Code
	// users, decomp is a specialized memory-pressure signal.
	g.visible[SlotCPU] = true
	g.visible[SlotMEM] = true
	g.visible[SlotToken] = true
	return g
}

// VisibleCount returns how many slots are currently shown.
func (g *Gauges) VisibleCount() int {
	n := 0
	for _, v := range g.visible {
		if v {
			n++
		}
	}
	return n
}

// IsVisible reports whether a slot is currently rendered.
func (g *Gauges) IsVisible(slot SparklineSlot) bool {
	if slot < 0 || int(slot) >= NumSparklineSlots {
		return false
	}
	return g.visible[slot]
}

// ToggleVisible flips a slot's visibility. Toggle-on with the row full is
// a silent no-op so the MaxVisibleSparklines cap can't be bypassed from
// dispatch code that doesn't know about it.
func (g *Gauges) ToggleVisible(slot SparklineSlot) {
	if slot < 0 || int(slot) >= NumSparklineSlots {
		return
	}
	if g.visible[slot] {
		g.visible[slot] = false
		return
	}
	if g.VisibleCount() >= MaxVisibleSparklines {
		return // silent no-op — sparkline row is full
	}
	g.visible[slot] = true
}

func (g *Gauges) SetSize(w, h int) {
	g.width = w
}

func (g *Gauges) Update(state *model.AppState) {
	g.state = state

	if state == nil || state.Current == nil {
		return
	}

	g.targets[SlotCPU] = state.Current.System.CPUPercent
	g.targets[SlotMEM] = state.Current.System.MemPercent()
	g.targets[SlotDecomp] = float64(state.Current.System.Decompressions)
	g.targets[SlotToken] = state.Current.Tokens.IOTokensPerSec
	g.targets[SlotPretty] = state.Current.Tokens.PrettyTokensPerSec

	// First snapshot: jump to target immediately (no spring from zero)
	if !g.seeded {
		for i := range g.springs {
			g.springs[i].pos = g.targets[i]
			g.springs[i].vel = 0
		}
		g.seeded = true
	}
}

// AnimTick advances spring physics one frame (config.AnimFPS) and appends the
// spring-interpolated value to the render history so sparklines scroll
// at animation rate, not collector rate.
func (g *Gauges) AnimTick() {
	for i := range g.springs {
		g.springs[i].pos, g.springs[i].vel = g.spring.Update(
			g.springs[i].pos, g.springs[i].vel, g.targets[i],
		)
	}

	if !g.seeded {
		return
	}

	g.tick++

	// Push spring position into render history — this is what drives sparkline scrolling.
	for i := range g.springs {
		g.renderHistory[i] = append(g.renderHistory[i], g.springs[i].pos)
		if len(g.renderHistory[i]) > config.MaxRenderHistory {
			g.renderHistory[i] = g.renderHistory[i][len(g.renderHistory[i])-config.MaxRenderHistory:]
		}
	}

	// Track online state at render rate
	online := g.state != nil && g.state.Online
	g.renderOnline = append(g.renderOnline, online)
	if len(g.renderOnline) > config.MaxRenderHistory {
		g.renderOnline = g.renderOnline[len(g.renderOnline)-config.MaxRenderHistory:]
	}

	// Peak smoothing: snap up on spikes, decay fast. The decay rate is
	// derived from a wall-clock half-life (config.peakDecayHalfLifeSec) so
	// the feel is invariant to AnimFPS.
	decayRate := g.anim.PeakDecayRate
	visibleSamples := g.width
	if visibleSamples < 1 {
		visibleSamples = 120
	}
	for i, hist := range g.renderHistory {
		start := 0
		if len(hist) > visibleSamples {
			start = len(hist) - visibleSamples
		}
		var windowPeak float64
		for _, v := range hist[start:] {
			if v > windowPeak {
				windowPeak = v
			}
		}
		if windowPeak > g.peaks[i] {
			g.peaks[i] = windowPeak
		} else {
			g.peaks[i] *= decayRate
			if g.peaks[i] < windowPeak {
				g.peaks[i] = windowPeak
			}
		}
	}
}

// ViewLines returns sparkline output as a slice of lines, trimmed to fit
// the given number of available lines. Full output is 6 lines (3 gauges
// × 2 rows each). With fewer lines, lower-priority gauges are dropped:
// ≥6 keeps all, ≥5 keeps CPU+MEM, ≥3 keeps CPU only, <3 returns nil.
func (g *Gauges) ViewLines(avail int) []string {
	if avail < 3 {
		return nil
	}
	full := g.View()
	if full == "" {
		return nil
	}
	lines := strings.Split(full, "\n")
	switch {
	case avail >= 6 || len(lines) <= avail:
		return lines
	case avail >= 5 && len(lines) >= 4:
		return lines[:4]
	default:
		if len(lines) >= 2 {
			return lines[:2]
		}
		return lines
	}
}

func (g *Gauges) View() string {
	if g.state == nil || g.state.Current == nil {
		return ""
	}

	// Layout: " <sparkline>      " (top)
	//         " <sparkline> NNN% " (bottom)
	margin := 1     // leading space
	valueWidth := 5 // " 100%"
	sparkWidth := g.width - margin - valueWidth - 1
	if sparkWidth < 1 {
		sparkWidth = 1
	}

	type gauge struct {
		slot     SparklineSlot
		data     []float64
		display  float64 // spring-animated value (for numeric readout)
		max      float64
		thresh   theme.SparkThresholds
		dotIdx   int  // index into theme.GaugeDots (used for label color)
		fmtVal   func(float64) string
		logScale bool // log1p height transform — see Token/Pretty entries below
	}

	fmtPct := func(v float64) string { return fmt.Sprintf("%3d%%", int(v)) }
	fmtCount := func(v float64) string {
		n := int64(v)
		switch {
		case n >= 1_000_000:
			return fmt.Sprintf("%3dM", n/1_000_000)
		case n >= 1_000:
			return fmt.Sprintf("%3dK", n/1_000)
		default:
			return fmt.Sprintf("%4d", n)
		}
	}

	// Token/Pretty render in log1p height mode anchored at crit (4000): a
	// typical chat reply ~500 io/s reads as ~75% height, a heavy 2000 io/s
	// burst as ~92%, and a parallel-multi-agent 8000+ io/s as full height
	// with red color. Without log, anything ≥ warn (1000) pinned at 100%
	// and color was the only differentiator above warn. Decomp keeps the
	// autoscale path because its range spans many orders of magnitude
	// already and there's no fixed "interesting ceiling" to anchor against.
	tokThresh := TokenSparkThresh()

	// Fixed-size array (not slice) so this stays on the stack — View runs
	// every render frame.
	allGauges := [NumSparklineSlots]gauge{
		{SlotCPU, g.renderHistory[SlotCPU], g.springs[SlotCPU].pos, 100,
			CPUSparkThresh(), int(SlotCPU), fmtPct, false},
		{SlotMEM, g.renderHistory[SlotMEM], g.springs[SlotMEM].pos, 100,
			MemSparkThresh(), int(SlotMEM), fmtPct, false},
		{SlotDecomp, g.renderHistory[SlotDecomp], g.springs[SlotDecomp].pos, g.peaks[SlotDecomp],
			DecompSparkThresh(), int(SlotDecomp), fmtCount, false},
		{SlotToken, g.renderHistory[SlotToken], g.springs[SlotToken].pos, tokThresh.Crit,
			tokThresh, int(SlotToken) % len(g.theme.GaugeDots), fmtCount, true},
		{SlotPretty, g.renderHistory[SlotPretty], g.springs[SlotPretty].pos, tokThresh.Crit,
			tokThresh, int(SlotPretty) % len(g.theme.GaugeDots), fmtCount, true},
	}

	var lines []string
	valuePad := strings.Repeat(" ", valueWidth)

	for _, ga := range allGauges {
		// Skip upstream of RenderSparkline (bubblezone marker policy).
		if !g.visible[ga.slot] {
			continue
		}

		// Lazily allocate reusable interpolation buffers
		if g.sparkBufs[ga.slot] == nil {
			g.sparkBufs[ga.slot] = NewSparkBufs(sparkWidth)
		}

		// Render 2-row sparkline with online/offline mask (buffer-pooled).
		// Per-slot scale strategy: log1p for Token/Pretty (wide dynamic
		// range above warn), linear elsewhere.
		render := RenderSparkline
		if ga.logScale {
			render = RenderSparklineLog
		}
		pair := render(ga.data, g.renderOnline, sparkWidth, ga.max, &ga.thresh, g.tick+int(ga.slot)*2, g.sparkBufs[ga.slot], g.theme, g.dimmed)

		labelANSI := g.theme.GaugeDots[ga.dotIdx].ANSI
		if g.dimmed {
			labelANSI = g.theme.GaugeDots[ga.dotIdx].DimmedANSI
		}
		pair = OverlayLabel(pair, gaugeLabels[ga.slot], len(ga.data), sparkWidth, labelANSI)

		// Current value — spring-animated, colored by severity gradient
		var coloredVal string
		if !g.state.Online {
			coloredVal = "\033[2;36m" + "----" + sparkReset
		} else {
			var valColor string
			if g.dimmed {
				valColor = g.theme.SeverityColorDimmed(ga.display, &ga.thresh)
			} else {
				valColor = g.theme.SeverityColor(ga.display, &ga.thresh)
			}
			coloredVal = valColor + ga.fmtVal(ga.display) + sparkReset
		}

		lines = append(lines, fmt.Sprintf(" %s %s", pair.Top, valuePad))
		lines = append(lines, fmt.Sprintf(" %s %s", pair.Bottom, coloredVal))
	}

	return strings.Join(lines, "\n")
}
