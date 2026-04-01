package panes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Heatmap renders a spectrogram: rows=types, columns=ticks, cell color=count intensity.
type Heatmap struct {
	// Ring buffer of snapshots. Each snapshot maps type -> count.
	snapshots []map[string]int
	maxSnaps  int
	// Sticky set of types seen, sorted alphabetically.
	knownTypes []string
	width      int
	height     int
}

func NewHeatmap() *Heatmap {
	return &Heatmap{
		maxSnaps: 120,
	}
}

func (h *Heatmap) SetSize(w, h2 int) {
	h.width = w
	h.height = h2
	// Recalculate max snapshots based on available columns
	labelMargin := 4 // "  N "
	cols := w - labelMargin
	if cols < 1 {
		cols = 1
	}
	h.maxSnaps = cols
	// Trim if needed
	if len(h.snapshots) > h.maxSnaps {
		h.snapshots = h.snapshots[len(h.snapshots)-h.maxSnaps:]
	}
}

func (h *Heatmap) Update(tick jsonl.Tick) {
	counts := tick.TypeCounts()
	h.snapshots = append(h.snapshots, counts)
	if len(h.snapshots) > h.maxSnaps {
		h.snapshots = h.snapshots[len(h.snapshots)-h.maxSnaps:]
	}

	// Update known types (sticky)
	for t := range counts {
		found := false
		for _, kt := range h.knownTypes {
			if kt == t {
				found = true
				break
			}
		}
		if !found {
			h.knownTypes = append(h.knownTypes, t)
			sort.Strings(h.knownTypes)
		}
	}
}

// cellBg returns an ANSI 256-color background escape for a count value.
func cellBg(count int) string {
	switch {
	case count == 0:
		return ""
	case count == 1:
		return "\033[48;5;232m"
	case count <= 3:
		return "\033[48;5;235m"
	case count <= 7:
		return "\033[48;5;130m"
	case count <= 15:
		return "\033[48;5;208m"
	default:
		return "\033[48;5;196m"
	}
}

func (h *Heatmap) View() string {
	if len(h.knownTypes) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("  waiting for data...")
	}

	var lines []string
	reset := "\033[0m"

	for _, t := range h.knownTypes {
		color := ui.TypeColor[t]
		label := lipgloss.NewStyle().Foreground(color).Render(t)
		var row strings.Builder
		row.WriteString("  ")
		row.WriteString(label)
		row.WriteString(" ")

		for _, snap := range h.snapshots {
			count := snap[t]
			bg := cellBg(count)
			if bg != "" {
				row.WriteString(bg)
				row.WriteString(" ")
				row.WriteString(reset)
			} else {
				row.WriteString(" ")
			}
		}

		// Pad remaining columns
		remaining := h.maxSnaps - len(h.snapshots)
		if remaining > 0 {
			row.WriteString(strings.Repeat(" ", remaining))
		}

		lines = append(lines, row.String())
	}

	// Pad remaining height
	for len(lines) < h.height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// Deltas tracks spawn/death counts between consecutive ticks.
type Deltas struct {
	prevPIDs map[int]bool
	Spawns   int
	Deaths   int
	Net      int
}

func NewDeltas() *Deltas {
	return &Deltas{}
}

func (d *Deltas) Update(tick jsonl.Tick) {
	currPIDs := make(map[int]bool, len(tick.Procs))
	for _, p := range tick.Procs {
		currPIDs[p.PID] = true
	}

	if d.prevPIDs != nil {
		d.Spawns = 0
		d.Deaths = 0
		for pid := range currPIDs {
			if !d.prevPIDs[pid] {
				d.Spawns++
			}
		}
		for pid := range d.prevPIDs {
			if !currPIDs[pid] {
				d.Deaths++
			}
		}
		d.Net = d.Spawns - d.Deaths
	} else {
		d.Spawns = tick.Spawns()
		d.Deaths = 0
		d.Net = d.Spawns
	}

	d.prevPIDs = currPIDs
}

// FormatRate formats a rate value with sign and color.
func FormatRate(val int, label string, warn, crit int) string {
	abs := val
	if abs < 0 {
		abs = -abs
	}
	color := ui.ThresholdColor(abs, warn, crit)
	style := lipgloss.NewStyle().Foreground(color)
	return style.Render(fmt.Sprintf("%s%d/s", label, abs))
}
