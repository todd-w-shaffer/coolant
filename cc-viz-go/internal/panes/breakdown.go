package panes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Breakdown renders a horizontal bar chart of processes grouped by type.
type Breakdown struct {
	width  int
	height int
}

func NewBreakdown() *Breakdown {
	return &Breakdown{}
}

func (b *Breakdown) SetSize(w, h int) {
	b.width = w
	b.height = h
}

type typeCount struct {
	Type  string
	Count int
}

func (b *Breakdown) View(tick jsonl.Tick) string {
	if tick.Count == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("  waiting for data...")
	}

	counts := tick.TypeCounts()

	// Sort by count descending
	var sorted []typeCount
	for t, c := range counts {
		sorted = append(sorted, typeCount{t, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})

	maxCount := 0
	total := 0
	for _, tc := range sorted {
		if tc.Count > maxCount {
			maxCount = tc.Count
		}
		total += tc.Count
	}

	barWidth := b.width - 10 // "  N " + " NNN"
	if barWidth < 1 {
		barWidth = 1
	}

	var lines []string

	for _, tc := range sorted {
		color := ui.TypeColor[tc.Type]
		label := lipgloss.NewStyle().Foreground(color).Render(tc.Type)

		filled := 0
		if maxCount > 0 {
			filled = tc.Count * barWidth / maxCount
			if filled < 1 && tc.Count > 0 {
				filled = 1
			}
		}
		empty := barWidth - filled

		line := fmt.Sprintf("  %s %s%s %d",
			label,
			lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled)),
			lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat("░", empty)),
			tc.Count,
		)
		lines = append(lines, line)
	}

	// Footer
	lines = append(lines, "")
	totalColor := ui.ThresholdColor(total, ui.TotalWarn, ui.TotalCrit)
	footer := fmt.Sprintf("  %s    types: %d",
		lipgloss.NewStyle().Foreground(totalColor).Render(fmt.Sprintf("total: %d", total)),
		len(sorted),
	)
	lines = append(lines, footer)

	for len(lines) < b.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
