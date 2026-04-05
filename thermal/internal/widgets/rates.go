package widgets

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// Pointers to shared threshold vars from sparkline.go — no per-frame allocation.
var (
	ratesCPUThresh  = &CPUSparkThresh
	ratesMemThresh  = &MemSparkThresh
	ratesSwapThresh = &SwapSparkThresh
	ratesGPUThresh  = &GPUSparkThresh
)

// Rates renders spawn/death/net rates + system stats, all fixed-width:
// spawn:+003/s  death:-001/s  net:+002/s  |  CPU:034%  MEM:11.2/16.0GB  SWAP:00.0GB  GPU:005%
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
	swapGB := float64(snap.System.SwapUsedBytes) / float64(model.GB)
	gpuPct := int(snap.System.GPUPercent)

	var sb strings.Builder
	sb.Grow(256)

	// Gauge stats: " ●CPU:NNN% ●MEM:NN.N/NNGB ●SWAP:NN.NGB ●GPU:NNN%"
	sb.WriteString(" ")
	sb.WriteString(ui.GaugeDots[0].Formatted)
	sb.WriteString("CPU:")
	sb.WriteString(severityColor(snap.System.CPUPercent, ratesCPUThresh))
	sb.WriteString(fmt.Sprintf("%03d%%", cpuPct))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(ui.GaugeDots[1].Formatted)
	sb.WriteString("MEM:")
	sb.WriteString(severityColor(memPct, ratesMemThresh))
	sb.WriteString(fmt.Sprintf("%04.1f/%02dGB", memUsedGB, memTotalGB))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(ui.GaugeDots[2].Formatted)
	sb.WriteString("SWAP:")
	sb.WriteString(severityColor(swapGB, ratesSwapThresh))
	sb.WriteString(fmt.Sprintf("%04.1fGB", swapGB))
	sb.WriteString(sparkReset)

	sb.WriteString("  ")
	sb.WriteString(ui.GaugeDots[3].Formatted)
	sb.WriteString("GPU:")
	sb.WriteString(severityColor(snap.System.GPUPercent, ratesGPUThresh))
	sb.WriteString(fmt.Sprintf("%03d%%", gpuPct))
	sb.WriteString(sparkReset)

	// Separator
	sb.WriteString(ui.DimText("  |  "))

	// Rates: warm/cool/net
	sb.WriteString(ui.ColorText(lipgloss.Color("208"), spawnStr))
	sb.WriteString("  ")
	sb.WriteString(ui.ColorText(ui.CyanColor, deathStr))
	sb.WriteString("  ")
	sb.WriteString(ui.ColorText(lipgloss.Color("7"), netStr))

	// Help hint
	sb.WriteString("  ")
	sb.WriteString(ui.DimText("[h] help"))

	return sb.String()
}
