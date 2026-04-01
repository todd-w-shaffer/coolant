package panes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Waterfall renders per-process lifetime bars.
type Waterfall struct {
	width  int
	height int
}

func NewWaterfall() *Waterfall {
	return &Waterfall{}
}

func (w *Waterfall) SetSize(width, height int) {
	w.width = width
	w.height = height
}

func (w *Waterfall) View(tick jsonl.Tick) string {
	if tick.Count == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("  waiting for data...")
	}

	// Header
	totalColor := ui.ThresholdColor(tick.Count, ui.TotalWarn, ui.TotalCrit)
	header := fmt.Sprintf("  WATERFALL ── %s alive",
		lipgloss.NewStyle().Foreground(totalColor).Bold(true).Render(fmt.Sprintf("%d", tick.Count)),
	)

	// Sort processes by age descending (oldest first)
	procs := make([]jsonl.Proc, len(tick.Procs))
	copy(procs, tick.Procs)
	sort.Slice(procs, func(i, j int) bool {
		return procs[i].Age > procs[j].Age
	})

	// Available rows for process bars
	availRows := w.height - 2 // header + padding
	if availRows < 1 {
		availRows = 1
	}

	// Bar width
	barWidth := w.width - 6 // "  N " + padding
	if barWidth < 1 {
		barWidth = 1
	}

	// Max age for scaling
	maxAge := 1
	if len(procs) > 0 && procs[0].Age > 0 {
		maxAge = procs[0].Age
	}

	var lines []string
	lines = append(lines, header)

	// Overflow: show newest processes, skip oldest
	overflow := 0
	displayProcs := procs
	if len(procs) > availRows {
		overflow = len(procs) - availRows + 1 // +1 for overflow indicator
		displayProcs = procs[overflow:]
	}

	if overflow > 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(fmt.Sprintf("   (%d more above)", overflow)))
	}

	reset := "\033[0m"

	for _, p := range displayProcs {
		typeColor := ui.TypeColor[p.Type]

		// Age-based intensity
		var prefix, suffix string
		switch {
		case p.Age <= 2:
			prefix = "\033[1m" // bold
			suffix = reset
		case p.Age <= 10:
			prefix = ""
			suffix = ""
		case p.Age <= 30:
			prefix = "\033[2m" // dim
			suffix = reset
		default:
			prefix = "\033[2m"
			suffix = reset
			typeColor = lipgloss.Color("8") // gray for very old
		}

		// Bar length proportional to age
		filled := p.Age * barWidth / maxAge
		if filled < 1 && p.Age > 0 {
			filled = 1
		}
		empty := barWidth - filled

		label := lipgloss.NewStyle().Foreground(typeColor).Render(p.Type)
		barColor := lipgloss.NewStyle().Foreground(typeColor)

		var row strings.Builder
		row.WriteString("  ")
		row.WriteString(prefix)
		row.WriteString(label)
		row.WriteString(" ")
		row.WriteString(barColor.Render(strings.Repeat("█", filled)))
		row.WriteString(suffix)
		row.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat("░", empty)))

		lines = append(lines, row.String())
	}

	// Pad remaining height
	for len(lines) < w.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}
