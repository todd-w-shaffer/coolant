package widgets

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Fixed type display order: heavy → light. Never reshuffles.
var typeOrder = []string{"V", "N", "T", "P", "G", "R", "F", "S", "C", "X"}

// ProcBar renders a compact process type breakdown using smoothed counts:
// N:18 ██████████████████  V:12 ████████████  T:8 ████████
type ProcBar struct {
	width int
	state *model.AppState
}

func NewProcBar() *ProcBar {
	return &ProcBar{}
}

func (p *ProcBar) SetSize(w, h int) {
	p.width = w
}

func (p *ProcBar) Update(state *model.AppState) {
	p.state = state
}

func (p *ProcBar) View() string {
	if p.state == nil || len(p.state.SmoothedCounts) == 0 {
		return ""
	}

	// Collect visible types in fixed order, skip zeros
	type entry struct {
		code  string
		count int // rounded from smoothed
	}
	var entries []entry
	maxCount := 0
	for _, code := range typeOrder {
		smoothed, ok := p.state.SmoothedCounts[code]
		if !ok || smoothed < 0.5 {
			continue
		}
		count := int(math.Round(smoothed))
		entries = append(entries, entry{code, count})
		if count > maxCount {
			maxCount = count
		}
	}

	if maxCount == 0 || len(entries) == 0 {
		return ""
	}

	// Calculate bar widths
	availWidth := p.width - 2 // margins
	perEntryOverhead := 7     // "X:NN " + "  "
	totalOverhead := len(entries) * perEntryOverhead
	barBudget := availWidth - totalOverhead
	if barBudget < len(entries) {
		barBudget = len(entries)
	}
	maxBarLen := barBudget / len(entries)
	if maxBarLen < 1 {
		maxBarLen = 1
	}

	var parts []string
	for _, e := range entries {
		color := ui.TypeColor[e.code]
		if color == "" {
			color = lipgloss.Color("8")
		}

		barLen := e.count * maxBarLen / maxCount
		if barLen < 1 && e.count > 0 {
			barLen = 1
		}

		label := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%s:%d", e.code, e.count))
		bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", barLen))

		parts = append(parts, fmt.Sprintf("%s %s", label, bar))
	}

	return " " + strings.Join(parts, "  ")
}
