// Package layout composes widgets into screen layouts for the thermal dashboard.
package layout

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

// Horizontal is the bottom-strip layout engine (wide, short — ~244x10).
// Layout order:
//
//	Line 1:   [i] plugin CTA (if no plugin)
//	Line 2:   [ overall temp + msg | test:004 | build:008 | run:018 | search:005 | shell:004 ]
//	Lines 3-8: 2-row sparklines (cpu%, mem%, compressor — 2 rows each)
//	Line 9:   spawn:+003/s  death:-001/s  net:+002/s  |  CPU:034%  MEM:11/16GB  SWAP:00000MB
//	Lines 10-11: alerts
type Horizontal struct {
	width     int
	height    int
	state     *model.AppState
	headline  *widgets.Headline
	gauges    *widgets.Gauges
	rates     *widgets.Rates
	alerts    *widgets.Alerts
	helpMode  bool
	collapsed bool
	theme     *theme.Theme
}

func NewHorizontal(th *theme.Theme, ap *anim.Profile) *Horizontal {
	return &Horizontal{
		state:    model.NewAppState(),
		headline: widgets.NewHeadline(th, ap),
		gauges:   widgets.NewGauges(th, ap),
		rates:    widgets.NewRates(th),
		alerts:   widgets.NewAlerts(th),
		theme:    th,
	}
}

func (h *Horizontal) State() *model.AppState {
	return h.state
}

func (h *Horizontal) SetSize(w, height int) {
	h.width = w
	h.height = height
	h.headline.SetSize(w, 1)
	h.gauges.SetSize(w, 6)
	h.rates.SetSize(w, 1)
	h.alerts.SetSize(w, 2)
}

// SetHighScoreMode toggles KITT-as-highscore on the agent dot display.
func (h *Horizontal) SetHighScoreMode(on bool) {
	h.headline.SetHighScoreMode(on)
}

func (h *Horizontal) ToggleHelp() {
	h.helpMode = !h.helpMode
}

func (h *Horizontal) ToggleCollapse() {
	h.collapsed = !h.collapsed
}

func (h *Horizontal) IsCollapsed() bool {
	return h.collapsed
}

func (h *Horizontal) Update(state *model.AppState) {
	h.state = state
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
		lines = append(lines, h.headline.View())
	}

	if h.helpMode && h.height >= 5 {
		lines = append(lines, h.helpView()...)
	} else if h.height >= 3 {
		lines = append(lines, h.gauges.ViewLines(h.height)...)
	}

	if h.height >= 8 {
		lines = append(lines, h.rates.View())
	}

	return h.padToHeight(lines)
}

func (h *Horizontal) helpView() []string {
	d := h.theme.HelpColor
	ct := ui.ColorText
	dim := ui.DimText
	sp := h.theme.SessionPhase

	diamond := func(c color.Color) string { return ct(c, "⌬") }

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
			ct(d, "subagents — tidal wave hollow/mid/filled, ghosts KITT-scan") + "  " +
			dim("categories track process types in the headline bar"),
		" " + dim("[h] close") + "  " + dim("[c] collapse") + "  " + dim("[x] purge ghosts") + "  " + dim("[q] quit"),
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
	if len(hints) == 0 {
		return ""
	}
	hints = append(hints, "[c] collapse")
	return ui.DimText(" " + strings.Join(hints, "  "))
}
