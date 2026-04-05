package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// Overall thermal gradient — same 5-level scheme as category boxes.
var overallGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold
	{lipgloss.Color("2"), lipgloss.Color("233")},   // cool: green text
	{lipgloss.Color("3"), lipgloss.Color("234")},   // warm: yellow text
	{lipgloss.Color("208"), lipgloss.Color("235")}, // hot: orange text
	{lipgloss.Color("196"), lipgloss.Color("52")},  // critical: red on dark red
}

// Headline renders the unified thermal bar:
// [ Claude's humming along  ◆ ◆ ◆ | test:004 | build:008 | run:018 | search:005 | shell:004 ]
type Headline struct {
	width  int
	state  *model.AppState
	agents *BreatheDots
}

func NewHeadline() *Headline {
	return &Headline{
		agents: NewBreatheDots(),
	}
}

func (h *Headline) SetSize(w, height int) {
	h.width = w
}

func (h *Headline) Update(state *model.AppState) {
	h.state = state
	if state == nil {
		return
	}
	h.agents.SetTarget(state.AgentCount())
}

// AnimTick advances agent icon springs and breathing phases.
func (h *Headline) AnimTick() {
	h.agents.AnimTick()
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

	// Render agent icons (right-aligned in overall cell) — need bg color for transparency
	var iconBg color.Color
	if !h.state.Online {
		iconBg = lipgloss.Color("67")
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		iconBg = overallGradient[overallLevel].bg
	}
	iconStr, iconVisWidth := h.agents.Render(ui.SessionGlyph, iconBg, 0)

	// Build overall cell — offline gets its own look
	var overallCell string
	if !h.state.Online {
		quip := model.OfflineMessage(h.state.OfflineDuration, h.state.IdleCycle)
		bg := lipgloss.Color("67")
		fg := lipgloss.Color("#000000")
		overallCell = h.buildOverallCell(quip, fg, bg, iconStr, iconVisWidth, overallWidth)
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		overallThermal := overallGradient[overallLevel]
		quip := h.state.StableQuip()
		overallCell = h.buildOverallCell(quip, overallThermal.fg, overallThermal.bg, iconStr, iconVisWidth, overallWidth)
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

// buildOverallCell constructs the overall headline cell with quip left-aligned
// and agent icons right-aligned, all sharing the same background.
func (h *Headline) buildOverallCell(quip string, fg, bg color.Color, iconStr string, iconVisWidth, totalWidth int) string {
	// iconMargin: 1 cell gap between quip and icons when icons are present
	iconMargin := 0
	if iconVisWidth > 0 {
		iconMargin = 1
	}

	maxQuip := totalWidth - 2 - iconVisWidth - iconMargin
	if maxQuip < 0 {
		maxQuip = 0
	}
	if len(quip) > maxQuip {
		quip = quip[:maxQuip]
	}

	baseStyle := lipgloss.NewStyle().Foreground(fg).Background(bg)
	bgStyle := lipgloss.NewStyle().Background(bg)

	left := baseStyle.Render(" " + quip)

	padWidth := totalWidth - 1 - len(quip) - iconVisWidth - iconMargin
	if padWidth < 0 {
		padWidth = 0
	}
	pad := bgStyle.Render(strings.Repeat(" ", padWidth))

	if iconVisWidth == 0 {
		return left + pad
	}
	return left + pad + iconStr + bgStyle.Render(" ")
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
