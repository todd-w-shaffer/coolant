package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

// Dot counts for the three animation preview rows.
const (
	animActiveDots      = 4
	animStaleDots       = 5
	animHighscoreActive = 2
	animHighscoreDone   = 4
	animDuration        = 3 * time.Second
)

type animTickMsg time.Time
type animDoneMsg struct{}

// animSection pairs a label with its BreatheDots widget for table-driven rendering.
type animSection struct {
	label string
	dots  *widgets.BreatheDots
}

// animModel is the bubbletea model for --animate preview mode.
type animModel struct {
	header   string // pre-computed header (theme/profile names)
	sections []animSection
}

func newAnimModel(th *theme.Theme, ap *anim.Profile) animModel {
	active := widgets.NewBreatheDots(th, ap)
	active.SetTarget(animActiveDots)

	stale := widgets.NewBreatheDots(th, ap)
	stale.SetTarget(animStaleDots)
	stale.SetStaleCount(animStaleDots)

	highscore := widgets.NewBreatheDots(th, ap)
	highscore.SetHighScoreMode(true)
	highscore.SetTarget(animHighscoreActive)
	hsIDs := make([]string, animHighscoreDone)
	for i := range hsIDs {
		hsIDs[i] = fmt.Sprintf("demo-%d", i)
	}
	highscore.SetCompletedAgents(hsIDs)

	return animModel{
		header: swatchHeader(fmt.Sprintf("animate: %s / %s", th.Name, ap.Name)),
		sections: []animSection{
			{"Tidal Wave", active},
			{"KITT Scanner", stale},
			{"Highscore", highscore},
		},
	}
}

func (m animModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(config.AnimInterval, func(t time.Time) tea.Msg { return animTickMsg(t) }),
		tea.Tick(animDuration, func(time.Time) tea.Msg { return animDoneMsg{} }),
	)
}

func (m animModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return m, tea.Quit
	case animDoneMsg:
		return m, tea.Quit
	case animTickMsg:
		m.tick()
		return m, tea.Tick(config.AnimInterval, func(t time.Time) tea.Msg { return animTickMsg(t) })
	}
	return m, nil
}

// tick advances all animation groups by one frame.
func (m *animModel) tick() {
	for _, sec := range m.sections {
		sec.dots.AnimTick()
	}
}

func (m animModel) View() tea.View {
	return tea.View{Content: m.view()}
}

// view returns the rendered string (used by both View() and tests).
func (m animModel) view() string {
	var sb strings.Builder
	sb.WriteString(m.header)

	for _, sec := range m.sections {
		sb.WriteString(dimLabel(sec.label))
		sb.WriteString("\n  ")
		s, _ := sec.dots.Render(ui.AgentGlyphHollow, ui.AgentGlyphMid, ui.AgentGlyphFilled, nil, 0)
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

// runAnimate launches the bubbletea animate preview.
func runAnimate(th *theme.Theme, ap *anim.Profile) error {
	m := newAnimModel(th, ap)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
