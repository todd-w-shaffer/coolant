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
	{"●", "\033[35m"}, // swap — magenta dot
}

// Gauges renders 3 sparklines: CPU%, MEM%, SWAP.
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
	}

	gauges := []gauge{
		{g.state.CPUHistory(), g.state.Current.System.CPUPercent, 100,
			SparkThresholds{Warn: 70, Crit: 90},
			gaugeDots[0].dot, gaugeDots[0].color},
		{g.state.MemHistory(), g.state.Current.System.MemPercent(), 100,
			SparkThresholds{Warn: 60, Crit: 80},
			gaugeDots[1].dot, gaugeDots[1].color},
		{g.state.SwapHistory(), g.state.Current.System.SwapPercent(), 0,
			SparkThresholds{Warn: 10, Crit: 100},
			gaugeDots[2].dot, gaugeDots[2].color},
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
			valStr := fmt.Sprintf("%3d%%", int(ga.current))
			coloredVal = valColor + valStr + sparkReset
		}

		lines = append(lines, fmt.Sprintf(" %s %s %s", dot, spark, coloredVal))
	}

	return strings.Join(lines, "\n")
}
