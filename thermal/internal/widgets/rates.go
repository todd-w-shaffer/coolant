package widgets

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
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
	memUsedGB := snap.System.MemUsedBytes / (1 << 30)
	memTotalGB := snap.System.MemTotalBytes / (1 << 30)
	memPct := snap.System.MemPercent()
	swapMB := snap.System.SwapUsedBytes / (1 << 20)
	headroomGB := float64(s.Headroom.MemAvailBytes) / float64(1<<30)

	cpuStr := fmt.Sprintf("CPU:%03d%%", cpuPct)
	memStr := fmt.Sprintf("MEM:%02d/%02dGB", memUsedGB, memTotalGB)
	swapStr := fmt.Sprintf("SWAP:%05dMB", swapMB)
	headStr := fmt.Sprintf("head:~%04.1fGB", headroomGB)

	// Color the stats
	cpuColor := thresholdColor(snap.System.CPUPercent, 70, 90)
	memColor := thresholdColor(memPct, 60, 80)
	swapColor := lipgloss.Color("8")
	if swapMB > 0 {
		swapColor = lipgloss.Color("3")
	}
	if swapMB > 2048 {
		swapColor = lipgloss.Color("1")
	}

	sep := dim.Render("  |  ")

	rates := fmt.Sprintf(" %s  %s  %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(spawnStr),
		lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(deathStr),
		lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Render(netStr),
	)

	stats := fmt.Sprintf("%s  %s  %s  %s",
		lipgloss.NewStyle().Foreground(cpuColor).Render(cpuStr),
		lipgloss.NewStyle().Foreground(memColor).Render(memStr),
		lipgloss.NewStyle().Foreground(swapColor).Render(swapStr),
		dim.Render(headStr),
	)

	return rates + sep + stats
}
