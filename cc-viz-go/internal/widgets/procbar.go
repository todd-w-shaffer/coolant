package widgets

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/collector"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
)

// Per-category thermal thresholds: how many procs before it gets warm/hot.
// Encodes danger — 3 test procs is warm, 3 search procs is ice cold.
var catThresholds = map[string][2]int{
	"test":   {2, 4},   // warm at 2, hot at 4 — each ~1GB
	"build":  {3, 6},   // warm at 3, hot at 6 — ~300MB each
	"run":    {4, 8},   // warm at 4, hot at 8 — variable weight
	"search": {10, 25}, // warm at 10, hot at 25 — lightweight
	"shell":  {15, 40}, // warm at 15, hot at 40 — ephemeral
}

// Thermal gradient: 5 levels from invisible to glowing.
// Each level has a foreground color (label+count) and background color (box fill).
type thermalLevel struct {
	fg lipgloss.Color
	bg lipgloss.Color
}

var thermalGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold: nearly invisible
	{lipgloss.Color("240"), lipgloss.Color("234")}, // cool: barely there
	{lipgloss.Color("180"), lipgloss.Color("235")}, // warm: dim amber text
	{lipgloss.Color("214"), lipgloss.Color("236")}, // hot: orange, readable
	{lipgloss.Color("196"), lipgloss.Color("52")},  // critical: bright red on dark red
}

// ProcBar renders five fixed-width thermal boxes:
// [ test:004 ][ build:008 ][ run:018 ][ search:005 ][ shell:004 ]
type ProcBar struct {
	width int
	state *model.AppState
}

func NewProcBar() *ProcBar {
	return &ProcBar{}
}

func (p *ProcBar) SetSize(w, h int) {
	p.width = w
}

func (p *ProcBar) Update(state *model.AppState) {
	p.state = state
}

func (p *ProcBar) View() string {
	if p.state == nil || p.state.Current == nil {
		return ""
	}

	numCats := len(collector.Categories)
	if numCats == 0 {
		return ""
	}

	// Fixed cell width: divide available space evenly
	cellWidth := (p.width - 2) / numCats // -2 for outer margins
	if cellWidth < 10 {
		cellWidth = 10
	}

	var cells []string
	for _, cat := range collector.Categories {
		smoothed := p.state.SmoothedCats[cat.Name]
		count := int(math.Round(smoothed))

		// Determine thermal level (0-4) based on per-category thresholds
		level := thermalLevelFor(cat.Name, count)
		thermal := thermalGradient[level]

		// Format: "label:NNN" — fixed width, zero-padded count
		content := fmt.Sprintf("%s:%03d", cat.Label, count)

		// Pad content to cell width
		padTotal := cellWidth - len(content)
		padLeft := padTotal / 2
		padRight := padTotal - padLeft
		if padLeft < 0 {
			padLeft = 0
		}
		if padRight < 0 {
			padRight = 0
		}

		padded := strings.Repeat(" ", padLeft) + content + strings.Repeat(" ", padRight)

		cell := lipgloss.NewStyle().
			Foreground(thermal.fg).
			Background(thermal.bg).
			Render(padded)

		cells = append(cells, cell)
	}

	return " " + strings.Join(cells, "")
}

// thermalLevelFor returns 0-4 based on count vs category thresholds.
func thermalLevelFor(catName string, count int) int {
	thresh, ok := catThresholds[catName]
	if !ok {
		thresh = [2]int{10, 25} // default
	}
	warm := thresh[0]
	hot := thresh[1]

	if count == 0 {
		return 0 // cold: invisible
	}

	// Interpolate between levels
	switch {
	case count >= hot:
		return 4 // critical
	case count >= (warm+hot)/2:
		return 3 // hot
	case count >= warm:
		return 2 // warm
	default:
		return 1 // cool: barely visible
	}
}
