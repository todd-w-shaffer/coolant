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

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
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
	focusedAgentID string // non-empty → focused-agent sub-mode within intel
	collapsed      bool
	theme          *theme.Theme
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
	h.intelMode = false
	h.focusedAgentID = ""
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
	if h.intelMode && h.focusedAgentID != "" {
		// Focused → session summary: clear focus, keep intel.
		h.focusedAgentID = ""
	} else {
		// Session summary → neutral, or neutral → session summary.
		h.intelMode = !h.intelMode
		h.focusedAgentID = ""
	}
}

func (h *Horizontal) DismissIntel() {
	h.intelMode = false
	h.focusedAgentID = ""
}

// FocusAgent enters the focused-agent intel sub-mode for the given agent ID.
// Enters intel mode if not already active.
func (h *Horizontal) FocusAgent(id string) {
	h.helpMode = HelpShort
	h.intelMode = true
	h.focusedAgentID = id
}

// FocusedAgentID returns the currently focused agent ID, or "" if none.
func (h *Horizontal) FocusedAgentID() string {
	return h.focusedAgentID
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
		h.intelMode = false // dismiss stale intel on idle transition
		h.focusedAgentID = ""
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
		" " + ct(d, "i session intel") + "  " +
			dim("|") + " " + ct(d, "x purge stale") + "  " + ct(d, "c collapse"),
		" " + dim("press any key to dismiss"),
	}
}

func (h *Horizontal) intelView() []string {
	// Focused-agent sub-mode: render single-agent view.
	if h.focusedAgentID != "" {
		return h.focusedIntelView()
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
	row5 := " " + dim("output") + "    " + bytesStr + sep + orphanStr + sep + driftStr

	return []string{row1, row2, row3, row4, row5}
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

	// Row 3: transcript size + path
	var row3 string
	if rec.TranscriptBytes > 0 {
		row3 = " " + dim("size") + "      " + ct(formatBytesCompact(rec.TranscriptBytes)) + " " + dim("transcript")
	} else {
		row3 = " " + dim("size") + "      " + dim("no transcript")
	}

	// Row 4: transcript path
	var row4 string
	if rec.TranscriptPath != "" {
		row4 = " " + dim("path") + "      " + dim(rec.TranscriptPath)
	} else {
		row4 = " " + dim("path") + "      " + dim("—")
	}

	// Row 5: hint
	row5 := " " + dim("i session summary · any key to dismiss")

	return []string{row1, row2, row3, row4, row5}
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
