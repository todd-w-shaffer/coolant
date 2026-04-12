package widgets

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// TestHeadline_ViewLinesTwoRowsWhenOnline — online+active state returns two
// lines of equal visible width, so the headline paints a 2-row strip.
func TestHeadline_ViewLinesTwoRowsWhenOnline(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	lines := h.ViewLines()
	if len(lines) != 2 {
		t.Fatalf("online active: got %d line(s), want 2", len(lines))
	}
	w0 := ansi.StringWidth(lines[0])
	w1 := ansi.StringWidth(lines[1])
	if w0 != w1 {
		t.Errorf("row widths differ: top=%d bot=%d", w0, w1)
	}
	if w0 == 0 {
		t.Errorf("top row empty")
	}
}

// TestHeadline_ViewLinesAlwaysTwoRows — offline path still returns two rows
// so the layout below never reflows when the readout flashes off. The
// bottom row is bg-filled space but present.
func TestHeadline_ViewLinesAlwaysTwoRows(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	state := fixtureState()
	state.Online = false
	h.Update(state)

	lines := h.ViewLines()
	if len(lines) != 2 {
		t.Errorf("offline: got %d line(s), want 2 (no-reflow contract)", len(lines))
	}
	if ansi.StringWidth(lines[0]) != ansi.StringWidth(lines[1]) {
		t.Errorf("offline: row widths differ top=%d bot=%d",
			ansi.StringWidth(lines[0]), ansi.StringWidth(lines[1]))
	}
}

// TestHeadline_BuildAboveShell — in the restacked layout, build lives on
// the top row and shell on the bottom row (one stacked cell, not two
// side-by-side cells).
func TestHeadline_BuildAboveShell(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	lines := h.ViewLines()
	top, bot := ansi.Strip(lines[0]), ansi.Strip(lines[1])
	if !strings.Contains(top, "build:") {
		t.Errorf("top row missing build:\n%s", top)
	}
	if strings.Contains(top, "shell:") {
		t.Errorf("top row should NOT contain shell: (moved to bot)\n%s", top)
	}
	if !strings.Contains(bot, "shell:") {
		t.Errorf("bot row missing shell:\n%s", bot)
	}
	if strings.Contains(bot, "build:") {
		t.Errorf("bot row should NOT contain build: (lives on top)\n%s", bot)
	}
}

// TestHeadline_LCDOnFarRight — online fixture renders the LCD fragment as
// the rightmost visible content on both rows (no trailing bg-padded
// runtime cells after it).
func TestHeadline_LCDOnFarRight(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	// Pump a few ticks so the segment readout settles.
	for i := 0; i < 4; i++ {
		h.AnimTick()
	}

	lines := h.ViewLines()
	top := ansi.Strip(lines[0])
	// The temperature value is 2 digits 0-99; confirm a digit bitmap
	// pixel is in the final ~12 columns of the visible top row.
	if len(top) < 12 {
		t.Fatalf("top row too short: %q", top)
	}
	// Build / shell labels must not appear after the LCD — they are
	// the second-rightmost cluster, so "build:" should be to the
	// LEFT of any LCD braille pixel.
	lastBuild := strings.LastIndex(top, "build:")
	if lastBuild == -1 {
		t.Fatalf("expected build: on top row: %q", top)
	}
	trailing := top[lastBuild+len("build:"):]
	// After "build:NNN" there should be LCD content and nothing else
	// in the label family: no "shell:" and no runtime labels.
	if strings.Contains(trailing, "shell:") {
		t.Errorf("shell: should not appear after build: on top row\n%s", top)
	}
}

// TestHeadline_RuntimesOnTopRow — dynamic runtime labels appear on the top
// row, adjacent to the sessions cluster. Bottom-row bg below them is where
// the ghost KITT trail lives.
func TestHeadline_RuntimesOnTopRow(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	state := fixtureState()
	state.SmoothedCats["node"] = 5
	h.Update(state)

	lines := h.ViewLines()
	top, bot := ansi.Strip(lines[0]), ansi.Strip(lines[1])
	if !strings.Contains(top, "node") {
		t.Errorf("top row missing node runtime: %q", top)
	}
	if strings.Contains(bot, "node") {
		t.Errorf("bot row should NOT contain node (moved to top): %q", bot)
	}
}

// TestHeadline_GhostsDontPushActiveAgents — when the ghost/KITT tail is
// large, the right cluster (and therefore active agents pinned against its
// left edge) must not shift. Regression: previously the session/agent cell
// grew to accommodate ghosts, shoving everything leftward across the screen.
func TestHeadline_GhostsDontPushActiveAgents(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	// Screen column of "shell:" on the bot row — the anchor for the
	// active-agents cell that lives one divider-space to its left.
	shellCol := func() int {
		stripped := ansi.Strip(h.ViewLines()[1])
		idx := strings.Index(stripped, "shell:")
		if idx < 0 {
			return -1
		}
		return utf8.RuneCountInString(stripped[:idx])
	}

	// Baseline: 2 active agents, no stales.
	h.agents.SetTarget(2)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}
	baselineCol := shellCol()
	if baselineCol < 0 {
		t.Fatalf("baseline: missing shell: anchor")
	}

	// Stressed: active count unchanged, 10 stale dots dumped into KITT.
	h.agents.SetTarget(12)
	h.agents.SetStaleCount(10)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}
	stressedCol := shellCol()
	if stressedCol != baselineCol {
		t.Errorf("right cluster drifted: shell col baseline=%d stressed=%d", baselineCol, stressedCol)
	}
}

// TestHeadline_GhostsAlignUnderSessionsWhenNoActives — when active agents
// go to zero but stale/KITT ghosts remain, the ghost trail's right edge
// must sit flush under the sessions column (absorbing the empty stack
// cell) instead of floating at its left edge with a gap.
func TestHeadline_GhostsAlignUnderSessionsWhenNoActives(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	// Force the target state: 0 actives, 3 stale/ghost dots. SetTarget
	// spawns the dot slots; SetStaleCount marks them all stale so the
	// active/ghost split returns activeWidth=0, ghostWidth=3.
	h.agents.SetTarget(3)
	h.agents.SetStaleCount(3)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}

	lines := h.ViewLines()
	top := ansi.Strip(lines[0])
	bot := ansi.Strip(lines[1])

	// Sessions column on top row.
	sessCol := strings.IndexRune(top, '⌬')
	if sessCol < 0 {
		t.Fatalf("top row missing session diamond ⌬: %q", top)
	}
	// Convert byte index to rune column.
	sessRuneCol := utf8.RuneCountInString(top[:sessCol])

	// Rightmost ghost glyph on bot row.
	rightmostGhost := -1
	col := 0
	for _, r := range bot {
		if r == '⬡' || r == '⏣' || r == '⬢' {
			rightmostGhost = col
		}
		col++
	}
	if rightmostGhost < 0 {
		t.Fatalf("bot row missing ghost glyph (precondition: stale dots should render): %q", bot)
	}

	if rightmostGhost != sessRuneCol {
		t.Errorf("ghost right-edge not aligned under sessions column:\n"+
			"  ghost rightmost col = %d\n"+
			"  sessions  ⌬    col = %d\n"+
			"  top: %q\n  bot: %q",
			rightmostGhost, sessRuneCol, top, bot)
	}
}

// TestHeadline_GhostsFlowIntoPartialSessionSlack — when sessions outnumber
// active agents, the active row has left-slack inside the stack cell. The
// ghost trail must absorb that slack so ghost+active render as one
// contiguous ribbon on the bot row (no interior gap), matching the
// full-absorb behavior that already fires at activeWidth==0.
func TestHeadline_GhostsFlowIntoPartialSessionSlack(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)

	// Build a 4-session snapshot so sessionWidth (=4) exceeds the single
	// active agent's width (=1). Combined with 2 ghosts this produces a
	// 3-glyph bot ribbon that should render contiguously.
	state := model.NewAppState()
	state.Update(collector.Snapshot{
		Sessions: []collector.SessionTree{
			{RootPID: 1000, RootComm: "claude"},
			{RootPID: 2000, RootComm: "claude"},
			{RootPID: 3000, RootComm: "claude"},
			{RootPID: 4000, RootComm: "claude"},
		},
		Online: true,
	})
	h.Update(state)

	// 3 dots total, 2 of them stale → 1 active + 2 ghosts.
	h.agents.SetTarget(3)
	h.agents.SetStaleCount(2)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}

	lines := h.ViewLines()
	bot := ansi.Strip(lines[1])

	isGlyph := func(r rune) bool {
		return string(r) == ui.AgentGlyphHollow ||
			string(r) == ui.AgentGlyphMid ||
			string(r) == ui.AgentGlyphFilled
	}
	runes := []rune(bot)
	first, last := -1, -1
	glyphCount := 0
	for i, r := range runes {
		if isGlyph(r) {
			if first < 0 {
				first = i
			}
			last = i
			glyphCount++
		}
	}
	if glyphCount != 3 {
		t.Fatalf("expected 3 glyphs (2 ghost + 1 active), got %d: %q", glyphCount, bot)
	}

	// Ribbon must be exactly "G S G S G" — alternating glyph/space, where
	// S is a single bg cell. This catches both bg-padding gaps (too wide)
	// and missing separators between ghost-right and active-left (glyphs
	// visually colliding).
	span := runes[first : last+1]
	want := 2*glyphCount - 1
	if len(span) != want {
		t.Fatalf("ribbon span len=%d want %d (expected alternating G/S/G/S/G): %q\nbot: %q",
			len(span), want, string(span), bot)
	}
	for i, r := range span {
		if i%2 == 0 && !isGlyph(r) {
			t.Errorf("span idx %d expected glyph, got %q: %q", i, r, string(span))
		}
		if i%2 == 1 && r != ' ' {
			t.Errorf("span idx %d expected separator space, got %q: %q", i, r, string(span))
		}
	}
}

// TestHeadline_SessionsAboveAgents — sessions diamonds on top row, agent
// hex glyphs on bottom row, at approximately the same column range.
func TestHeadline_SessionsAboveAgents(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())
	h.agents.SetTarget(3)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}

	lines := h.ViewLines()
	top, bot := ansi.Strip(lines[0]), ansi.Strip(lines[1])
	if !strings.ContainsRune(top, '⌬') {
		t.Errorf("top row missing session diamond ⌬:\n%s", top)
	}
	hasAgentGlyph := strings.ContainsRune(bot, '⬡') ||
		strings.ContainsRune(bot, '⏣') ||
		strings.ContainsRune(bot, '⬢')
	if !hasAgentGlyph {
		t.Errorf("bot row missing agent hex glyph:\n%s", bot)
	}
}

// TestHeadline_MeltdownPulseDrivesModulation — at meltdown, successive
// AnimTicks must change the rendered output. This proves the pulse phase
// is owned at Headline (single oscillator) and actually reaches the
// segment readout's fg color.
func TestHeadline_MeltdownPulseDrivesModulation(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)

	state := fixtureState()
	state.ThreatLevel = model.ThreatMeltdown
	h.Update(state)

	frames := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		lines := h.ViewLines()
		frames = append(frames, lines[0])
		h.AnimTick()
	}
	distinct := map[string]bool{}
	for _, f := range frames {
		distinct[f] = true
	}
	if len(distinct) < 2 {
		t.Errorf("meltdown pulse produced %d distinct top frames across 8 ticks, want >=2", len(distinct))
	}
}

// TestRenderLCDFrag_OfflineZeroWidth — when headline is offline the LCD
// fragment is suppressed: empty rows and zero visible width.
func TestRenderLCDFrag_OfflineZeroWidth(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	state := fixtureState()
	state.Online = false
	h.Update(state)

	rp := h.renderLCDFrag(th.OfflineBg, 1.0)
	if rp.visWidth != 0 {
		t.Errorf("offline: visWidth=%d want 0", rp.visWidth)
	}
	if rp.top != "" || rp.bot != "" {
		t.Errorf("offline: expected empty rows, got top=%q bot=%q", rp.top, rp.bot)
	}
}

// TestRenderLCDFrag_OnlineTwoRowsEqualWidth — online fragment: both rows
// measured at the reported visWidth and non-zero.
func TestRenderLCDFrag_OnlineTwoRowsEqualWidth(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	iconBg := th.OverallGradient[1].Bg
	rp := h.renderLCDFrag(iconBg, 1.0)
	if rp.visWidth == 0 {
		t.Fatalf("online: visWidth=0, expected non-zero LCD")
	}
	if got := ansi.StringWidth(rp.top); got != rp.visWidth {
		t.Errorf("top ansi width=%d want %d", got, rp.visWidth)
	}
	if got := ansi.StringWidth(rp.bot); got != rp.visWidth {
		t.Errorf("bot ansi width=%d want %d", got, rp.visWidth)
	}
}

// TestRenderSessionsAgentsStack_WidthMaxOfRows — stack width is max of the
// session and agent row widths; both rows padded to that width.
func TestRenderSessionsAgentsStack_WidthMaxOfRows(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	var sessions []collector.SessionTree
	if h.state.Current != nil {
		sessions = h.state.Current.Sessions
	}
	iconBg := th.OverallGradient[1].Bg
	sessionStr, sessionWidth := renderSessionDiamonds(sessions, iconBg, th)
	_, activeStr, _, activeWidth := h.agents.RenderSplit(ui.AgentGlyphHollow, ui.AgentGlyphMid, ui.AgentGlyphFilled, iconBg, 0)

	rp := h.renderSessionsAgentsStack(sessionStr, sessionWidth, activeStr, activeWidth, iconBg)

	if got := ansi.StringWidth(rp.top); got != rp.visWidth {
		t.Errorf("top ansi width=%d visWidth=%d", got, rp.visWidth)
	}
	if got := ansi.StringWidth(rp.bot); got != rp.visWidth {
		t.Errorf("bot ansi width=%d visWidth=%d", got, rp.visWidth)
	}
}

// TestRenderSessionsAgentsStack_EmptySessionsStillFillsRow — no sessions
// but 3 agents: top row is bg-filled to agent width, not empty. Prevents
// the cell from becoming L-shaped.
func TestRenderSessionsAgentsStack_EmptySessionsStillFillsRow(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())
	h.agents.SetTarget(3)
	for i := 0; i < 60; i++ {
		h.agents.AnimTick()
	}

	iconBg := th.OverallGradient[1].Bg
	_, activeStr, _, activeWidth := h.agents.RenderSplit(ui.AgentGlyphHollow, ui.AgentGlyphMid, ui.AgentGlyphFilled, iconBg, 0)
	if activeWidth == 0 {
		t.Fatalf("precondition: active agents row should be non-empty")
	}

	rp := h.renderSessionsAgentsStack("", 0, activeStr, activeWidth, iconBg)

	if rp.visWidth != activeWidth {
		t.Errorf("visWidth=%d want %d (active width)", rp.visWidth, activeWidth)
	}
	if ansi.StringWidth(rp.top) != activeWidth {
		t.Errorf("top width=%d want %d (bg-padded)", ansi.StringWidth(rp.top), activeWidth)
	}
}

// TestRenderSessionsAgentsStack_BothEmptyReturnsZero — no sessions, no
// agents: the whole fragment collapses so ViewLines can omit the divider.
func TestRenderSessionsAgentsStack_BothEmptyReturnsZero(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	iconBg := th.OverallGradient[1].Bg
	rp := h.renderSessionsAgentsStack("", 0, "", 0, iconBg)
	if rp.visWidth != 0 || rp.top != "" || rp.bot != "" {
		t.Errorf("empty stack: got %+v, want zero rowPair", rp)
	}
}

// TestRenderBuildShellStack_BothRowsFixedWidth — both rows render at
// fixedCellWidth so the stack is a consistent rectangle.
func TestRenderBuildShellStack_BothRowsFixedWidth(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	smoothed := map[string]float64{"build": 2, "shell": 1}
	rp := h.renderBuildShellStack(smoothed)
	if rp.visWidth != fixedCellWidth {
		t.Errorf("visWidth=%d want %d", rp.visWidth, fixedCellWidth)
	}
	if got := ansi.StringWidth(rp.top); got != fixedCellWidth {
		t.Errorf("top width=%d want %d", got, fixedCellWidth)
	}
	if got := ansi.StringWidth(rp.bot); got != fixedCellWidth {
		t.Errorf("bot width=%d want %d", got, fixedCellWidth)
	}
}

// TestRenderBuildShellStack_TopIsBuildBotIsShell — structural contract:
// build on top, shell on bottom. Strip ANSI to test content.
func TestRenderBuildShellStack_TopIsBuildBotIsShell(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	smoothed := map[string]float64{"build": 2, "shell": 1}
	rp := h.renderBuildShellStack(smoothed)
	if !strings.Contains(ansi.Strip(rp.top), "build:") {
		t.Errorf("top missing build: %q", ansi.Strip(rp.top))
	}
	if !strings.Contains(ansi.Strip(rp.bot), "shell:") {
		t.Errorf("bot missing shell: %q", ansi.Strip(rp.bot))
	}
}

// TestHeadline_OfflineNoLCDNoStacks — offline mode must NOT leak stack
// content onto either row. Quip goes on top, bot is bg-filled.
func TestHeadline_OfflineNoLCDNoStacks(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	state := fixtureState()
	state.Online = false
	h.Update(state)

	lines := h.ViewLines()
	for i, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "build:") {
			t.Errorf("offline line %d leaks build:%q", i, stripped)
		}
		if strings.Contains(stripped, "shell:") {
			t.Errorf("offline line %d leaks shell:%q", i, stripped)
		}
		if strings.ContainsRune(stripped, '⌬') {
			t.Errorf("offline line %d leaks session diamond:%q", i, stripped)
		}
	}
}

// TestHeadline_RightMarginProtectsLCDDegreeGlyph — the LCD's degree glyph
// is the rightmost visible content on the headline and some terminals
// clip the final column. Reserve at least one cell of right-side bg
// padding so the degree always renders unclipped.
func TestHeadline_RightMarginProtectsLCDDegreeGlyph(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())
	// Pump ticks so the readout settles out of ghost/flash states.
	for i := 0; i < 8; i++ {
		h.AnimTick()
	}

	lines := h.ViewLines()
	isBraille := func(r rune) bool { return r >= 0x2800 && r <= 0x28FF }

	for rowIdx, line := range lines {
		stripped := ansi.Strip(line)
		runes := []rune(stripped)
		if len(runes) == 0 {
			t.Fatalf("row %d empty", rowIdx)
		}
		last := runes[len(runes)-1]
		if isBraille(last) && last != 0x2800 {
			t.Errorf("row %d last rune %U is a filled braille cell — LCD content flush against right edge, will be clipped:\n%q",
				rowIdx, last, stripped)
		}
	}
}

// TestHeadline_NarrowTerminalDoesNotPanic — small widths must not panic and
// must still produce two equal-width rows.
func TestHeadline_NarrowTerminalDoesNotPanic(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(40, 2)
	h.Update(fixtureState())

	lines := h.ViewLines()
	if len(lines) != 2 {
		t.Fatalf("narrow: got %d lines, want 2", len(lines))
	}
	if ansi.StringWidth(lines[0]) != ansi.StringWidth(lines[1]) {
		t.Errorf("narrow: row widths differ top=%d bot=%d",
			ansi.StringWidth(lines[0]), ansi.StringWidth(lines[1]))
	}
}

func TestVisibleCategoriesFixedAlwaysPresent(t *testing.T) {
	smoothed := map[string]float64{} // all zero
	got := visibleCategories(smoothed)
	// build and shell must appear even with zero counts
	found := map[string]bool{}
	for _, cat := range got {
		found[cat.Name] = true
	}
	if !found["build"] {
		t.Error("build should always be visible")
	}
	if !found["shell"] {
		t.Error("shell should always be visible")
	}
}

func TestVisibleCategoriesDynamicAppearsWhenNonZero(t *testing.T) {
	smoothed := map[string]float64{"node": 5.0}
	got := visibleCategories(smoothed)
	found := map[string]bool{}
	for _, cat := range got {
		found[cat.Name] = true
	}
	if !found["node"] {
		t.Error("node should be visible when count > 0")
	}
}

func TestVisibleCategoriesDynamicHiddenWhenZero(t *testing.T) {
	smoothed := map[string]float64{} // no go
	got := visibleCategories(smoothed)
	for _, cat := range got {
		if cat.Name == "go" {
			t.Error("go should not be visible when count is zero")
		}
	}
}

func TestVisibleCategoriesPreservesOrder(t *testing.T) {
	smoothed := map[string]float64{"node": 5.0, "go": 2.0, "rust": 1.0}
	got := visibleCategories(smoothed)
	// Should follow collector.Categories order
	prevOrder := -1
	for _, cat := range got {
		for _, ref := range collector.Categories {
			if ref.Name == cat.Name {
				if ref.Order < prevOrder {
					t.Errorf("category %q (order %d) appeared after order %d", cat.Name, ref.Order, prevOrder)
				}
				prevOrder = ref.Order
			}
		}
	}
}
