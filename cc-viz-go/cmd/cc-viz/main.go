package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/demo"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/panes"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// ── Messages ────────────────────────────────────────────────

type tickMsg jsonl.Tick

// ── Model ───────────────────────────────────────────────────

type model struct {
	width  int
	height int

	// Panes
	heatmap   *panes.Heatmap
	waveform  *panes.Waveform
	waterfall *panes.Waterfall
	breakdown *panes.Breakdown
	phaseRing *panes.PhaseRing
	alertLog  *panes.AlertLog
	deltas    *panes.Deltas

	// State
	lastTick jsonl.Tick
	dataPath string
	done     chan struct{}
}

func newModel(dataPath string) model {
	return model{
		heatmap:   panes.NewHeatmap(),
		waveform:  panes.NewWaveform(),
		waterfall: panes.NewWaterfall(),
		breakdown: panes.NewBreakdown(),
		phaseRing: panes.NewPhaseRing(20),
		alertLog:  panes.NewAlertLog(),
		deltas:    panes.NewDeltas(),
		dataPath:  dataPath,
		done:      make(chan struct{}),
	}
}

var tickChan chan jsonl.Tick

func (m model) Init() tea.Cmd {
	tickChan = make(chan jsonl.Tick, 16)
	go jsonl.Tail(m.dataPath, tickChan, m.done)
	return waitForTick()
}

func waitForTick() tea.Cmd {
	return func() tea.Msg {
		tick, ok := <-tickChan
		if !ok {
			return nil
		}
		return tickMsg(tick)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			close(m.done)
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case tickMsg:
		tick := jsonl.Tick(msg)
		m.lastTick = tick

		// Update deltas
		m.deltas.Update(tick)

		// Update all panes
		m.heatmap.Update(tick)
		m.waveform.Update(m.deltas.Spawns, m.deltas.Deaths, m.deltas.Net)

		// Phase classification
		spawns := tick.Spawns()
		phase := panes.ClassifyPhase(tick.Count, spawns, m.deltas.Net)
		m.phaseRing.Push(phase)
		m.alertLog.Check(tick.Count, spawns, m.deltas.Net, phase)

		return m, waitForTick()
	}

	return m, nil
}

func (m *model) updateSizes() {
	if m.width < 4 || m.height < 4 {
		return
	}
	halfW := m.width / 2
	leftW := halfW
	rightW := m.width - halfW

	statusH := 2
	gridH := m.height - statusH
	if gridH < 2 {
		gridH = 2
	}
	topH := gridH / 2
	bottomH := gridH - topH

	m.heatmap.SetSize(leftW, topH)
	m.waveform.SetSize(rightW, topH)
	m.waterfall.SetSize(leftW, bottomH)
	m.breakdown.SetSize(rightW, bottomH)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	topLeft := m.heatmap.View()
	topRight := m.waveform.View()
	bottomLeft := m.waterfall.View(m.lastTick)
	bottomRight := m.breakdown.View(m.lastTick)

	// Status row: phase ring + alert log last entry
	statusRow := fmt.Sprintf("  %s", m.phaseRing.View())
	alertView := m.alertLog.View(1)
	if alertView != "" {
		statusRow += "\n" + alertView
	}

	return ui.RenderGrid(topLeft, topRight, bottomLeft, bottomRight, statusRow, m.width, m.height)
}

// ── Main ────────────────────────────────────────────────────

func main() {
	dataPath := flag.String("data", "/tmp/cc-procs.jsonl", "Path to JSONL data file")
	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	flag.Parse()

	if *demoMode {
		done := make(chan struct{})
		go demo.Run(*dataPath, 1*time.Second, done)
		defer close(done)
		// Give demo a moment to write first line
		time.Sleep(200 * time.Millisecond)
	}

	m := newModel(*dataPath)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
