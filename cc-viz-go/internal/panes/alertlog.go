package panes

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// AlertEntry is a single log entry.
type AlertEntry struct {
	Time    time.Time
	Message string
	Level   int // 0=info, 1=warn, 2=crit
}

// AlertLog tracks events and renders a scrolling log.
type AlertLog struct {
	entries   []AlertEntry
	maxLines  int
	prevTotal int
	prevPhase Phase
}

func NewAlertLog() *AlertLog {
	return &AlertLog{
		maxLines:  50,
		prevPhase: PhaseCalm,
	}
}

// Check evaluates current state and generates alerts.
func (a *AlertLog) Check(total, spawns, net int, phase Phase) {
	now := time.Now()

	// Phase transitions
	if phase != a.prevPhase {
		level := 0
		if phase == PhaseExploding {
			level = 2
		} else if phase == PhaseRamping {
			level = 1
		}
		a.add(AlertEntry{
			Time:    now,
			Message: fmt.Sprintf("phase: %s → %s", a.prevPhase, phase),
			Level:   level,
		})
		a.prevPhase = phase
	}

	// Total threshold crossings
	if a.prevTotal < ui.TotalCrit && total >= ui.TotalCrit {
		a.add(AlertEntry{now, fmt.Sprintf("total %d crossed CRIT (%d)", total, ui.TotalCrit), 2})
	} else if a.prevTotal < ui.TotalWarn && total >= ui.TotalWarn {
		a.add(AlertEntry{now, fmt.Sprintf("total %d crossed WARN (%d)", total, ui.TotalWarn), 1})
	}

	// Spawn burst
	if spawns >= ui.SpawnCrit {
		a.add(AlertEntry{now, fmt.Sprintf("spawn burst: %d/s (crit: %d)", spawns, ui.SpawnCrit), 2})
	}

	a.prevTotal = total
}

func (a *AlertLog) add(e AlertEntry) {
	a.entries = append(a.entries, e)
	if len(a.entries) > a.maxLines {
		a.entries = a.entries[len(a.entries)-a.maxLines:]
	}
}

func (a *AlertLog) View(height int) string {
	if len(a.entries) == 0 {
		return ""
	}

	// Show last N entries that fit
	visible := height
	if visible > len(a.entries) {
		visible = len(a.entries)
	}
	start := len(a.entries) - visible

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var lines []string
	for _, e := range a.entries[start:] {
		ts := dim.Render(e.Time.Format("15:04:05"))
		var msgStyle lipgloss.Style
		switch e.Level {
		case 2:
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		case 1:
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
		default:
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
		}
		lines = append(lines, fmt.Sprintf("  %s  %s", ts, msgStyle.Render(e.Message)))
	}

	return strings.Join(lines, "\n")
}
