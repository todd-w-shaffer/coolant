package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// Headline renders the unified thermal bar:
// [ Claude's humming along  ◆ ◆ ◆ | test:004 | build:008 | run:018 | search:005 | shell:004 ]
type Headline struct {
	width  int
	state  *model.AppState
	agents *BreatheDots
	theme  *theme.Theme
}

func NewHeadline(th *theme.Theme) *Headline {
	return &Headline{
		agents: NewBreatheDots(th),
		theme:  th,
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
	h.agents.SetStaleCount(state.StaleAgentCount())
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
func renderCatCell(cat collector.Category, smoothed map[string]float64, cellWidth int, th *theme.Theme) string {
	s := smoothed[cat.Name]
	count := int(math.Round(s))
	level := thermalLevelFor(cat.Name, count)
	thermal := th.CategoryGradient[level]

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
		Foreground(thermal.Fg).
		Background(thermal.Bg).
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
		iconBg = h.theme.OfflineBg
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		iconBg = h.theme.OverallGradient[overallLevel].Bg
	}
	iconStr, iconVisWidth := h.agents.Render(ui.AgentGlyphHollow, ui.AgentGlyphFilled, iconBg, 0)

	// Render session phase diamonds
	var sessions []collector.SessionTree
	if h.state.Current != nil {
		sessions = h.state.Current.Sessions
	}
	sessionStr, sessionVisWidth := renderSessionDiamonds(sessions, iconBg, h.theme)

	// Build overall cell
	var overallCell string
	if !h.state.Online {
		quip := model.OfflineMessage(h.state.OfflineDuration, h.state.IdleCycle)
		overallCell = h.buildOverallCell(quip, h.theme.OfflineFg, h.theme.OfflineBg, iconStr, iconVisWidth, sessionStr, sessionVisWidth, overallWidth)
	} else {
		overallLevel := threatToThermal(h.state.ThreatLevel)
		overallThermal := h.theme.OverallGradient[overallLevel]
		quip := h.state.StableQuip()
		overallCell = h.buildOverallCell(quip, overallThermal.Fg, overallThermal.Bg, iconStr, iconVisWidth, sessionStr, sessionVisWidth, overallWidth)
	}

	// Render dynamic cells (left of fixed, grow leftward)
	var dynamicCells []string
	for _, cat := range dynamic {
		dynamicCells = append(dynamicCells, renderCatCell(cat, h.state.SmoothedCats, dynamicCellWidth, h.theme))
	}

	// Render fixed cells (right-anchored, compact)
	var fixedCells []string
	for _, cat := range fixed {
		fixedCells = append(fixedCells, renderCatCell(cat, h.state.SmoothedCats, fixedCellWidth, h.theme))
	}

	return overallCell + strings.Join(dynamicCells, "") + strings.Join(fixedCells, "")
}

// buildOverallCell constructs the overall headline cell with quip left-aligned,
// agent icons and session diamonds right-aligned, all sharing the same background.
func (h *Headline) buildOverallCell(quip string, fg, bg color.Color, iconStr string, iconVisWidth int, sessionStr string, sessionVisWidth int, totalWidth int) string {
	// Margins between sections
	iconMargin := 0
	if iconVisWidth > 0 {
		iconMargin = 1
	}
	sessionMargin := 0
	if sessionVisWidth > 0 {
		sessionMargin = 1
	}

	rightWidth := iconVisWidth + iconMargin + sessionVisWidth + sessionMargin
	maxQuip := totalWidth - 2 - rightWidth
	if maxQuip < 0 {
		maxQuip = 0
	}
	if len(quip) > maxQuip {
		quip = quip[:maxQuip]
	}

	baseStyle := lipgloss.NewStyle().Foreground(fg).Background(bg)
	bgStyle := lipgloss.NewStyle().Background(bg)

	left := baseStyle.Render(" " + quip)

	padWidth := totalWidth - 1 - len(quip) - rightWidth
	if padWidth < 0 {
		padWidth = 0
	}
	pad := bgStyle.Render(strings.Repeat(" ", padWidth))

	var right strings.Builder
	if iconVisWidth > 0 {
		right.WriteString(iconStr)
	}
	if sessionVisWidth > 0 {
		if iconVisWidth > 0 {
			right.WriteString(bgStyle.Render(" "))
		}
		right.WriteString(sessionStr)
	}
	if iconVisWidth > 0 || sessionVisWidth > 0 {
		right.WriteString(bgStyle.Render(" "))
	}

	return left + pad + right.String()
}

// renderSessionDiamonds renders phase-colored ⌬ icons for each session,
// placed on the given background. Returns the rendered string and its visual width.
func renderSessionDiamonds(sessions []collector.SessionTree, bg color.Color, th *theme.Theme) (string, int) {
	if len(sessions) == 0 {
		return "", 0
	}

	groups := sessionGroupCounts(sessions)
	bgStyle := lipgloss.NewStyle().Background(bg)

	var sb strings.Builder
	visWidth := 0
	for i, g := range groups {
		if i > 0 {
			sb.WriteString(bgStyle.Render(" "))
			visWidth++
		}
		var c color.Color
		if g.total() == 0 {
			c = th.SessionPhase.Idle
		} else {
			c = sessionPhaseColor(&g, th)
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Background(bg).Render("⌬"))
		visWidth++
	}
	return sb.String(), visWidth
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
