package widgets

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// Alerts renders a scrolling alert log.
type Alerts struct {
	width  int
	height int
	state  *model.AppState
}

func NewAlerts() *Alerts {
	return &Alerts{}
}

func (a *Alerts) SetSize(w, h int) {
	a.width = w
	a.height = h
}

func (a *Alerts) Update(state *model.AppState) {
	a.state = state
}

func (a *Alerts) View() string {
	if a.state == nil || len(a.state.Alerts) == 0 {
		return ""
	}

	visible := a.height
	if visible <= 0 {
		visible = 2
	}
	if visible > len(a.state.Alerts) {
		visible = len(a.state.Alerts)
	}
	start := len(a.state.Alerts) - visible

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var lines []string
	for _, alert := range a.state.Alerts[start:] {
		ts := dim.Render(alert.Time.Format("15:04:05"))
		color := ThreatColor[alert.Level]
		msg := lipgloss.NewStyle().Foreground(color).Render(alert.Message)
		lines = append(lines, fmt.Sprintf(" %s  %s", ts, msg))
	}

	return strings.Join(lines, "\n")
}
