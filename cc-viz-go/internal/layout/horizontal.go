package layout

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/widgets"
)

// Horizontal is the bottom-strip layout engine (wide, short — ~244x10).
type Horizontal struct {
	width    int
	height   int
	state    *model.AppState
	headline *widgets.Headline
	gauges   *widgets.Gauges
	procbar  *widgets.ProcBar
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

	// Allocate lines by priority based on available height
	if h.height >= 1 {
		lines = append(lines, h.headline.View())
	}
	if h.height >= 5 {
		// Full gauges: 4 sparklines
		for _, line := range strings.Split(h.gauges.View(), "\n") {
			lines = append(lines, line)
		}
	} else if h.height >= 3 {
		// Compact: just mem% and cpu%
		gaugeLines := strings.Split(h.gauges.View(), "\n")
		if len(gaugeLines) >= 2 {
			lines = append(lines, gaugeLines[1]) // cpu%
			lines = append(lines, gaugeLines[2]) // mem%
		}
	}
	if h.height >= 7 {
		lines = append(lines, h.procbar.View())
	}
	if h.height >= 8 {
		lines = append(lines, h.rates.View())
	}
	if h.height >= 9 {
		alertLines := strings.Split(h.alerts.View(), "\n")
		remaining := h.height - len(lines) - 1 // save 1 for CTA
		for i := 0; i < remaining && i < len(alertLines); i++ {
			lines = append(lines, alertLines[i])
		}
	}

	// Plugin CTA at bottom (persistent subtle)
	if h.height >= 2 && !h.state.PluginActive {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		lines = append(lines, dim.Render(" [i] install coolant plugin for agent-level insights"))
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
	cool := lipgloss.NewStyle().Foreground(lipgloss.Color("6")) // cyan

	quip := cool.Render(h.state.StableQuip())

	var lines []string
	lines = append(lines, " "+cool.Render("◉")+" "+dim.Render("coolant")+"  "+quip)

	// Still show system stats while idle
	if h.state.Current != nil && h.height >= 3 {
		snap := h.state.Current
		memGB := float64(snap.System.MemUsedBytes) / float64(1<<30)
		totalGB := float64(snap.System.MemTotalBytes) / float64(1<<30)
		stats := dim.Render(fmt.Sprintf(" CPU %d%%  MEM %.1f/%.0fGB", int(snap.System.CPUPercent), memGB, totalGB))
		lines = append(lines, stats)
	}

	for len(lines) < h.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
