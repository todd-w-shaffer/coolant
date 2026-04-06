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

	// Stats lines: spawn/death/net + CPU/MEM/SWAP, then sessions + processes
	if h.height >= 9 {
		rateLines := strings.Split(h.rates.View(), "\n")
		lines = append(lines, rateLines...)
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
	dot := func(i int) string { return ui.GaugeDots[i].ANSI + ui.GaugeDots[i].Char + "\033[0m" }
	desc := lipgloss.Color("250")
	cg := ui.CategoryGlyphFormatted
	return []string{
		fmt.Sprintf(" %s  %s  %s", dot(0), ui.ColorText(desc, "CPU"), ui.ColorText(desc, "how hard your cores are working — when this maxes out, everything slows down")),
		fmt.Sprintf(" %s  %s  %s", dot(1), ui.ColorText(desc, "MEM"), ui.ColorText(desc, "memory actually in use by apps — when this fills up, swap starts and things get ugly")),
		fmt.Sprintf(" %s  %s  %s", dot(2), ui.ColorText(desc, "COMP"), ui.ColorText(desc, "memory compressor struggling — this spikes 10-20s before your machine locks up")),
		fmt.Sprintf(" %s %s  %s %s  %s %s %s %s %s %s %s", ui.DimText("⊞"), ui.ColorText(desc, "Desktop"),
			ui.DimText("⊙"), ui.ColorText(desc, "Chrome"),
			ui.ColorText(ui.CyanColor, "⌬"), ui.ColorText(desc, "Code"),
			cg["node"], cg["go"], cg["python"], cg["rust"], cg["build"]),
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
