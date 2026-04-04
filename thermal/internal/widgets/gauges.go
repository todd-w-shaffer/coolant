package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// maxRenderHistory limits the render-rate sample buffer (~20s at 30fps).
const maxRenderHistory = 600

// Gauge dot colors — left-edge indicator per row.
var gaugeDots = []struct {
	dot   string
	color string
}{
	{"●", "\033[37m"}, // cpu — white dot
	{"●", "\033[36m"}, // mem — cyan dot
	{"●", "\033[35m"}, // compressor — magenta dot
}

// springState tracks harmonica spring position and velocity for one gauge.
type springState struct {
	pos float64
	vel float64
}

// Gauges renders 3 sparklines: CPU%, MEM%, compressor decompressions/tick.
// Each dot is severity-colored when online, rainbow when offline.
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
}

func NewGauges() *Gauges {
	return &Gauges{
		spring: harmonica.NewSpring(harmonica.FPS(30), 5.0, 1.0),
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
		if len(g.renderHistory[i]) > maxRenderHistory {
			g.renderHistory[i] = g.renderHistory[i][len(g.renderHistory[i])-maxRenderHistory:]
		}
	}

	// Track online state at render rate
	online := g.state != nil && g.state.Online
	g.renderOnline = append(g.renderOnline, online)
	if len(g.renderOnline) > maxRenderHistory {
		g.renderOnline = g.renderOnline[len(g.renderOnline)-maxRenderHistory:]
	}

	// Peak smoothing: snap up on spikes, decay fast.
	// Adjusted decay for 30fps (~1.3s half-life: 0.982^30 ≈ 0.58/s).
	const decayRate = 0.982
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

func (g *Gauges) View() string {
	if g.state == nil || g.state.Current == nil {
		return ""
	}

	// Layout: " ● <sparkline> NNN%"
	dotWidth := 3   // " ● "
	valueWidth := 5 // " 100%"
	sparkWidth := g.width - dotWidth - valueWidth - 1
	if sparkWidth < 1 {
		sparkWidth = 1
	}

	type gauge struct {
		data    []float64
		display float64 // spring-animated value (for numeric readout)
		max     float64
		thresh  SparkThresholds
		dot     string
		dotClr  string
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
			SparkThresholds{Warn: 70, Crit: 90},
			gaugeDots[0].dot, gaugeDots[0].color, fmtPct},
		{g.renderHistory[1], g.springs[1].pos, 100,
			SparkThresholds{Warn: 60, Crit: 80},
			gaugeDots[1].dot, gaugeDots[1].color, fmtPct},
		{g.renderHistory[2], g.springs[2].pos, g.peaks[2],
			SparkThresholds{Warn: 5000, Crit: 20000},
			gaugeDots[2].dot, gaugeDots[2].color, fmtDecomp},
	}

	var lines []string
	padding := strings.Repeat(" ", dotWidth)
	valuePad := strings.Repeat(" ", valueWidth)

	for i, ga := range gauges {
		dot := ga.dotClr + ga.dot + sparkReset

		// Render 2-row sparkline with online/offline mask
		pair := RenderSparklineWithMask(ga.data, g.renderOnline, sparkWidth, ga.max, &ga.thresh, g.tick+i*2)

		// Current value — spring-animated, colored by severity gradient
		var coloredVal string
		if !g.state.Online {
			coloredVal = "\033[2;36m" + "----" + sparkReset
		} else {
			valColor := severityColor(ga.display, &ga.thresh)
			coloredVal = valColor + ga.fmtVal(ga.display) + sparkReset
		}

		lines = append(lines, fmt.Sprintf("%s%s %s", padding, pair.Top, valuePad))
		lines = append(lines, fmt.Sprintf(" %s %s %s", dot, pair.Bottom, coloredVal))
	}

	return strings.Join(lines, "\n")
}
