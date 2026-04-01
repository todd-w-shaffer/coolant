package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
)

// Gauges renders labeled sparklines for procs, CPU%, MEM%, SWAP.
type Gauges struct {
	width int
	state *model.AppState
}

func NewGauges() *Gauges {
	return &Gauges{}
}

func (g *Gauges) SetSize(w, h int) {
	g.width = w
}

func (g *Gauges) Update(state *model.AppState) {
	g.state = state
}

func (g *Gauges) View() string {
	if g.state == nil || g.state.Current == nil {
		return ""
	}

	labelWidth := 6 // "procs " / "cpu%  " / etc.
	valueWidth := 5 // " 100%"
	sparkWidth := g.width - labelWidth - valueWidth - 2
	if sparkWidth < 5 {
		sparkWidth = 5
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	type gauge struct {
		label   string
		data    []float64
		current float64
		max     float64
		unit    string
		warn    float64
		crit    float64
	}

	gauges := []gauge{
		{"procs", g.state.ProcCountHistory(), float64(g.state.Current.TotalProcs()), 0, "", 50, 100},
		{"cpu%", g.state.CPUHistory(), g.state.Current.System.CPUPercent, 100, "%", 70, 90},
		{"mem%", g.state.MemHistory(), g.state.Current.System.MemPercent(), 100, "%", 60, 80},
		{"swap", g.state.SwapHistory(), g.state.Current.System.SwapPercent(), 100, "%", 1, 50},
	}

	var lines []string
	for _, ga := range gauges {
		label := dim.Render(fmt.Sprintf("%-5s", ga.label))
		spark := RenderSparkline(ga.data, sparkWidth, ga.max)

		// Color the sparkline based on current value
		sparkColor := thresholdColor(ga.current, ga.warn, ga.crit)
		coloredSpark := lipgloss.NewStyle().Foreground(sparkColor).Render(spark)

		// Format current value
		var valStr string
		if ga.unit == "%" {
			valStr = fmt.Sprintf("%3d%%", int(ga.current))
		} else {
			valStr = fmt.Sprintf("%4.0f", ga.current)
		}
		coloredVal := lipgloss.NewStyle().Foreground(sparkColor).Render(valStr)

		lines = append(lines, fmt.Sprintf(" %s %s %s", label, coloredSpark, coloredVal))
	}

	return strings.Join(lines, "\n")
}
