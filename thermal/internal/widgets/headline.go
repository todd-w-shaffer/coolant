package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// meltdownPhaseStep advances the meltdown pulse phase per AnimTick for a
// 1 Hz oscillation at the project's AnimFPS cadence.
var meltdownPhaseStep = 2 * math.Pi / float64(config.AnimFPS)

// Headline renders the unified thermal bar. It is a 2-row strip when online
// (top row: quip + LCD readout + agent icons + session diamonds + category
// cells; bottom row: readout continuation) and a 1-row strip when offline
// or idle.
type Headline struct {
	width  int
	state  *model.AppState
	agents *BreatheDots
	temp   *SegmentReadout
	theme  *theme.Theme

	// pulsePhase is the single meltdown oscillator; the segment readout
	// and any future bar-level throb consume this same phase so the whole
	// headline throbs together.
	pulsePhase float64
	meltdown   bool
}

func NewHeadline(th *theme.Theme, ap *anim.Profile) *Headline {
	return &Headline{
		agents: NewBreatheDots(th, ap),
		temp:   NewSegmentReadout(th, ap),
		theme:  th,
	}
}

func (h *Headline) SetSize(w, height int) {
	h.width = w
}

func (h *Headline) Update(state *model.AppState) {
	h.state = state
	if state == nil {
		h.meltdown = false
		return
	}
	h.agents.SetTarget(state.AgentCount())
	h.agents.SetStaleCount(state.StaleAgentCount())
	h.agents.SetCompletedCount(state.CompletedAgentCount())
	h.meltdown = state.Online && state.ThreatLevel == model.ThreatMeltdown

	// Drive the readout from data-update cadence, not render cadence. If we
	// updated inside ViewLines() the ghost trail would re-arm on every
	// sub-second oscillation and the readout would stay permanently dimmed.
	if state.Online {
		h.temp.Update(model.OverallTemperature(state), threatToThermal(state.ThreatLevel))
	}
}

// SetHighScoreMode toggles KITT-as-highscore on the agent dot display.
func (h *Headline) SetHighScoreMode(on bool) {
	h.agents.SetHighScoreMode(on)
}

// AnimTick advances agent icon springs, the readout's ghost/flash
// countdowns, and (during meltdown) the single bar-wide pulse phase.
func (h *Headline) AnimTick() {
	h.agents.AnimTick()
	h.temp.AnimTick()
	if h.meltdown {
		h.pulsePhase += meltdownPhaseStep
		if h.pulsePhase > 2*math.Pi {
			h.pulsePhase -= 2 * math.Pi
		}
	}
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

// View returns the headline's rendered strip, with rows joined by '\n' when
// in 2-row mode. Callers that need per-row access use ViewLines.
func (h *Headline) View() string {
	return strings.Join(h.ViewLines(), "\n")
}

// ViewLines returns the headline as a slice of rendered rows — 2 rows online
// and active, 1 row offline. The 2-row form is additive: the top row still
// shows everything the 1-row legacy form showed; the bottom row paints the
// LCD readout's lower half and bg-fills the remainder.
func (h *Headline) ViewLines() []string {
	if h.state == nil {
		return []string{""}
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

	fixedTotalWidth := len(fixed) * fixedCellWidth
	dynamicCount := len(dynamic)
	dynamicCellWidth := 0
	if dynamicCount > 0 {
		dynamicCellWidth = 8
	}
	dynamicTotalWidth := dynamicCount * dynamicCellWidth

	overallWidth := h.width - fixedTotalWidth - dynamicTotalWidth
	if overallWidth < 20 {
		overallWidth = 20
	}

	twoRow := h.state.Online

	var iconBg, fg color.Color
	var quip string
	level := 0
	if !h.state.Online {
		iconBg = h.theme.OfflineBg
		fg = h.theme.OfflineFg
		quip = model.OfflineMessage(h.state.OfflineDuration, h.state.IdleCycle)
	} else {
		level = threatToThermal(h.state.ThreatLevel)
		iconBg = h.theme.OverallGradient[level].Bg
		fg = h.theme.OverallGradient[level].Fg
		quip = h.state.StableQuip()
	}

	iconStr, iconVisWidth := h.agents.Render(ui.AgentGlyphHollow, ui.AgentGlyphMid, ui.AgentGlyphFilled, iconBg, 0)

	var sessions []collector.SessionTree
	if h.state.Current != nil {
		sessions = h.state.Current.Sessions
	}
	sessionStr, sessionVisWidth := renderSessionDiamonds(sessions, iconBg, h.theme)

	tempTop, tempBot, tempVisWidth := "", "", 0
	if twoRow {
		pulseScale := 1.0
		if h.meltdown {
			pulseScale = 0.6 + 0.4*(math.Sin(h.pulsePhase)+1)/2
		}
		tempTop, tempBot, tempVisWidth = h.temp.RenderWithPulse(iconBg, pulseScale)
	}

	topCell, botCell := h.buildOverallCell(quip, fg, iconBg,
		tempTop, tempBot, tempVisWidth,
		iconStr, iconVisWidth, sessionStr, sessionVisWidth, overallWidth)

	var dynamicCells []string
	for _, cat := range dynamic {
		dynamicCells = append(dynamicCells, renderCatCell(cat, h.state.SmoothedCats, dynamicCellWidth, h.theme))
	}
	var fixedCells []string
	for _, cat := range fixed {
		fixedCells = append(fixedCells, renderCatCell(cat, h.state.SmoothedCats, fixedCellWidth, h.theme))
	}
	catsTop := strings.Join(dynamicCells, "") + strings.Join(fixedCells, "")
	catsWidth := dynamicTotalWidth + fixedTotalWidth

	// Always emit two rows so the downstream layout never reflows when the
	// LCD flashes off (e.g., demo's offline cycles). When the readout is
	// hidden, botCell is empty and the bottom row is just bg-filled space
	// of the same width as the overall cell, plus plain-space padding for
	// the category region.
	topLine := topCell + catsTop
	if botCell == "" {
		bgStyle := lipgloss.NewStyle().Background(iconBg)
		botCell = bgStyle.Render(strings.Repeat(" ", overallWidth))
	}
	botLine := botCell + strings.Repeat(" ", catsWidth)
	return []string{topLine, botLine}
}

// buildOverallCell builds the quip/readout/icons/sessions zone and returns
// the top and bottom rows of the overall cell. botLine is "" in 1-row
// (offline) mode.
func (h *Headline) buildOverallCell(quip string, fg, bg color.Color,
	tempTop, tempBot string, tempVisWidth int,
	iconStr string, iconVisWidth int,
	sessionStr string, sessionVisWidth int, totalWidth int) (topLine, botLine string) {
	tempMargin := 0
	if tempVisWidth > 0 {
		tempMargin = 1
	}
	iconMargin := 0
	if iconVisWidth > 0 {
		iconMargin = 1
	}
	sessionMargin := 0
	if sessionVisWidth > 0 {
		sessionMargin = 1
	}

	rightWidth := tempVisWidth + tempMargin + iconVisWidth + iconMargin + sessionVisWidth + sessionMargin
	maxQuip := totalWidth - 2 - rightWidth
	if maxQuip < 0 {
		maxQuip = 0
	}
	// Quip widths must be counted in runes, not bytes — offline messages
	// contain em-dashes (3 bytes, 1 cell) and a byte count would throw
	// the pad math off by 2 cells per multi-byte rune.
	if utf8.RuneCountInString(quip) > maxQuip {
		quip = truncRunes(quip, maxQuip)
	}
	quipWidth := utf8.RuneCountInString(quip)

	baseStyle := lipgloss.NewStyle().Foreground(fg).Background(bg)
	bgStyle := lipgloss.NewStyle().Background(bg)

	left := baseStyle.Render(" " + quip)
	padWidth := totalWidth - 1 - quipWidth - rightWidth
	if padWidth < 0 {
		padWidth = 0
	}
	pad := bgStyle.Render(strings.Repeat(" ", padWidth))

	var right strings.Builder
	if tempVisWidth > 0 {
		right.WriteString(tempTop)
		right.WriteString(bgStyle.Render(" "))
	}
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

	topLine = left + pad + right.String()
	if tempVisWidth == 0 {
		return topLine, ""
	}

	leftBotWidth := 1 + quipWidth + padWidth
	rightAfterTemp := rightWidth - tempVisWidth // trailing bg after the readout
	botLine = bgStyle.Render(strings.Repeat(" ", leftBotWidth)) + tempBot + bgStyle.Render(strings.Repeat(" ", rightAfterTemp))
	return topLine, botLine
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
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Background(bg).Render(ui.SessionDiamondGlyph))
		visWidth++
	}
	return sb.String(), visWidth
}

// truncRunes returns s truncated to at most n runes (not bytes) so
// multibyte characters don't get split mid-codepoint.
func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i, count := 0, 0
	for ; i < len(s) && count < n; count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
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
