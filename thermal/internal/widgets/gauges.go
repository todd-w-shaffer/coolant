package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

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
type Gauges struct {
	width   int
	state   *model.AppState
	tick    int
	spring  harmonica.Spring
	springs [3]springState // one per gauge: cpu, mem, compressor
	targets [3]float64     // snapshot target values
	history [3][]float64   // cached history slices, rebuilt on snapshot only
	peaks   [3]float64     // decaying peak per gauge — snaps up, fades slowly
	seeded  bool           // true after first snapshot (skip spring on init)
}

func NewGauges() *Gauges {
	return &Gauges{
		spring: harmonica.NewSpring(harmonica.FPS(15), 5.0, 1.0),
	}
}

func (g *Gauges) SetSize(w, h int) {
	g.width = w
}

func (g *Gauges) Update(state *model.AppState) {
	g.state = state
	g.tick++

	if state == nil || state.Current == nil {
		return
	}

	decomps := float64(state.Current.System.Decompressions)
	g.targets[0] = state.Current.System.CPUPercent
	g.targets[1] = state.Current.System.MemPercent()
	g.targets[2] = decomps

	// Cache history slices — only rebuilt on snapshot, not every frame.
	g.history[0] = state.CPUHistory()
	g.history[1] = state.MemHistory()
	g.history[2] = state.CompressorHistory()

	// Update decaying peaks — snap up instantly on spikes, decay slowly
	// so historical bars don't jump when a spike scrolls out of view.
	const decayRate = 0.995 // ~3s half-life at 150ms ticks
	for i, hist := range g.history {
		// Find current window peak
		var windowPeak float64
		for _, v := range hist {
			if v > windowPeak {
				windowPeak = v
			}
		}
		if windowPeak > g.peaks[i] {
			g.peaks[i] = windowPeak // snap up
		} else {
			g.peaks[i] *= decayRate // slow decay
			if g.peaks[i] < windowPeak {
				g.peaks[i] = windowPeak // floor at current data
			}
		}
	}

	// First snapshot: jump to target immediately (no spring from zero)
	if !g.seeded {
		for i := range g.springs {
			g.springs[i].pos = g.targets[i]
			g.springs[i].vel = 0
		}
		g.seeded = true
	}
}

// AnimTick advances spring physics one frame (~15fps).
func (g *Gauges) AnimTick() {
	for i := range g.springs {
		g.springs[i].pos, g.springs[i].vel = g.spring.Update(
			g.springs[i].pos, g.springs[i].vel, g.targets[i],
		)
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
		current float64 // raw snapshot value (for sparkline color)
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
		{g.history[0], g.targets[0], g.springs[0].pos, g.peaks[0],
			SparkThresholds{Warn: 70, Crit: 90},
			gaugeDots[0].dot, gaugeDots[0].color, fmtPct},
		{g.history[1], g.targets[1], g.springs[1].pos, g.peaks[1],
			SparkThresholds{Warn: 60, Crit: 80},
			gaugeDots[1].dot, gaugeDots[1].color, fmtPct},
		{g.history[2], g.targets[2], g.springs[2].pos, g.peaks[2],
			SparkThresholds{Warn: 5000, Crit: 20000},
			gaugeDots[2].dot, gaugeDots[2].color, fmtDecomp},
	}

	var lines []string
	for i, ga := range gauges {
		dot := ga.dotClr + ga.dot + sparkReset

		// Replace the most recent data point with the spring-animated value
		// so the rightmost sparkline dot eases at 15fps between snapshots.
		data := ga.data
		if len(data) > 0 && g.seeded {
			data = make([]float64, len(ga.data))
			copy(data, ga.data)
			data[len(data)-1] = ga.display
		}

		// Render with online/offline mask — rainbow dots inline with real data
		spark := RenderSparklineWithMask(data, g.state.OnlineLog, sparkWidth, ga.max, &ga.thresh, g.tick+i*2)

		// Current value — spring-animated, colored by severity gradient
		var coloredVal string
		if !g.state.Online {
			coloredVal = "\033[2;36m" + "----" + sparkReset
		} else {
			valColor := severityColor(ga.display, &ga.thresh)
			coloredVal = valColor + ga.fmtVal(ga.display) + sparkReset
		}

		lines = append(lines, fmt.Sprintf(" %s %s %s", dot, spark, coloredVal))
	}

	return strings.Join(lines, "\n")
}
