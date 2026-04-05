package widgets

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
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

// Rates renders spawn/death/net rates + system stats, all fixed-width,
// plus a hierarchical session row showing per-session category-typed process glyphs.
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

	// Hierarchical session row: ◆ ▲▲●●◇·· [07]  ◆ ▲▲▲●■■◇ [08]
	sb.WriteString("\n ")
	sb.WriteString(renderSessionRow(snap.Sessions))

	return sb.String()
}

// sessionGroup holds categorized process counts for one session.
// Uses a fixed array indexed by category order instead of a map to avoid
// per-frame map allocations in the hot render path.
type sessionGroup struct {
	cats [numCategories]int
}

// numCategories is len(collector.Categories), known at compile time.
const numCategories = 5

// catIndex maps category name → fixed array index. Built once at init.
var catIndex map[string]int

func init() {
	catIndex = make(map[string]int, numCategories)
	for i, cat := range collector.Categories {
		catIndex[cat.Name] = i
	}
}

func (g *sessionGroup) total() int {
	t := 0
	for _, c := range g.cats {
		t += c
	}
	return t
}

// sessionGroupCounts categorizes each session's descendants by activity category.
func sessionGroupCounts(sessions []collector.SessionTree) []sessionGroup {
	if len(sessions) == 0 {
		return nil
	}
	groups := make([]sessionGroup, len(sessions))
	for i, sess := range sessions {
		for _, p := range sess.Descendants {
			cat, ok := collector.TypeToCategory[p.TypeCode]
			if !ok {
				cat = "shell"
			}
			if idx, ok := catIndex[cat]; ok {
				groups[i].cats[idx]++
			}
		}
	}
	return groups
}

// formatFixedCount returns a 2-digit fixed-width count, or "++" for >= 100.
func formatFixedCount(n int) string {
	if n >= 100 {
		return "++"
	}
	return fmt.Sprintf("%02d", n)
}

// renderSessionRow produces the hierarchical session display:
// ◆ ▲▲●●◇·· [07]  ◆ ▲▲▲●■■◇ [08]
func renderSessionRow(sessions []collector.SessionTree) string {
	groups := sessionGroupCounts(sessions)
	if len(groups) == 0 {
		return ui.DimText("no sessions")
	}

	var parts []string
	idle := 0
	for _, g := range groups {
		total := g.total()
		if total == 0 {
			idle++
			continue
		}

		var sb strings.Builder
		sb.WriteString(ui.ColorText(ui.CyanColor, ui.SessionGlyph))
		sb.WriteString(" ")

		for i, cat := range collector.Categories {
			n := g.cats[i]
			if n == 0 {
				continue
			}
			formatted := ui.CategoryGlyphFormatted[cat.Name]
			if formatted == "" {
				formatted = ui.DimText(ui.CategoryGlyphDefault)
			}
			sb.WriteString(strings.Repeat(formatted, n))
		}

		sb.WriteString(ui.DimText(" [" + formatFixedCount(total) + "]"))
		parts = append(parts, sb.String())
	}

	if idle > 0 {
		parts = append(parts, ui.DimText(fmt.Sprintf("+%d", idle)))
	}

	return strings.Join(parts, "  ")
}
