package widgets

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
)

// ThreatColor maps threat levels to lipgloss colors.
var ThreatColor = map[model.ThreatLevel]lipgloss.Color{
	model.ThreatCool:     lipgloss.Color("2"),   // green
	model.ThreatWarm:     lipgloss.Color("3"),   // yellow
	model.ThreatHot:      lipgloss.Color("208"), // orange
	model.ThreatMeltdown: lipgloss.Color("1"),   // red
}

// Headline renders the top status line:
// ◉ WARM  sessions: 3  procs: 47  |  CPU 34%  MEM 11.2/16GB (70%)  SWAP 0MB  headroom: ~4.8GB
type Headline struct {
	width int
	state *model.AppState
}

func NewHeadline() *Headline {
	return &Headline{}
}

func (h *Headline) SetSize(w, height int) {
	h.width = w
}

func (h *Headline) Update(state *model.AppState) {
	h.state = state
}

func (h *Headline) View() string {
	if h.state == nil || h.state.Current == nil {
		return ""
	}
	s := h.state
	snap := s.Current

	color := ThreatColor[s.ThreatLevel]
	dot := lipgloss.NewStyle().Foreground(color).Render("◉")
	level := lipgloss.NewStyle().Foreground(color).Bold(true).Render(s.ThreatLevel.String())
	quip := lipgloss.NewStyle().Foreground(color).Render(s.StableQuip())

	memUsedGB := float64(snap.System.MemUsedBytes) / float64(1<<30)
	memTotalGB := float64(snap.System.MemTotalBytes) / float64(1<<30)
	memPct := snap.System.MemPercent()
	swapMB := snap.System.SwapUsedBytes / (1 << 20)

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Build left section: threat + counts
	left := fmt.Sprintf("%s %s  %s", dot, level, quip)

	// Build right section: system stats
	memColor := thresholdColor(memPct, 60, 80)
	cpuColor := thresholdColor(snap.System.CPUPercent, 70, 90)

	right := fmt.Sprintf("%s %s  %s  %s",
		lipgloss.NewStyle().Foreground(cpuColor).Render(fmt.Sprintf("CPU %d%%", int(snap.System.CPUPercent))),
		lipgloss.NewStyle().Foreground(memColor).Render(fmt.Sprintf("MEM %.1f/%.0fGB (%d%%)", memUsedGB, memTotalGB, int(memPct))),
		formatSwap(swapMB),
		dim.Render(fmt.Sprintf("headroom: ~%s", model.FormatBytes(s.Headroom.MemAvailBytes))),
	)

	// Counts in the middle
	counts := fmt.Sprintf("%s  %s",
		dim.Render(fmt.Sprintf("sessions: %d", s.SessionCount)),
		dim.Render(fmt.Sprintf("procs: %d", snap.TotalProcs())),
	)

	sep := dim.Render("  |  ")

	return left + "  " + counts + sep + right
}

func formatSwap(mb int64) string {
	if mb == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("SWAP 0MB")
	}
	color := lipgloss.Color("1") // red — any swap is bad
	if mb < 1024 {
		return lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("SWAP %dMB", mb))
	}
	return lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("SWAP %.1fGB", float64(mb)/1024))
}

func thresholdColor(val, warn, crit float64) lipgloss.Color {
	if val >= crit {
		return lipgloss.Color("1") // red
	}
	if val >= warn {
		return lipgloss.Color("3") // yellow
	}
	return lipgloss.Color("7") // white
}
