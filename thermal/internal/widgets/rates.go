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
		durStr := "just now"
		if s.OfflineDuration >= time.Minute {
			durStr = fmt.Sprintf("%dm %ds", int(s.OfflineDuration.Minutes()), int(s.OfflineDuration.Seconds())%60)
		} else if s.OfflineDuration > 0 {
			durStr = fmt.Sprintf("%ds", int(s.OfflineDuration.Seconds()))
		}
		return " " + ui.ColorText(ui.CyanColor, fmt.Sprintf("OFFLINE %s", durStr)) +
			ui.DimText("  —  no API connection, processes will wind down")
	}

	// Warm/cool/net — fixed width with sign
	spawnStr := fmt.Sprintf("warm:%+04d/s", s.LastSpawns())
	deathStr := fmt.Sprintf("cool:-%03d/s", s.LastDeaths())
	netVal := int(s.NetRate)
	netStr := fmt.Sprintf("net:%+04d/s", netVal)

	// System stats — fixed width
	cpuPct := int(snap.System.CPUPercent)
	memUsedGB := float64(snap.System.MemUsedBytes) / float64(model.GB)
	memTotalGB := snap.System.MemTotalBytes / int64(model.GB)
	memPct := snap.System.MemPercent()
	swapMB := snap.System.SwapUsedBytes / int64(model.MB)

	cpuDot := ui.GaugeDots[0].ANSI + ui.GaugeDots[0].Char + "\033[0m "
	memDot := ui.GaugeDots[1].ANSI + ui.GaugeDots[1].Char + "\033[0m "
	swapDot := ui.GaugeDots[2].ANSI + ui.GaugeDots[2].Char + "\033[0m "

	cpuStr := fmt.Sprintf("%sCPU:%03d%%", cpuDot, cpuPct)
	memStr := fmt.Sprintf("%sMEM:%04.1f/%02dGB", memDot, memUsedGB, memTotalGB)
	swapStr := fmt.Sprintf("%sSWAP:%05dMB", swapDot, swapMB)

	// Color the stats
	cpuColor := ui.ThresholdColor(snap.System.CPUPercent, 70, 90)
	memColor := ui.ThresholdColor(memPct, 60, 80)
	swapColor := ui.ThresholdColor(float64(swapMB), 1, 2048)

	sep := ui.DimText("  |  ")

	stats := fmt.Sprintf(" %s  %s  %s",
		ui.ColorText(cpuColor, cpuStr),
		ui.ColorText(memColor, memStr),
		ui.ColorText(swapColor, swapStr),
	)

	rates := fmt.Sprintf("%s  %s  %s",
		ui.ColorText(lipgloss.Color("208"), spawnStr),
		ui.ColorText(ui.CyanColor, deathStr),
		ui.ColorText(lipgloss.Color("7"), netStr),
	)

	help := ui.DimText("[h] help")

	return stats + sep + rates + "  " + help
}
