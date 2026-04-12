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

// Gauge label words — pre-rendered once at init.
var gaugeLabels [3]BrailleWord

func init() {
	gaugeLabels[0] = RenderBrailleWord("CPU")
	gaugeLabels[1] = RenderBrailleWord("MEM")
	gaugeLabels[2] = RenderBrailleWord("SWAP")
}

// Gauges renders 3 sparklines: CPU%, MEM%, compressor decompressions/tick.
// Braille text labels scroll in at startup and get pushed off by incoming data.
// Numeric readouts are spring-animated for smooth easing between values.
// Sparklines scroll at animation rate (30fps) via spring-interpolated render history.
type Gauges struct {
	width         int
	state         *model.AppState
	tick          int
	spring        harmonica.Spring
	springs       [3]springState // one per gauge: cpu, mem, compressor
	targets       [3]float64     // snapshot target values
	renderHistory [3][]float64   // spring-interpolated samples pushed every AnimTick
	renderOnline  []bool         // online/offline state pushed every AnimTick
	peaks         [3]float64     // decaying peak per gauge — snaps up, fades slowly
	seeded        bool           // true after first snapshot (skip spring on init)
	sparkBufs     [3]*SparkBufs  // reusable interpolation buffers, one per gauge
	theme         *theme.Theme
	anim          *anim.Profile
	dimmed        bool // render via theme's dim LUTs (for behind-overlay mode)
}

// SetDimmed toggles dim rendering. Layout flips this on before rendering
// gauges behind an overlay, and off after.
func (g *Gauges) SetDimmed(d bool) { g.dimmed = d }

func NewGauges(th *theme.Theme, ap *anim.Profile) *Gauges {
	return &Gauges{
		spring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), ap.SpringFreq, ap.SpringDamping),
		theme:  th,
		anim:   ap,
	}
}

func (g *Gauges) SetSize(w, h int) {
	g.width = w
}

func (g *Gauges) Update(state *model.AppState) {
	g.state = state

	if state == nil || state.Current == nil {
		return
	}

	decomps := float64(state.Current.System.Decompressions)
	g.targets[0] = state.Current.System.CPUPercent
	g.targets[1] = state.Current.System.MemPercent()
	g.targets[2] = decomps

	// First snapshot: jump to target immediately (no spring from zero)
	if !g.seeded {
		for i := range g.springs {
			g.springs[i].pos = g.targets[i]
			g.springs[i].vel = 0
		}
		g.seeded = true
	}
}

// AnimTick advances spring physics one frame (~30fps) and appends the
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

	// Peak smoothing: snap up on spikes, decay fast.
	// Adjusted decay for 30fps (~1.3s half-life: 0.982^30 ≈ 0.58/s).
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
		data    []float64
		display float64 // spring-animated value (for numeric readout)
		max     float64
		thresh  theme.SparkThresholds
		dotIdx  int // index into theme.GaugeDots (used for label color)
		fmtVal  func(float64) string
	}

	fmtPct := func(v float64) string { return fmt.Sprintf("%3d%%", int(v)) }
	fmtDecomp := func(v float64) string {
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

	gauges := []gauge{
		{g.renderHistory[0], g.springs[0].pos, 100,
			CPUSparkThresh(), 0, fmtPct},
		{g.renderHistory[1], g.springs[1].pos, 100,
			MemSparkThresh(), 1, fmtPct},
		{g.renderHistory[2], g.springs[2].pos, g.peaks[2],
			DecompSparkThresh(), 2, fmtDecomp},
	}

	var lines []string
	valuePad := strings.Repeat(" ", valueWidth)

	for i, ga := range gauges {
		// Lazily allocate reusable interpolation buffers
		if g.sparkBufs[i] == nil {
			g.sparkBufs[i] = NewSparkBufs(sparkWidth)
		}

		// Render 2-row sparkline with online/offline mask (buffer-pooled)
		pair := RenderSparkline(ga.data, g.renderOnline, sparkWidth, ga.max, &ga.thresh, g.tick+i*2, g.sparkBufs[i], g.theme, g.dimmed)

		labelANSI := g.theme.GaugeDots[ga.dotIdx].ANSI
		if g.dimmed {
			labelANSI = g.theme.GaugeDots[ga.dotIdx].DimmedANSI
		}
		pair = OverlayLabel(pair, gaugeLabels[i], len(ga.data), sparkWidth, labelANSI)

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
