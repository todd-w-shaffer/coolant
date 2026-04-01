package layout

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/widgets"
)

// Horizontal is the bottom-strip layout engine (wide, short — ~244x10).
// Layout order:
//   Line 1:  [i] plugin CTA (if no plugin)
//   Line 2:  [ overall temp + msg | test:004 | build:008 | run:018 | search:005 | shell:004 ]
//   Lines 3-6: sparklines (procs, cpu%, mem%, swap)
//   Line 7:  spawn:+003/s  death:-001/s  net:+002/s  |  CPU:034%  MEM:11/16GB  SWAP:00000MB
//   Lines 8-9: alerts
//   Line 10: (overflow)
type Horizontal struct {
	width    int
	height   int
	state    *model.AppState
	headline *widgets.Headline
	gauges   *widgets.Gauges
	procbar  *widgets.ProcBar // kept for reference but headline now has thermal boxes
	rates    *widgets.Rates
	alerts   *widgets.Alerts
}

func NewHorizontal() *Horizontal {
	return &Horizontal{
		state:    model.NewAppState(),
		headline: widgets.NewHeadline(),
		gauges:   widgets.NewGauges(),
		procbar:  widgets.NewProcBar(),
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
	h.gauges.SetSize(w, 4)
	h.procbar.SetSize(w, 1)
	h.rates.SetSize(w, 1)
	h.alerts.SetSize(w, 2)
}

func (h *Horizontal) Update(state *model.AppState) {
	h.state = state
	h.headline.Update(state)
	h.gauges.Update(state)
	h.procbar.Update(state)
	h.rates.Update(state)
	h.alerts.Update(state)
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
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Line 1: Plugin CTA (persistent subtle, at the top)
	if !h.state.PluginActive && h.height >= 2 {
		lines = append(lines, dim.Render(" [i] install coolant plugin for agent-level insights"))
	}

	// Line 2: Thermal bar (overall + categories)
	if h.height >= 2 {
		lines = append(lines, h.headline.View())
	}

	// Lines 3-6: Sparklines
	if h.height >= 6 {
		for _, line := range strings.Split(h.gauges.View(), "\n") {
			lines = append(lines, line)
		}
	} else if h.height >= 4 {
		// Compact: just cpu% and mem%
		gaugeLines := strings.Split(h.gauges.View(), "\n")
		if len(gaugeLines) >= 3 {
			lines = append(lines, gaugeLines[1]) // cpu%
			lines = append(lines, gaugeLines[2]) // mem%
		}
	}

	// Stats line: spawn/death/net + CPU/MEM/SWAP
	if h.height >= 7 {
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

func (h *Horizontal) idleView() string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cool := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

	quip := cool.Render(h.state.StableQuip())

	var lines []string

	// CTA even when idle
	if !h.state.PluginActive {
		lines = append(lines, dim.Render(" [i] install coolant plugin for agent-level insights"))
	}

	lines = append(lines, " "+cool.Render("◉")+" "+dim.Render("coolant")+"  "+quip)

	// System stats while idle
	if h.state.Current != nil && h.height >= 4 {
		snap := h.state.Current
		memGB := snap.System.MemUsedBytes / (1 << 30)
		totalGB := snap.System.MemTotalBytes / (1 << 30)
		stats := dim.Render(fmt.Sprintf(" CPU:%03d%%  MEM:%02d/%02dGB", int(snap.System.CPUPercent), memGB, totalGB))
		lines = append(lines, stats)
	}

	for len(lines) < h.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
