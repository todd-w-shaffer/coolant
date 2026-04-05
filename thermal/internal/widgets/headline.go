package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// Overall thermal gradient — same 5-level scheme as category boxes.
var overallGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold
	{lipgloss.Color("2"), lipgloss.Color("233")},   // cool: green text
	{lipgloss.Color("3"), lipgloss.Color("234")},   // warm: yellow text
	{lipgloss.Color("208"), lipgloss.Color("235")}, // hot: orange text
	{lipgloss.Color("196"), lipgloss.Color("52")},  // critical: red on dark red
}

// agentIcon tracks one breathing agent icon's animation state.
type agentIcon struct {
	alive float64 // spring position: 0→1 fading in, 1→0 fading out
	vel   float64
	phase float64 // breathing phase accumulator (radians)
	dying bool
}

// agentGlyph is the per-icon character rendered in the headline.
const agentGlyph = "◆"

// Headline renders the unified thermal bar:
// [ Claude's humming along  ◆ ◆ ◆ | test:004 | build:008 | run:018 | search:005 | shell:004 ]
type Headline struct {
	width     int
	state     *model.AppState
	spring    harmonica.Spring
	icons     []agentIcon
	nextPhase float64 // monotonic counter for phase offset seeding
}

func NewHeadline() *Headline {
	return &Headline{
		spring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), config.SpringFreq, config.SpringDamping),
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

	target := state.SessionCount

	// Count alive (non-dying) icons
	aliveCount := 0
	for _, ic := range h.icons {
		if !ic.dying {
			aliveCount++
		}
	}

	if target > aliveCount {
		for i := 0; i < target-aliveCount; i++ {
			h.nextPhase += 0.7
			h.icons = append(h.icons, agentIcon{phase: h.nextPhase})
		}
	} else if target < aliveCount {
		// Mark excess alive icons as dying (from the end)
		toKill := aliveCount - target
		for i := len(h.icons) - 1; i >= 0 && toKill > 0; i-- {
			if !h.icons[i].dying {
				h.icons[i].dying = true
				toKill--
			}
		}
	}
}

// AnimTick advances agent icon springs and breathing phases.
func (h *Headline) AnimTick() {
	for i := range h.icons {
		target := 1.0
		if h.icons[i].dying {
			target = 0.0
		}
		h.icons[i].alive, h.icons[i].vel = h.spring.Update(
			h.icons[i].alive, h.icons[i].vel, target,
		)
		// Advance breathing phase only while alive
		if !h.icons[i].dying {
			h.icons[i].phase += config.BreathePhaseStep
		}
	}

	// Remove fully faded icons
	n := 0
	for _, ic := range h.icons {
		if !(ic.dying && ic.alive < config.BreatheFadeEps) {
			h.icons[n] = ic
			n++
		}
	}
	h.icons = h.icons[:n]
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
	iconStr, iconVisWidth := h.renderIcons(iconBg)

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

// renderIcons produces the styled icon string and its visible cell width.
// bg is the headline cell's background — icons layer over it transparently.
func (h *Headline) renderIcons(bg color.Color) (string, int) {
	if len(h.icons) == 0 {
		return "", 0
	}

	var buf strings.Builder
	visWidth := 0
	spacer := lipgloss.NewStyle().Background(bg).Render(" ")

	for i, ic := range h.icons {
		// Breathing: oscillate brightness between min and max via sine wave
		breathT := 0.5 + 0.5*math.Sin(ic.phase)
		brightness := ic.alive * (config.BreatheMinBright + (config.BreatheMaxBright-config.BreatheMinBright)*breathT)
		if brightness < 0 {
			brightness = 0
		}
		if brightness > 1 {
			brightness = 1
		}

		fg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
			uint8(config.BreatheBaseR*brightness),
			uint8(config.BreatheBaseG*brightness),
			uint8(config.BreatheBaseB*brightness),
		))

		if i > 0 {
			buf.WriteString(spacer)
			visWidth++
		}
		buf.WriteString(lipgloss.NewStyle().Foreground(fg).Background(bg).Render(agentGlyph))
		visWidth++
	}

	return buf.String(), visWidth
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
