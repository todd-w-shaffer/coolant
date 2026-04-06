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

// fixedCellWidth is the compact width for always-visible category boxes.
// "build:002" = 9 chars + 1 padding each side = 11
const fixedCellWidth = 11

// visibleCategories returns categories that should appear in the headline:
// fixed categories (build, shell) always, dynamic runtimes only when count >= 0.5.
func visibleCategories(smoothed map[string]float64) []collector.Category {
	var visible []collector.Category
	for _, cat := range collector.Categories {
		if collector.FixedCategories[cat.Name] || smoothed[cat.Name] >= 0.5 {
			visible = append(visible, cat)
		}
	}
	return visible
}

// renderCatCell renders a single category box at the given width.
// Fixed categories show "name:NNN", dynamic runtimes show just "name".
func renderCatCell(cat collector.Category, smoothed map[string]float64, cellWidth int) string {
	s := smoothed[cat.Name]
	count := int(math.Round(s))
	level := thermalLevelFor(cat.Name, count)
	thermal := thermalGradient[level]

	var content string
	if collector.FixedCategories[cat.Name] {
		content = fmt.Sprintf("%s:%03d", cat.Label, count)
	} else {
		content = cat.Label
	}

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
	return lipgloss.NewStyle().
		Foreground(thermal.fg).
		Background(thermal.bg).
		Render(padded)
}

func (h *Headline) View() string {
	if h.state == nil {
		return ""
	}

	// Split categories into dynamic (left) and fixed (right-anchored)
	var dynamic, fixed []collector.Category
	for _, cat := range collector.Categories {
		if collector.FixedCategories[cat.Name] {
			fixed = append(fixed, cat)
		} else if h.state.SmoothedCats[cat.Name] >= 0.5 {
			dynamic = append(dynamic, cat)
		}
	}

	// Layout: [quip + agents | ...dynamic... | fixed | fixed ]
	// Fixed cells are compact and right-anchored
	fixedTotalWidth := len(fixed) * fixedCellWidth
	dynamicCount := len(dynamic)

	// Overall cell gets what's left after fixed and dynamic
	// Dynamic cells are compact — just the label, no count (e.g. " node " = 8 chars)
	dynamicCellWidth := 0
	if dynamicCount > 0 {
		dynamicCellWidth = 8
	}
	dynamicTotalWidth := dynamicCount * dynamicCellWidth

	overallWidth := h.width - fixedTotalWidth - dynamicTotalWidth
	if overallWidth < 20 {
		overallWidth = 20
	}

	// Render agent icons
	var iconBg color.Color
	if !h.state.Online {
		iconBg = lipgloss.Color("67")
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		iconBg = overallGradient[overallLevel].bg
	}
	iconStr, iconVisWidth := h.agents.Render(ui.SessionGlyph, iconBg, 0)

	// Build overall cell
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

	// Render dynamic cells (left of fixed, grow leftward)
	var dynamicCells []string
	for _, cat := range dynamic {
		dynamicCells = append(dynamicCells, renderCatCell(cat, h.state.SmoothedCats, dynamicCellWidth))
	}

	// Render fixed cells (right-anchored, compact)
	var fixedCells []string
	for _, cat := range fixed {
		fixedCells = append(fixedCells, renderCatCell(cat, h.state.SmoothedCats, fixedCellWidth))
	}

	return overallCell + strings.Join(dynamicCells, "") + strings.Join(fixedCells, "")
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
