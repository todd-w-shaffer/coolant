package widgets

import (
	"fmt"
	"strings"

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

// Gauges renders 3 sparklines: CPU%, MEM%, compressor decompressions/tick.
// Each dot is severity-colored when online, rainbow when offline.
// Transitions are seamless — rainbow dots sit inline in the timeline.
type Gauges struct {
	width int
	state *model.AppState
	tick  int
}

func NewGauges() *Gauges {
	return &Gauges{}
}

func (g *Gauges) SetSize(w, h int) {
	g.width = w
}

func (g *Gauges) Update(state *model.AppState) {
	g.state = state
	g.tick++
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
		current float64
		max     float64
		thresh  SparkThresholds
		dot     string
		dotClr  string
		fmtVal  func(float64) string // custom value formatter
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

	decomps := float64(g.state.Current.System.Decompressions)

	gauges := []gauge{
		{g.state.CPUHistory(), g.state.Current.System.CPUPercent, 100,
			SparkThresholds{Warn: 70, Crit: 90},
			gaugeDots[0].dot, gaugeDots[0].color, fmtPct},
		{g.state.MemHistory(), g.state.Current.System.MemPercent(), 100,
			SparkThresholds{Warn: 60, Crit: 80},
			gaugeDots[1].dot, gaugeDots[1].color, fmtPct},
		{g.state.CompressorHistory(), decomps, 0,
			SparkThresholds{Warn: 5000, Crit: 20000},
			gaugeDots[2].dot, gaugeDots[2].color, fmtDecomp},
	}

	var lines []string
	for i, ga := range gauges {
		dot := ga.dotClr + ga.dot + sparkReset

		// Render with online/offline mask — rainbow dots inline with real data
		spark := RenderSparklineWithMask(ga.data, g.state.OnlineLog, sparkWidth, ga.max, &ga.thresh, g.tick+i*2)

		// Current value — colored by threshold, dim if offline
		var coloredVal string
		if !g.state.Online {
			coloredVal = "\033[2;36m" + "----" + sparkReset
		} else {
			valColor := sparkGreen
			if ga.current >= ga.thresh.Crit {
				valColor = sparkRed
			} else if ga.current >= ga.thresh.Warn {
				valColor = sparkYellow
			}
			coloredVal = valColor + ga.fmtVal(ga.current) + sparkReset
		}

		lines = append(lines, fmt.Sprintf(" %s %s %s", dot, spark, coloredVal))
	}

	return strings.Join(lines, "\n")
}
