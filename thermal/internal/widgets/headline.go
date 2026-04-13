package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"
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
	bloom  *HeatBloom
	theme  *theme.Theme

	// pulsePhase is the single meltdown oscillator; the segment readout
	// and any future bar-level throb consume this same phase so the whole
	// headline throbs together.
	pulsePhase float64
	meltdown   bool

	// Directional heat rails for the stacked build/shell cells. Each cell's
	// rail rides CategoryGradient for live warming and eases back to
	// iconBg over BuildShellEmberDecay after its count drops.
	buildRail railState
	shellRail railState

	// now is the wall-clock source for ember decay. Tests inject a fixed
	// clock to capture deterministic mid-decay goldens; production runs
	// leave it defaulted to time.Now.
	now func() time.Time
}

func NewHeadline(th *theme.Theme, ap *anim.Profile) *Headline {
	return &Headline{
		agents: NewBreatheDots(th, ap),
		temp:   NewSegmentReadout(th, ap),
		bloom:  NewHeatBloom(th, ap),
		theme:  th,
		now:    time.Now,
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
	h.bloom.Update(state)
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
	h.bloom.AnimTick()
	if h.meltdown {
		h.pulsePhase += meltdownPhaseStep
		if h.pulsePhase > 2*math.Pi {
			h.pulsePhase -= 2 * math.Pi
		}
	}
}

// rowPair is a 2-row rendered fragment at a fixed visible width. visWidth
// counts cells (post-ANSI), so layers can pad/align deterministically.
type rowPair struct {
	top, bot string
	visWidth int
}

// bgPad returns n cells of iconBg-styled whitespace. Many headline fragments
// need this and re-constructing the lipgloss style inline is noisy.
func bgPad(bg color.Color, n int) string {
	if n <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", n))
}

// bloomedBgPad renders n cells starting at startCol on the given row,
// painting each cell with the HeatBloom's BgAt contribution (falling
// back to iconBg past the bloom's right-boundary or where alpha is zero).
// Emits truecolor bg escapes directly and coalesces equal-color runs so
// the 30fps left-zone repaint doesn't allocate a lipgloss.Style per cell.
func (h *Headline) bloomedBgPad(iconBg color.Color, startCol, n, row int) string {
	if n <= 0 {
		return ""
	}
	var sb strings.Builder
	var prev string
	for i := 0; i < n; i++ {
		c := h.bloom.BgAt(startCol+i, row, iconBg)
		esc := truecolorBg(c)
		if esc != prev {
			if prev != "" {
				sb.WriteString("\x1b[0m")
			}
			sb.WriteString(esc)
			prev = esc
		}
		sb.WriteByte(' ')
	}
	if prev != "" {
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

// truecolorBg emits \033[48;2;R;G;Bm for any color.Color. Kept local to
// the bloom hot path so the 30fps repaint skips lipgloss.NewStyle allocs.
func truecolorBg(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

// renderLCDFrag wraps the segment readout as a rowPair. Returns a zero
// fragment when the headline is offline or the readout is suppressed.
func (h *Headline) renderLCDFrag(iconBg color.Color, pulseScale float64) rowPair {
	if h.state == nil || !h.state.Online {
		return rowPair{}
	}
	top, bot, w := h.temp.RenderWithPulse(iconBg, pulseScale)
	if w == 0 {
		return rowPair{}
	}
	return rowPair{top: top, bot: bot, visWidth: w}
}

// renderSessionsAgentsStack stacks session diamonds (top) over ACTIVE agents
// (bottom). Ghost/stale/KITT agents are NOT in this cell — they render as a
// separate fragment extending leftward under the runtime column so growth
// in the stale tail doesn't push this cell around. Both rows right-anchor
// within the cell so session and active-agent glyphs stay flush against
// the build/shell divider on their right edge.
func (h *Headline) renderSessionsAgentsStack(sessionStr string, sessionWidth int, activeStr string, activeWidth int, iconBg color.Color) rowPair {
	if sessionWidth == 0 && activeWidth == 0 {
		return rowPair{}
	}

	cellWidth := sessionWidth
	if activeWidth > cellWidth {
		cellWidth = activeWidth
	}

	padLeft := func(s string, w int) string {
		if w >= cellWidth {
			return s
		}
		return bgPad(iconBg, cellWidth-w) + s
	}

	return rowPair{
		top:      padLeft(sessionStr, sessionWidth),
		bot:      padLeft(activeStr, activeWidth),
		visWidth: cellWidth,
	}
}

// buildCat and shellCat are resolved once at package init so the per-frame
// render path doesn't linear-scan collector.Categories.
var buildCat, shellCat collector.Category

func init() {
	for _, c := range collector.Categories {
		switch c.Name {
		case "build":
			buildCat = c
		case "shell":
			shellCat = c
		}
	}
}

// buildRailGlyph paints the top edge of the build cell; shellRailGlyph
// paints the bottom edge of the shell cell. One-eighth blocks sit at the
// cell edge bordering the row separator, so the rail reads as a directional
// origin line rather than a box outline.
const (
	buildRailGlyph = '▔' // U+2594 UPPER ONE EIGHTH BLOCK
	shellRailGlyph = '▁' // U+2581 LOWER ONE EIGHTH BLOCK
)

// renderBuildShellStack stacks build:NNN (top) over shell:NNN (bottom).
// The cell backdrop is pinned to iconBg for both rows; activity pressure
// is signalled by a heat-colored rail on each cell's origin edge (top
// for build, bottom for shell). The rail Fg rides CategoryGradient for
// warming and eases back to iconBg over BuildShellEmberDecay when the
// count drops, so a burst reads as a directional ember trail instead of
// a full-cell bg paint that would fight the heatbloom for the same
// visual channel.
func (h *Headline) renderBuildShellStack(smoothed map[string]float64, iconBg color.Color) rowPair {
	now := h.now()
	buildLevel := thermalLevelFor(buildCat.Name, int(math.Round(smoothed[buildCat.Name])))
	shellLevel := thermalLevelFor(shellCat.Name, int(math.Round(smoothed[shellCat.Name])))
	h.buildRail.update(buildLevel, now, config.BuildShellEmberDecay)
	h.shellRail.update(shellLevel, now, config.BuildShellEmberDecay)

	return rowPair{
		top:      renderRailCell(buildCat, smoothed, fixedCellWidth, h.theme, iconBg, h.buildRail, h.buildRail.decayAt(now, config.BuildShellEmberDecay), buildRailGlyph),
		bot:      renderRailCell(shellCat, smoothed, fixedCellWidth, h.theme, iconBg, h.shellRail, h.shellRail.decayAt(now, config.BuildShellEmberDecay), shellRailGlyph),
		visWidth: fixedCellWidth,
	}
}

// renderRailCell renders a build/shell cell with iconBg across the full
// width, pinned-Fg text for "name:NNN", and rail glyphs in the padding
// slots. Rail Fg comes from railColor(th, peakLevel, iconBg, decay) so
// idle cells emit a row of iconBg-on-iconBg glyphs (invisible) and peak
// cells emit CategoryGradient[level].Fg. Text color is pinned to
// CategoryGradient[1].Fg (calm baseline) so legibility is constant
// regardless of heat level — the old path had digits go dark-red on
// dark-red at critical.
func renderRailCell(cat collector.Category, smoothed map[string]float64,
	cellWidth int, th *theme.Theme, iconBg color.Color, rs railState,
	decay float64, edge rune) string {
	s := smoothed[cat.Name]
	count := int(math.Round(s))

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

	railFg := railColor(th, rs.peakLevel, iconBg, decay)
	railStyle := lipgloss.NewStyle().Foreground(railFg).Background(iconBg)
	textStyle := lipgloss.NewStyle().
		Foreground(th.CategoryGradient[1].Fg).
		Background(iconBg)

	edgeRun := strings.Repeat(string(edge), padLeft)
	trailRun := strings.Repeat(string(edge), padRight)

	return railStyle.Render(edgeRun) + textStyle.Render(content) + railStyle.Render(trailRun)
}

// fixedCellWidth is the compact width for always-visible category boxes.
// "build:002" = 9 chars + 1 padding each side = 11
const fixedCellWidth = 11

// headlineRightMargin reserves trailing bg cells on both rows so styled
// glyphs at the right edge (the LCD's degree sign today, anything else
// tomorrow) have unstyled slack before the terminal column.
const headlineRightMargin = 2

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

// dynamicCellWidth is the width of each dynamic runtime cell on the bottom
// row of the left zone.
const dynamicCellWidth = 8

// ViewLines returns the headline as a 2-line slice. Layout (online):
//
//	[quip ...........................]  [      ] [⌬ session] [build:000] [LCD]
//	[                    runtime cells]  [      ] [⬡ agent  ] [shell:001] [LCD]
//
// Right-aligned cluster (sessions/agents → build/shell → LCD) stays pinned;
// runtimes appear/disappear as the only dynamic churn, growing leftward into
// the quip pad. Offline collapses to a quip-only top row with bg-filled bot.
func (h *Headline) ViewLines() []string {
	if h.state == nil {
		return []string{""}
	}
	if !h.state.Online {
		return h.offlineViewLines()
	}

	// Pin the headline backdrop to the calm-baseline bg — temperature is now
	// communicated by the thermographic bloom alone. Letting iconBg drift
	// across OverallGradient levels fought the bloom for the same channel.
	iconBg := h.theme.OverallGradient[1].Bg

	pulseScale := 1.0
	if h.meltdown {
		pulseScale = 0.6 + 0.4*(math.Sin(h.pulsePhase)+1)/2
	}
	lcd := h.renderLCDFrag(iconBg, pulseScale)

	buildShell := h.renderBuildShellStack(h.state.SmoothedCats, iconBg)

	var sessions []collector.SessionTree
	if h.state.Current != nil {
		sessions = h.state.Current.Sessions
	}
	sessionStr, sessionWidth := renderSessionDiamonds(sessions, iconBg, h.theme)
	ghostStr, activeStr, ghostWidth, activeWidth := h.agents.RenderSplit(ui.AgentGlyphHollow, ui.AgentGlyphMid, ui.AgentGlyphFilled, iconBg, 0)
	sessAgents := h.renderSessionsAgentsStack(sessionStr, sessionWidth, activeStr, activeWidth, iconBg)

	var dynamic []collector.Category
	for _, cat := range collector.Categories {
		if !collector.FixedCategories[cat.Name] && h.state.SmoothedCats[cat.Name] >= 0.5 {
			dynamic = append(dynamic, cat)
		}
	}
	runtimeWidth := len(dynamic) * dynamicCellWidth

	bgStyle := lipgloss.NewStyle().Background(iconBg)
	divider := bgStyle.Render(" ")

	// Compose right cluster with single-space dividers. The bot row of the
	// first (sessions/agents) fragment is tracked separately so it can be
	// absorbed into the ghost trail when no active agents exist — ghosts
	// then end flush under the sessions column instead of floating at its
	// left edge.
	var rightTop, rightBot strings.Builder
	rightVis := 0
	appendFrag := func(f rowPair) {
		if f.visWidth == 0 {
			return
		}
		if rightVis > 0 {
			rightTop.WriteString(divider)
			rightBot.WriteString(divider)
			rightVis++
		}
		rightTop.WriteString(f.top)
		rightBot.WriteString(f.bot)
		rightVis += f.visWidth
	}
	appendFrag(sessAgents)
	appendFrag(buildShell)
	appendFrag(lcd)

	// Ghost trail absorbs the stack cell's left-slack on the bot row so
	// ghost→active reads as one continuous ribbon. Zero when actives meet
	// or exceed sessions; equals sessionWidth in the full-absorb case.
	absorbWidth := sessionWidth - activeWidth
	if absorbWidth < 0 {
		absorbWidth = 0
	}

	// Left combined width = quip zone + runtime zone, sharing the bot row for
	// the ghost tail. Ghosts right-anchor within this width so they visually
	// extend leftward from the stack cell under the runtimes.
	leftCombined := h.width - rightVis - headlineRightMargin
	if rightVis > 0 {
		leftCombined--
	}
	if leftCombined < 0 {
		leftCombined = 0
	}
	leftWidth := leftCombined - runtimeWidth
	if leftWidth < 0 {
		leftWidth = 0
	}

	// Configure the bloom for this frame's left-zone dimensions. Width is
	// the full left-combined area so the bloom's right-boundary guard can
	// reason about the same coordinate space the ghost ribbon inhabits.
	h.bloom.SetSize(leftCombined, 2)

	// Top-row-left is intentionally blank text — the thermographic bloom
	// paints cell-varying backgrounds here as the dashboard's atmospheric
	// accent. Future content overlays will compose over it.
	leftTop := ""
	if leftWidth > 0 {
		leftTop = h.bloomedBgPad(iconBg, 0, leftWidth, 0)
	}

	var runtimeCells strings.Builder
	for _, cat := range dynamic {
		runtimeCells.WriteString(renderCatCell(cat, h.state.SmoothedCats, dynamicCellWidth, h.theme))
	}
	runtimeTop := runtimeCells.String()

	// Ghost tail right-anchored. When absorbing, the area extends right by
	// sep + absorbWidth; one of those cells is reserved as a ribbon
	// separator when both halves are present so their glyphs don't
	// collide. Overflow silently extends leftward.
	needRibbonSep := absorbWidth > 0 && ghostWidth > 0 && activeWidth > 0
	ghostArea := leftCombined
	if absorbWidth > 0 {
		ghostArea += 1 + absorbWidth
		if needRibbonSep {
			ghostArea--
		}
	}
	ghostPadLeft := ghostArea - ghostWidth
	if ghostPadLeft < 0 {
		ghostPadLeft = 0
	}
	botLeft := h.bloomedBgPad(iconBg, 0, ghostPadLeft, 1) + ghostStr

	sep := ""
	if rightVis > 0 {
		sep = divider
	}

	topLine := leftTop + runtimeTop + sep + rightTop.String()

	// When absorbing, active glyphs render directly after the ghost trail
	// (no divider — same ribbon) with an optional single-cell separator,
	// then the remaining fragments append via rebuildBotRight.
	var botLine string
	if absorbWidth > 0 {
		ribbonSep := ""
		if needRibbonSep {
			ribbonSep = bgPad(iconBg, 1)
		}
		botLine = botLeft + ribbonSep + activeStr + h.rebuildBotRight(buildShell, lcd, divider)
	} else {
		botLine = botLeft + sep + rightBot.String()
	}

	margin := bgPad(iconBg, headlineRightMargin)
	topLine += margin
	botLine += margin
	return []string{topLine, botLine}
}

// rebuildBotRight composes the bot row's right-cluster starting from the
// buildShell fragment (used when the sessAgents cell is absorbed or
// partially absorbed into the ghost trail). Each fragment is preceded
// by a divider — the first is the ghost-area/right-cluster boundary.
func (h *Headline) rebuildBotRight(buildShell, lcd rowPair, divider string) string {
	var sb strings.Builder
	write := func(f rowPair) {
		if f.visWidth == 0 {
			return
		}
		sb.WriteString(divider)
		sb.WriteString(f.bot)
	}
	write(buildShell)
	write(lcd)
	return sb.String()
}

// offlineViewLines renders the quip-only offline mode: top row has the
// offline message on offline bg, bottom row is bg-filled to preserve the
// no-reflow 2-row contract.
func (h *Headline) offlineViewLines() []string {
	iconBg := h.theme.OfflineBg
	fg := h.theme.OfflineFg
	quip := model.OfflineMessage(h.state.OfflineDuration, h.state.IdleCycle)

	maxQuip := h.width - 1
	if maxQuip < 0 {
		maxQuip = 0
	}
	if utf8.RuneCountInString(quip) > maxQuip {
		quip = truncRunes(quip, maxQuip)
	}
	quipWidth := utf8.RuneCountInString(quip)

	quipStyle := lipgloss.NewStyle().Foreground(fg).Background(iconBg)

	padW := h.width - 1 - quipWidth
	if padW < 0 {
		padW = 0
	}
	topLine := quipStyle.Render(" "+quip) + bgPad(iconBg, padW)
	botLine := bgPad(iconBg, h.width)
	return []string{topLine, botLine}
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
