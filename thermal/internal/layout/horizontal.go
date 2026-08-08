// Package layout composes widgets into screen layouts for the thermal dashboard.
package layout

import (
	"cmp"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

// Help mode states. int8 (not bool) to leave room for future modes
// without changing the public API.
const (
	HelpShort int8 = 0
	HelpFull  int8 = 1
)

// Intel overlay pages. The `i` key cycles off → session summary →
// scoreboard → off; focused-agent sub-mode always returns to the
// session page (never scoreboard).
const (
	intelPageSession    int8 = 0
	intelPageScoreboard int8 = 1
)

// Horizontal is the bottom-strip layout engine (wide, short — ~244x10).
// Layout order:
//
//	Line 1:    notification bar (plugin CTA / update available)
//	Lines 2-3: headline (category cells, session diamonds, agent dots, LCD, battery)
//	Lines 4-9: sparkline gauges (cpu%, mem%, swap — 2 rows each)
//	           [h] help overlay or [i] intel overlay composites over dimmed gauges
//	Line 10:   rates (system stats, spawn/death/net, short help)
//	Lines 11+: alerts
type Horizontal struct {
	width          int
	height         int
	state          *model.AppState
	headline       *widgets.Headline
	gauges         *widgets.Gauges
	rates          *widgets.Rates
	alerts         *widgets.Alerts
	helpMode       int8
	intelMode      bool
	intelPage      int8   // page shown when intel is on and no agent is focused
	focusedAgentID string // non-empty → focused-agent sub-mode within intel
	collapsed      bool
	theme          *theme.Theme

	// scoreboardSrc overrides the scoreboard's stats source (tests
	// inject a counting fake). Nil → the state's attached aggregator.
	scoreboardSrc statsSource
	sbCache       scoreboardCache
}

// statsSource is the narrow read surface the scoreboard page pulls
// from. *stats.Aggregator satisfies it; window and distribution
// queries live on the live aggregator (each takes its read lock), not
// on Snapshot, so the pull captures everything in one pass and the
// render path never touches the source.
type statsSource interface {
	Snapshot() stats.Snapshot
	VisibleWindows() []string
	Window(days int) stats.Counters
	WindowByType(days int) map[string]int64
	WindowByProject(days int) map[string]int64
}

// scoreboardCache captures everything the scoreboard page renders in
// a single pull (§3.4 of the scoreboard spec). Mixing a cached
// snapshot with live window queries would show internally
// inconsistent numbers, so all of it lands together. pulledAt doubles
// as Renderer.Now for every format call on the page.
type scoreboardCache struct {
	snap        stats.Snapshot
	windowKeys  []string                  // "today" + VisibleWindows() + "lifetime", deduped
	windows     map[string]stats.Counters // per key in windowKeys
	byType30    map[string]int64
	byProject30 map[string]int64
	pulledAt    time.Time

	// rendered memoizes the formatted band so per-frame renders are a
	// slice return, not a re-format of unchanged data. Invalidated by
	// any re-pull (fresh struct) or a width change.
	rendered      []string
	renderedWidth int
}

func NewHorizontal(th *theme.Theme, ap *anim.Profile, km keys.KeyMap) *Horizontal {
	h := &Horizontal{
		state:    model.NewAppState(),
		headline: widgets.NewHeadline(th, ap),
		gauges:   widgets.NewGauges(th, ap),
		alerts:   widgets.NewAlerts(th),
		theme:    th,
	}
	h.rates = widgets.NewRates(th, km)
	return h
}

func (h *Horizontal) State() *model.AppState {
	return h.state
}

func (h *Horizontal) SetSize(w, height int) {
	h.width = w
	h.height = height
	h.headline.SetSize(w, 2)
	h.gauges.SetSize(w, 6)
	h.rates.SetSize(w, 1)
	h.alerts.SetSize(w, 2)
}

// RenderedAgentIDs returns the cached snapshot of completed-agent IDs that
// were zone-marked in the most recent render. Delegates to Headline.
func (h *Horizontal) RenderedAgentIDs() []string {
	return h.headline.RenderedAgentIDs()
}

// SetHoveredAgent sets the agent ID that should render at full brightness.
func (h *Horizontal) SetHoveredAgent(id string) {
	h.headline.SetHoveredAgent(id)
}

// SetHighScoreMode toggles KITT-as-highscore on the agent dot display.
func (h *Horizontal) SetHighScoreMode(on bool) {
	h.headline.SetHighScoreMode(on)
}

func (h *Horizontal) HelpMode() int8 {
	return h.helpMode
}

func (h *Horizontal) ToggleHelp() {
	h.DismissIntel()
	if h.helpMode == HelpFull {
		h.helpMode = HelpShort
		return
	}
	h.helpMode = HelpFull
}

func (h *Horizontal) DismissHelp() {
	h.helpMode = HelpShort
}

func (h *Horizontal) IntelMode() bool {
	return h.intelMode
}

func (h *Horizontal) ToggleIntel() {
	h.helpMode = HelpShort
	switch {
	case h.intelMode && h.focusedAgentID != "":
		// Focused → session summary: clear focus, keep intel.
		h.focusedAgentID = ""
		h.intelPage = intelPageSession
	case h.intelMode && h.intelPage == intelPageSession:
		h.intelPage = intelPageScoreboard
		h.ensureScoreboardPull(true)
	case h.intelMode:
		// Scoreboard → off.
		h.DismissIntel()
	default:
		// Off → session summary.
		h.intelMode = true
		h.intelPage = intelPageSession
		h.focusedAgentID = ""
	}
}

func (h *Horizontal) DismissIntel() {
	h.intelMode = false
	h.intelPage = intelPageSession
	h.focusedAgentID = ""
	h.sbCache = scoreboardCache{} // stale data must not resurface on re-entry
}

// resolveStatsSource resolves the scoreboard's data source: the test
// seam if set, else the state's attached aggregator. The
// nil-aggregator check stays explicit so a nil *stats.Aggregator
// never leaks out as a non-nil interface value.
func (h *Horizontal) resolveStatsSource() statsSource {
	if h.scoreboardSrc != nil {
		return h.scoreboardSrc
	}
	if agg := h.state.Aggregator(); agg != nil {
		return agg
	}
	return nil
}

// ensureScoreboardPull owns all the §3.4 pull moments in one place:
// page entry (force), unpopulated cache (covers any entry path that
// bypassed the `i` keypress), and UTC day rollover (otherwise the
// "today" row silently zeroes at midnight). Returns the resolved
// source, nil when no aggregator is attached.
func (h *Horizontal) ensureScoreboardPull(force bool) statsSource {
	src := h.resolveStatsSource()
	if src == nil {
		return nil
	}
	if force || h.sbCache.pulledAt.IsZero() ||
		stats.DayKey(time.Now().UTC()) != stats.DayKey(h.sbCache.pulledAt) {
		h.pullScoreboard(src)
	}
	return src
}

// pullScoreboard refills the cache from src in one pass.
func (h *Horizontal) pullScoreboard(src statsSource) {
	now := time.Now().UTC()
	snap := src.Snapshot()
	keys := format.WindowKeys(src.VisibleWindows())
	windows := make(map[string]stats.Counters, len(keys))
	for _, k := range keys {
		windows[k] = windowCountersFor(src, snap, k, now)
	}
	h.sbCache = scoreboardCache{
		snap:        snap,
		windowKeys:  keys,
		windows:     windows,
		byType30:    src.WindowByType(30),
		byProject30: src.WindowByProject(30),
		pulledAt:    now,
	}
}

// windowCountersFor maps a window key to its Counters. The key
// vocabulary is parsed by format.ParseWindowKey (shared with the
// CLI's windowCounters); only the source dispatch lives here.
func windowCountersFor(src statsSource, snap stats.Snapshot, key string, now time.Time) stats.Counters {
	switch kind, days := format.ParseWindowKey(key); kind {
	case format.WindowToday:
		return snap.Daily[stats.DayKey(now)]
	case format.WindowLifetime:
		return snap.Lifetime()
	case format.WindowDays:
		return src.Window(days)
	default:
		return stats.Counters{}
	}
}

// FocusAgent enters the focused-agent intel sub-mode for the given agent ID.
// Enters intel mode if not already active.
func (h *Horizontal) FocusAgent(id string) {
	h.helpMode = HelpShort
	h.intelMode = true
	h.intelPage = intelPageSession
	h.focusedAgentID = id
}

// FocusedAgentID returns the currently focused agent ID, or "" if none.
func (h *Horizontal) FocusedAgentID() string {
	return h.focusedAgentID
}

// FocusedTranscriptPath returns the transcript path of the currently focused
// agent, or "" if no agent is focused or the record has no path.
func (h *Horizontal) FocusedTranscriptPath() string {
	if h.focusedAgentID == "" {
		return ""
	}
	rec := h.state.LookupAgent(h.focusedAgentID)
	if rec == nil {
		return ""
	}
	return rec.TranscriptPath
}

func (h *Horizontal) ToggleCollapse() {
	h.collapsed = !h.collapsed
}

func (h *Horizontal) IsCollapsed() bool {
	return h.collapsed
}

func (h *Horizontal) Update(state *model.AppState) {
	h.state = state
	if state.IsIdle() {
		h.DismissIntel() // dismiss stale intel on idle transition
	}
	h.headline.Update(state)
	h.gauges.Update(state)
	h.rates.Update(state)
	h.alerts.Update(state)
}

// AnimTick advances spring animations between snapshots.
func (h *Horizontal) AnimTick() {
	h.headline.AnimTick()
	h.gauges.AnimTick()
}

func (h *Horizontal) View() string {
	if h.width == 0 || h.height == 0 {
		return ""
	}
	if h.state.IsIdle() {
		return h.idleView()
	}
	return h.activeView()
}

func (h *Horizontal) activeView() string {
	var lines []string

	if !h.collapsed && h.height >= 3 {
		if bar := h.notificationBar(); bar != "" {
			lines = append(lines, bar)
		}
	}

	if h.height >= 1 {
		lines = append(lines, h.headline.ViewLines()...)
	}

	if h.intelMode && h.height >= 6 {
		h.gauges.SetDimmed(true)
		gaugeLines := h.gauges.ViewLines(h.height - 1)
		h.gauges.SetDimmed(false)
		lines = append(lines, overlayContent(h.intelView(), gaugeLines)...)
	} else if h.helpMode == HelpFull && h.height >= 6 {
		h.gauges.SetDimmed(true)
		gaugeLines := h.gauges.ViewLines(h.height - 1)
		h.gauges.SetDimmed(false)
		lines = append(lines, overlayContent(h.helpView(), gaugeLines)...)
	} else if h.height >= 4 {
		lines = append(lines, h.gauges.ViewLines(h.height-1)...)
	}

	if h.height >= 9 {
		lines = append(lines, h.rates.View())
	}

	return h.padToHeight(lines)
}

// overlayContent composes help lines over dimmed gauge lines using the widest
// help line as the opacity border: each help row is padded to that width,
// and dimmed sparkline shows through from that column onward on every row.
// Gauge rows below the help block render as unmodified dimmed sparklines.
//
// Truncation contract: this composites at most len(gaugeLines) rows and
// silently drops the rest — callers own fitting their band to the canvas
// (the session page uses 5 of the ~6 gauge rows; the scoreboard band caps
// itself at 6).
func overlayContent(help, gaugeLines []string) []string {
	if len(gaugeLines) == 0 {
		return gaugeLines
	}
	var border int
	for _, l := range help {
		if w := ansi.StringWidth(l); w > border {
			border = w
		}
	}
	out := make([]string, len(gaugeLines))
	copy(out, gaugeLines)
	for i := 0; i < len(help) && i < len(out); i++ {
		pad := strings.Repeat(" ", border-ansi.StringWidth(help[i]))
		right := ansi.TruncateLeft(gaugeLines[i], border, "")
		out[i] = help[i] + pad + right
	}
	return out
}

func (h *Horizontal) helpView() []string {
	d := h.theme.HelpColor
	ct := ui.ColorText
	dim := ui.DimText
	sp := h.theme.SessionPhase

	diamond := func(c color.Color) string { return ct(c, ui.SessionDiamondGlyph) }

	return []string{
		" " + dim("sparklines") + " " + ct(d, "CPU cores") + "  " + ct(d, "MEM app memory") + "  " +
			ct(d, "SWAP compressor pressure — spikes before lockup") + "  " +
			dim("|") + " " + dim("⊞") + " " + ct(d, "Desktop") + " " + dim("⊙") + " " + ct(d, "Chrome"),
		" " + dim("sessions") + " " + diamond(sp.Idle) + "  " + ct(d, "idle") + " " +
			diamond(sp.Active) + " " + ct(d, "active") + "  " +
			diamond(sp.Language) + " " + ct(d, "language") + "  " +
			diamond(sp.Build) + " " + ct(d, "build") + "  " +
			diamond(sp.Explosion) + " " + ct(d, "shells (30+)"),
		" " + dim("agents") + " " + dim(ui.AgentGlyphHollow) + dim(ui.AgentGlyphMid) + dim(ui.AgentGlyphFilled) + " " +
			ct(d, "subagents — tidal wave, ghosts KITT-scan") + dim(" · ") +
			ct(d, "click dot for details"),
		" " + dim("filter") + " " + ct(d, "[ prev") + "  " + ct(d, "] next") + "  " + ct(d, "\\ clear") + "  " +
			dim("|") + " " + ct(d, "m toggle mouse") + "  " +
			dim("click a headline category to filter"),
		" " + ct(d, "i intel · scoreboard") + "  " +
			dim("|") + " " + ct(d, "x clear completed") + "  " + ct(d, "c collapse"),
		" " + dim("press any key to dismiss"),
	}
}

func (h *Horizontal) intelView() []string {
	// Focused-agent sub-mode: render single-agent view.
	if h.focusedAgentID != "" {
		return h.focusedIntelView()
	}
	if h.intelPage == intelPageScoreboard {
		return h.scoreboardView()
	}

	dim := ui.DimText
	ct := func(s string) string { return ui.ColorText(h.theme.HelpColor, s) }
	sep := dim(" · ")
	s := h.state

	// Row 1: agents — active, completed, peak, uptime
	active := ct(fmt.Sprintf("%d active", s.AgentCount()))
	completed := ct(fmt.Sprintf("%d completed", s.CompletedAgentCount()))
	peakVal := float64(s.PeakConcurrency())
	peakThresh := &theme.SparkThresholds{Warn: 3, Crit: 6}
	peakColor := h.theme.SeverityColor(peakVal, peakThresh)
	peak := peakColor + fmt.Sprintf("peak %d", s.PeakConcurrency()) + "\033[0m"
	row1 := " " + dim("agents") + "    " + active + sep + completed + sep + peak
	if uptime := s.SessionUptime(); uptime > 0 {
		row1 += sep + ct(formatUptime(uptime))
	}

	// Row 2: types — sorted by count descending, cap at 5
	typeCounts := s.AgentTypeCounts()
	type typeEntry struct {
		name  string
		count int
	}
	var entries []typeEntry
	for name, count := range typeCounts {
		entries = append(entries, typeEntry{name, count})
	}
	slices.SortStableFunc(entries, func(a, b typeEntry) int {
		if a.count != b.count {
			return cmp.Compare(b.count, a.count) // descending by count
		}
		return cmp.Compare(a.name, b.name) // ascending by name for ties
	})
	var typeParts []string
	if len(entries) <= 5 {
		for _, e := range entries {
			typeParts = append(typeParts, ct(fmt.Sprintf("%d %s", e.count, e.name)))
		}
	} else {
		for _, e := range entries[:4] {
			typeParts = append(typeParts, ct(fmt.Sprintf("%d %s", e.count, e.name)))
		}
		other := 0
		for _, e := range entries[4:] {
			other += e.count
		}
		typeParts = append(typeParts, ct(fmt.Sprintf("%d other", other)))
	}
	row2 := " " + dim("types") + "     "
	if len(typeParts) > 0 {
		row2 += strings.Join(typeParts, sep)
	} else {
		row2 += dim("none")
	}

	// Row 3: duration — avg, longest
	var row3 string
	records := s.CompletedRecords()
	if len(records) == 0 {
		row3 = " " + dim("duration") + "  " + dim("no completions yet")
	} else {
		var totalDur time.Duration
		var longest model.AgentRecord
		for _, rec := range records {
			totalDur += rec.Duration
			if rec.Duration > longest.Duration {
				longest = rec
			}
		}
		avg := totalDur / time.Duration(len(records))
		avgStr := ct(fmt.Sprintf("avg %s", formatDuration(avg)))
		longestStr := ct(fmt.Sprintf("longest %s", formatDuration(longest.Duration)))
		longestID := ""
		if longest.AgentID != "" {
			id := longest.AgentID
			if len(id) > 6 {
				id = id[:6]
			}
			longestID = " " + dim("("+longest.AgentType+" "+id+")")
		}
		row3 = " " + dim("duration") + "  " + avgStr + sep + longestStr + longestID
	}

	// Row 4: tools — throttled, blocked
	capCount := s.GateCapCount()
	suppressCount := s.GateSuppressCount()
	var throttledStr string
	if capCount > 0 {
		throttledStr = ui.ColorText(h.theme.SpawnColor, fmt.Sprintf("%d throttled", capCount))
	} else {
		throttledStr = dim("0 throttled")
	}
	blockedStr := ct(fmt.Sprintf("%d blocked", suppressCount))
	row4 := " " + dim("tools") + "     " + throttledStr + sep + blockedStr

	// Row 5: output — transcript bytes, orphans, drift
	totalBytes := s.TotalTranscriptBytes()
	bytesStr := ct(formatBytesCompact(totalBytes) + " transcripts")
	orphanStr := ct(fmt.Sprintf("%d orphans", s.OrphanStopCount()))
	driftStr := ct(fmt.Sprintf("drift %d", s.CounterDrift()))
	// Inline scoreboard hint keeps the page discoverable without a
	// sixth row — the 5-row band shape is a locked contract.
	row5 := " " + dim("output") + "    " + bytesStr + sep + orphanStr + sep + driftStr +
		sep + dim("i scoreboard")

	return []string{row1, row2, row3, row4, row5}
}

// scoreboardView renders the scoreboard intel page from sbCache.
// Steady-state renders read only the cache; the source is touched
// solely by the pull guard below (unpopulated cache or UTC day
// rollover — see pullScoreboard).
//
// Formatting vocabulary note: this file holds two. The session page
// keeps the private formatDuration/formatBytesCompact helpers (their
// tiering differs — "1m30s" vs stats/format's "1m 30s"); the
// scoreboard page renders exclusively through stats/format so the
// CLI and OTEL consumers stay copy-identical. Do not mix vocabularies
// within a page.
func (h *Horizontal) scoreboardView() []string {
	if src := h.ensureScoreboardPull(false); src == nil {
		// Bare test/degraded-init state — no aggregator attached.
		return []string{" " + ui.DimText("stats unavailable")}
	}
	// Everything below is pure formatting of the immutable cache, so
	// it memoizes per (pull, width) — steady-state frames return the
	// cached band.
	if h.sbCache.rendered == nil || h.sbCache.renderedWidth != h.width {
		h.sbCache.rendered = h.renderScoreboard()
		h.sbCache.renderedWidth = h.width
	}
	return h.sbCache.rendered
}

// renderScoreboard formats the band from sbCache. Called only on
// cache or width changes — see the memoization in scoreboardView.
func (h *Horizontal) renderScoreboard() []string {
	if h.sbCache.snap.FirstSeen.IsZero() {
		// Neutral copy only: a brand-new healthy install is also
		// FirstSeen-zero, so no upgrade hint here (that stays CLI-side
		// behind its stricter folded-events gate).
		return []string{" " + ui.DimText("no agent activity recorded yet — records appear after the first subagent runs")}
	}
	if h.width > 0 && h.width < scoreboardMinWidth {
		// Cycle order is unchanged — the page still occupies its slot
		// so muscle memory stays intact.
		return []string{" " + ui.DimText("scoreboard needs a wider window")}
	}

	dim := ui.DimText
	ct := func(s string) string { return ui.ColorText(h.theme.HelpColor, s) }
	title := " " + dim("── ") + ct("scoreboard · all-time") + dim(" ──  i session intel · any key dismiss")

	groups := [][]string{h.sbRecordsGroup(), h.sbWindowsGroup(), h.sbDistributionsGroup()}
	groups = fitColumnGroups(groups, h.width, ansi.StringWidth(sbGroupSep))
	return append([]string{title}, joinColumnGroups(groups, dim(sbGroupSep))...)
}

// sbGroupSep separates side-by-side column groups (rendered dim).
const sbGroupSep = " │ "

// scoreboardMinWidth is the floor below which the scoreboard page
// renders a single-line fallback instead of any column group.
const scoreboardMinWidth = 60

// fitColumnGroups keeps the leading groups that fit within width,
// dropping from the tail first (distributions, then windows). The
// first group always survives — records are the page's reason to
// exist, and the <scoreboardMinWidth fallback already bounds the
// degenerate case. width <= 0 means unsized (tests, pre-layout) and
// keeps everything.
func fitColumnGroups(groups [][]string, width, sepWidth int) [][]string {
	if width <= 0 {
		return groups
	}
	used := maxLineWidth(groups[0])
	keep := 1
	for _, g := range groups[1:] {
		used += sepWidth + maxLineWidth(g)
		if used > width {
			break
		}
		keep++
	}
	return groups[:keep]
}

// maxLineWidth returns the widest visible width across a group's
// lines.
func maxLineWidth(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, ansi.StringWidth(l))
	}
	return w
}

// sbCell is one label/value/meta row inside a scoreboard sub-column.
type sbCell struct {
	label, value, meta string
}

// sbRecordsGroup renders the seven eternal leaderboards' top-1
// entries as two side-by-side sub-columns (4 + 3, matching the §3.1
// sketch). Empty boards keep their label and show the dash glyph in
// the value column.
func (h *Horizontal) sbRecordsGroup() []string {
	rec := h.sbCache.snap.Records
	now := h.sbCache.pulledAt

	// Split boards + the burst cell into two balanced sub-columns
	// (4+3 today); deriving from len keeps the columns balanced if
	// the board table ever grows.
	boards := format.Boards()
	split := (len(boards) + 2) / 2
	left := make([]sbCell, 0, split)
	for _, b := range boards[:split] {
		v, when := format.FormatTop1(b.Kind, b.Pick(rec), now)
		left = append(left, sbCell{b.Label, v, when})
	}
	burstValue, burstWhen := format.FormatBurstTop1(rec.BiggestBurst, now)
	right := []sbCell{{format.BurstBoardLabel, burstValue, burstWhen}}
	for _, b := range boards[split:] {
		v, when := format.FormatTop1(b.Kind, b.Pick(rec), now)
		right = append(right, sbCell{b.Label, v, when})
	}
	return joinColumnGroups([][]string{h.styleCells(left), h.styleCells(right)}, ui.DimText(sbGroupSep))
}

// styleCells pads labels and values to per-column widths (computed on
// the plain strings, before styling) and applies the intel theme
// tokens: dim labels/meta, help-color values.
func (h *Horizontal) styleCells(cells []sbCell) []string {
	var lw, vw int
	for _, c := range cells {
		lw = max(lw, ansi.StringWidth(c.label))
		vw = max(vw, ansi.StringWidth(c.value))
	}
	ct := func(s string) string { return ui.ColorText(h.theme.HelpColor, s) }
	out := make([]string, len(cells))
	for i, c := range cells {
		line := " " + ui.DimText(padRight(c.label, lw)) + " " + ct(padRight(c.value, vw))
		if c.meta != "" {
			line += " " + ui.DimText(c.meta)
		}
		out[i] = line
	}
	return out
}

// sbWindowsGroup renders one row per cached window key in the shared
// counters row shape.
func (h *Horizontal) sbWindowsGroup() []string {
	c := &h.sbCache
	cells := make([]sbCell, 0, len(c.windowKeys))
	for _, k := range c.windowKeys {
		cells = append(cells, sbCell{format.FormatWindowLabel(k), format.FormatWindowCounters(c.windows[k]), ""})
	}
	return h.styleCells(cells)
}

// scoreboardDistTop is how many by_type rows the distributions column
// shows before collapsing the remainder into an inline "(N more)".
const scoreboardDistTop = 3

// sbDistributionsGroup renders by_type top-N plus a one-row
// by_project summary, lifetime · last-30d pairs throughout. Overflow
// collapses inline so the group never exceeds the content-row budget.
func (h *Horizontal) sbDistributionsGroup() []string {
	c := &h.sbCache
	dim := ui.DimText
	ct := func(s string) string { return ui.ColorText(h.theme.HelpColor, s) }

	lines := []string{" " + dim("by type (life · 30d)")}
	rows := format.DistRows(c.snap.ByType, c.byType30)
	shown := min(scoreboardDistTop, len(rows))
	for i := 0; i < shown; i++ {
		label, counts := format.FormatDistributionRow(rows[i].Key, rows[i].Lifetime, rows[i].Last30)
		line := "  " + ct(label) + " " + dim(counts)
		if i == shown-1 {
			line += overflowSuffix(len(rows) - shown)
		}
		lines = append(lines, line)
	}
	if proj := format.DistRows(c.snap.ByProject, c.byProject30); len(proj) > 0 {
		label, counts := format.FormatDistributionRow(proj[0].Key, proj[0].Lifetime, proj[0].Last30)
		lines = append(lines, " "+dim("by project:")+" "+ct(label)+" "+dim(counts)+overflowSuffix(len(proj)-1))
	}
	return lines
}

// overflowSuffix renders the dim " (N more)" tail, empty when nothing
// overflows.
func overflowSuffix(extra int) string {
	if extra <= 0 {
		return ""
	}
	return " " + ui.DimText(fmt.Sprintf("(%d more)", extra))
}

// joinColumnGroups pads each group's lines to that group's max
// visible width and joins them row-wise with sep. Shorter groups pad
// with blanks; the last group is never right-padded so the band's
// border (the widest line) stays content-driven. Widths use
// ansi.StringWidth — never len (zone marks inflate byte length).
func joinColumnGroups(groups [][]string, sep string) []string {
	rows := 0
	widths := make([]int, len(groups))
	for gi, g := range groups {
		rows = max(rows, len(g))
		widths[gi] = maxLineWidth(g)
	}
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		parts := make([]string, len(groups))
		for gi, g := range groups {
			line := ""
			if r < len(g) {
				line = g[r]
			}
			if gi < len(groups)-1 {
				line = padRight(line, widths[gi])
			}
			parts[gi] = line
		}
		out[r] = strings.TrimRight(strings.Join(parts, sep), " ")
	}
	return out
}

// padRight pads s with spaces to visible width w.
func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// focusedIntelView renders the single-agent focused view. Nil-guards
// LookupAgent on every render frame — if the record was evicted, clears
// focusedAgentID and falls back to session summary.
func (h *Horizontal) focusedIntelView() []string {
	rec := h.state.LookupAgent(h.focusedAgentID)
	if rec == nil {
		h.focusedAgentID = ""
		return h.intelView() // re-enters intelView which now takes session path
	}

	dim := ui.DimText
	ct := func(s string) string { return ui.ColorText(h.theme.HelpColor, s) }
	sep := dim(" · ")

	// Row 1: agent ID + type + duration
	idStr := ct(rec.AgentID)
	typeStr := ct(rec.AgentType)
	var durStr string
	switch {
	case rec.Purged:
		durStr = dim("purged")
	case rec.Orphan:
		durStr = dim("orphan (no start event)")
	case rec.Duration > 0:
		durStr = ct(formatDuration(rec.Duration))
	default:
		durStr = dim("active")
	}
	row1 := " " + dim("agent") + "     " + idStr + sep + typeStr + sep + durStr

	// Row 2: project + permission mode
	var row2Parts []string
	if rec.Project != "" {
		row2Parts = append(row2Parts, ct(rec.Project))
	}
	if rec.PermissionMode != "" {
		row2Parts = append(row2Parts, ct(rec.PermissionMode))
	}
	row2 := " " + dim("context") + "   "
	if len(row2Parts) > 0 {
		row2 += strings.Join(row2Parts, sep)
	} else {
		row2 += dim("none")
	}

	// Row 3: transcript size — "transcript" is a click target to open the file
	var row3 string
	if rec.TranscriptBytes > 0 && strings.HasPrefix(rec.TranscriptPath, "/") {
		label := zone.Mark(ui.PathZoneID(rec.AgentID), ui.ColorText(h.theme.HelpColor, "transcript"))
		row3 = " " + dim("size") + "      " + ct(formatBytesCompact(rec.TranscriptBytes)) + " " + label
	} else if rec.TranscriptBytes > 0 {
		row3 = " " + dim("size") + "      " + ct(formatBytesCompact(rec.TranscriptBytes)) + " " + dim("transcript")
	} else {
		row3 = " " + dim("size") + "      " + dim("no transcript")
	}

	// Row 4: hint
	row4 := " " + dim("i session summary · any key to dismiss")

	return []string{row1, row2, row3, row4}
}

// formatUptime formats a duration as "Xh Ym" or "Xm Ys" for compact display.
func formatUptime(d time.Duration) string {
	if d >= time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

// formatDuration formats a duration as compact seconds for agent durations.
func formatDuration(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	return fmt.Sprintf("%dm%ds", m, s)
}

// formatBytesCompact formats bytes as "N B" / "N.N KB" / "N.N MB" with
// decimal point and space for human-readable intel display. Distinct from
// model.FormatBytes which uses tighter "NKB"/"NMB"/"N.NGB" for gauges.
func formatBytesCompact(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func (h *Horizontal) idleView() string {
	quip := ui.ColorText(h.theme.IdleColor, h.state.StableQuip())

	var lines []string

	if !h.collapsed && h.height >= 3 {
		if bar := h.notificationBar(); bar != "" {
			lines = append(lines, bar)
		}
	}

	lines = append(lines, " "+ui.ColorText(h.theme.IdleColor, "◉")+" "+ui.DimText("coolant")+"  "+quip)

	if h.state.Current != nil && h.height >= 4 {
		snap := h.state.Current
		memGB := snap.System.MemUsedBytes / model.GB
		totalGB := snap.System.MemTotalBytes / model.GB
		stats := ui.DimText(fmt.Sprintf(" CPU:%03d%%  MEM:%02d/%02dGB", int(snap.System.CPUPercent), memGB, totalGB))
		lines = append(lines, stats)
	}

	return h.padToHeight(lines)
}

func (h *Horizontal) padToHeight(lines []string) string {
	for len(lines) < h.height {
		lines = append(lines, "")
	}
	if len(lines) > h.height {
		lines = lines[:h.height]
	}
	return strings.Join(lines, "\n")
}

// notificationBar renders the collapsible hint bar (shared between active and idle views).
// Returns empty string when plugin is active and there's nothing to show.
func (h *Horizontal) notificationBar() string {
	var hints []string
	if !h.state.PluginActive {
		hints = append(hints, "[i] install coolant plugin for agent-level insights")
	}
	if h.state.UpdateAvailable {
		hints = append(hints, "update available \u00b7 changelog \u2192 releases/latest")
	}
	if len(hints) == 0 {
		return ""
	}
	hints = append(hints, "[c] collapse")
	return ui.DimText(" " + strings.Join(hints, "  "))
}
