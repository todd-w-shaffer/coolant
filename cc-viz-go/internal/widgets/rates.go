package widgets

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/model"
)

// Rates renders spawn/death/net rates and optional memory projection warning.
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
	if r.state == nil {
		return ""
	}
	s := r.state

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	spawns := green.Render(fmt.Sprintf("spawn +%d/s", s.LastSpawns()))
	deaths := red.Render(fmt.Sprintf("death -%d/s", s.LastDeaths()))

	netSign := "+"
	netColor := green
	netVal := int(s.NetRate)
	if netVal < 0 {
		netSign = ""
		netColor = red
	}
	net := netColor.Render(fmt.Sprintf("net %s%d/s", netSign, netVal))

	line := fmt.Sprintf(" %s  %s  %s", spawns, deaths, net)

	// Append projection warning if present
	if s.Headroom.Warning != "" {
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(s.Headroom.Warning)
		line += dim.Render("  |  ") + warn
	}

	return line
}
