package layout

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
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
}

func NewHorizontal() *Horizontal {
	return &Horizontal{
		state:    model.NewAppState(),
		headline: widgets.NewHeadline(),
		gauges:   widgets.NewGauges(),
		rates:    widgets.NewRates(),
		alerts:   widgets.NewAlerts(),
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

	// Idle state
	if h.state.IsIdle() {
		return h.idleView()
	}

	var lines []string

	// Line 1: Notification bar (collapses away with [c], hidden when empty)
	if !h.collapsed && h.height >= 3 {
		if bar := h.notificationBar(); bar != "" {
			lines = append(lines, bar)
		}
	}

	// Line 1-2: Thermal bar (overall + categories)
	if h.height >= 1 {
		lines = append(lines, h.headline.View())
	}

	// Lines 3-8: 2-row sparklines (6 lines) or help overlay
	if h.helpMode && h.height >= 5 {
		lines = append(lines, h.helpView()...)
	} else if h.height >= 3 {
		gaugeLines := strings.Split(h.gauges.View(), "\n")
		if h.height >= 7 {
			lines = append(lines, gaugeLines...)
		} else if h.height >= 5 && len(gaugeLines) >= 4 {
			lines = append(lines, gaugeLines[0], gaugeLines[1])
			lines = append(lines, gaugeLines[2], gaugeLines[3])
		} else if len(gaugeLines) >= 2 {
			lines = append(lines, gaugeLines[0], gaugeLines[1])
		}
	}

	// Stats line: CPU/MEM/SWAP/GPU + warm/cool/net + static indicators
	if h.height >= 8 {
		lines = append(lines, h.rates.View())
	}

	// Pad to fill height
	for len(lines) < h.height {
		lines = append(lines, "")
	}
	if len(lines) > h.height {
		lines = lines[:h.height]
	}

	return strings.Join(lines, "\n")
}

func (h *Horizontal) helpView() []string {
	d := lipgloss.Color("250")
	ct := ui.ColorText

	// Phase diamond samples
	idle := ct(lipgloss.Color("245"), "⌬")
	green := ct(lipgloss.Color("2"), "⌬")
	yellow := ct(lipgloss.Color("3"), "⌬")
	orange := ct(lipgloss.Color("208"), "⌬")
	red := ct(lipgloss.Color("196"), "⌬")

	return []string{
		fmt.Sprintf(" %s %s  %s  %s  %s %s %s  %s %s",
			ui.DimText("sparklines"), ct(d, "CPU cores"),
			ct(d, "MEM app memory"),
			ct(d, "SWAP compressor pressure — spikes before lockup"),
			ui.DimText("|"), ui.DimText("⊞"), ct(d, "Desktop"), ui.DimText("⊙"), ct(d, "Chrome")),
		fmt.Sprintf(" %s %s  %s %s  %s %s  %s %s  %s %s  %s",
			ui.DimText("sessions"), idle, ct(d, "idle"),
			green, ct(d, "active"),
			yellow, ct(d, "language"),
			orange, ct(d, "build"),
			red, ct(d, "shells (30+)")),
		fmt.Sprintf(" %s %s%s %s  %s",
			ui.DimText("agents"),
			ui.DimText(ui.AgentGlyphHollow), ui.DimText(ui.AgentGlyphFilled),
			ct(d, "subagents — hexagons breathe hollow/filled, count matches headline"),
			ui.DimText("categories track process types in the headline bar")),
		fmt.Sprintf(" %s  %s  %s",
			ui.DimText("[h] close"), ui.DimText("[c] collapse"), ui.DimText("[q] quit")),
	}
}

func (h *Horizontal) idleView() string {
	quip := ui.ColorText(ui.CyanColor, h.state.StableQuip())

	var lines []string

	if !h.collapsed && h.height >= 3 {
		if bar := h.notificationBar(); bar != "" {
			lines = append(lines, bar)
		}
	}

	lines = append(lines, " "+ui.ColorText(ui.CyanColor, "◉")+" "+ui.DimText("coolant")+"  "+quip)

	// System stats while idle
	if h.state.Current != nil && h.height >= 4 {
		snap := h.state.Current
		memGB := snap.System.MemUsedBytes / int64(model.GB)
		totalGB := snap.System.MemTotalBytes / int64(model.GB)
		stats := ui.DimText(fmt.Sprintf(" CPU:%03d%%  MEM:%02d/%02dGB", int(snap.System.CPUPercent), memGB, totalGB))
		lines = append(lines, stats)
	}

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
