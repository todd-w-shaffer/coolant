package widgets

import (
	"fmt"
	"strings"

	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// Alerts renders a scrolling alert log.
type Alerts struct {
	width  int
	height int
	state  *model.AppState
	theme  *theme.Theme
}

func NewAlerts(th *theme.Theme) *Alerts {
	return &Alerts{theme: th}
}

func (a *Alerts) SetSize(w, h int) {
	a.width = w
	a.height = h
}

func (a *Alerts) Update(state *model.AppState) {
	a.state = state
}

func (a *Alerts) View() string {
	if a.state == nil || a.state.Alerts.Len() == 0 {
		return ""
	}

	visible := a.height
	if visible <= 0 {
		visible = 2
	}
	recent := a.state.Alerts.Last(visible)

	var lines []string
	for _, alert := range recent {
		ts := ui.DimText(alert.Time.Format("15:04:05"))
		color := a.theme.ThreatColors[alert.Level]
		msg := ui.ColorText(color, alert.Message)
		lines = append(lines, fmt.Sprintf(" %s  %s", ts, msg))
	}

	return strings.Join(lines, "\n")
}
