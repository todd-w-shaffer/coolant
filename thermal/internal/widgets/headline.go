package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// ThreatColor maps threat levels to lipgloss colors (used by alerts too).
var ThreatColor = map[model.ThreatLevel]color.Color{
	model.ThreatCool:     lipgloss.Color("2"),   // green
	model.ThreatWarm:     lipgloss.Color("3"),   // yellow
	model.ThreatHot:      lipgloss.Color("208"), // orange
	model.ThreatMeltdown: lipgloss.Color("1"),   // red
}

// Overall thermal gradient — same 5-level scheme as category boxes.
var overallGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold
	{lipgloss.Color("2"), lipgloss.Color("233")},   // cool: green text
	{lipgloss.Color("3"), lipgloss.Color("234")},   // warm: yellow text
	{lipgloss.Color("208"), lipgloss.Color("235")}, // hot: orange text
	{lipgloss.Color("196"), lipgloss.Color("52")},  // critical: red on dark red
}

// Headline renders the unified thermal bar:
// [ Claude's humming along  | test:004 | build:008 | run:018 | search:005 | shell:004 ]
type Headline struct {
	width int
	state *model.AppState
}

func NewHeadline() *Headline {
	return &Headline{}
}

func (h *Headline) SetSize(w, height int) {
	h.width = w
}

func (h *Headline) Update(state *model.AppState) {
	h.state = state
}

func (h *Headline) View() string {
	if h.state == nil {
		return ""
	}

	numCats := len(collector.Categories)

	// 50/50 split: overall gets half, categories share the other half
	overallWidth := h.width / 2
	if overallWidth < 20 {
		overallWidth = 20
	}
	catTotalWidth := h.width - overallWidth
	catCellWidth := catTotalWidth / numCats
	if catCellWidth < 10 {
		catCellWidth = 10
	}

	// Build overall cell — offline gets its own look
	var overallCell string
	if !h.state.Online {
		quip := model.OfflineMessage(h.state.OfflineDuration, h.state.IdleCycle)
		if len(quip) > overallWidth-2 {
			quip = quip[:overallWidth-2]
		}
		overallContent := fmt.Sprintf(" %-*s", overallWidth-1, quip)
		overallCell = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")). // true black text
			Background(lipgloss.Color("67")).      // steel blue bg
			Render(overallContent)
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		overallThermal := overallGradient[overallLevel]
		quip := h.state.StableQuip()
		if len(quip) > overallWidth-2 {
			quip = quip[:overallWidth-2]
		}
		overallContent := fmt.Sprintf(" %-*s", overallWidth-1, quip)
		overallCell = lipgloss.NewStyle().
			Foreground(overallThermal.fg).
			Background(overallThermal.bg).
			Render(overallContent)
	}

	// Build category cells
	var catCells []string
	for _, cat := range collector.Categories {
		smoothed := h.state.SmoothedCats[cat.Name]
		count := int(math.Round(smoothed))
		level := thermalLevelFor(cat.Name, count)
		thermal := thermalGradient[level]

		content := fmt.Sprintf("%s:%03d", cat.Label, count)

		// Center in cell
		padTotal := catCellWidth - len(content)
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

		catCells = append(catCells, cell)
	}

	return overallCell + strings.Join(catCells, "")
}

func threatToThermal(t model.ThreatLevel) int {
	switch t {
	case model.ThreatCool:
		return 1
	case model.ThreatWarm:
		return 2
	case model.ThreatHot:
		return 3
	case model.ThreatMeltdown:
		return 4
	default:
		return 0
	}
}
