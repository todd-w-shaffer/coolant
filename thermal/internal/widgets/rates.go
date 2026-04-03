package widgets

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// Rates renders spawn/death/net rates + system stats, all fixed-width:
// spawn:+003/s  death:-001/s  net:+002/s  |  CPU:034%  MEM:11.2/16.0GB  SWAP:0000MB  headroom:~04.8GB
type Rates struct {
	width int
	state *model.AppState
}

func NewRates() *Rates {
	return &Rates{}
}

func (r *Rates) SetSize(w, h int) {
	r.width = w
}

func (r *Rates) Update(state *model.AppState) {
	r.state = state
}

func (r *Rates) View() string {
	if r.state == nil || r.state.Current == nil {
		return ""
	}
	s := r.state
	snap := s.Current

	// Offline: show duration instead of rates
	if !s.Online {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
		durStr := "just now"
		if s.OfflineDuration >= time.Minute {
			durStr = fmt.Sprintf("%dm %ds", int(s.OfflineDuration.Minutes()), int(s.OfflineDuration.Seconds())%60)
		} else if s.OfflineDuration > 0 {
			durStr = fmt.Sprintf("%ds", int(s.OfflineDuration.Seconds()))
		}
		return " " + cyan.Render(fmt.Sprintf("OFFLINE %s", durStr)) +
			dim.Render("  —  no API connection, processes will wind down")
	}

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Warm/cool/net — fixed width with sign
	spawnStr := fmt.Sprintf("warm:%+04d/s", s.LastSpawns())
	deathStr := fmt.Sprintf("cool:-%03d/s", s.LastDeaths())
	netVal := int(s.NetRate)
	netStr := fmt.Sprintf("net:%+04d/s", netVal)

	// System stats — fixed width
	cpuPct := int(snap.System.CPUPercent)
	memUsedGB := float64(snap.System.MemUsedBytes) / float64(1<<30)
	memTotalGB := snap.System.MemTotalBytes / (1 << 30)
	memPct := snap.System.MemPercent()
	swapMB := snap.System.SwapUsedBytes / (1 << 20)

	cpuDot := "\033[37m●\033[0m "  // white — matches sparkline row 1
	memDot := "\033[36m●\033[0m "  // cyan — matches sparkline row 2
	swapDot := "\033[35m●\033[0m " // magenta — matches sparkline row 3

	cpuStr := fmt.Sprintf("%sCPU:%03d%%", cpuDot, cpuPct)
	memStr := fmt.Sprintf("%sMEM:%04.1f/%02dGB", memDot, memUsedGB, memTotalGB)
	swapStr := fmt.Sprintf("%sSWAP:%05dMB", swapDot, swapMB)

	// Color the stats
	cpuColor := ui.ThresholdColor(snap.System.CPUPercent, 70, 90)
	memColor := ui.ThresholdColor(memPct, 60, 80)
	swapColor := lipgloss.Color("8")
	if swapMB > 0 {
		swapColor = lipgloss.Color("3")
	}
	if swapMB > 2048 {
		swapColor = lipgloss.Color("1")
	}

	sep := dim.Render("  |  ")

	stats := fmt.Sprintf(" %s  %s  %s",
		lipgloss.NewStyle().Foreground(cpuColor).Render(cpuStr),
		lipgloss.NewStyle().Foreground(memColor).Render(memStr),
		lipgloss.NewStyle().Foreground(swapColor).Render(swapStr),
	)

	rates := fmt.Sprintf("%s  %s  %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(spawnStr),
		lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(deathStr),
		lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Render(netStr),
	)

	help := dim.Render("[h] help")

	return stats + sep + rates + "  " + help
}
