package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/collector"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/demo"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/layout"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/panes"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// ── Messages ────────────────────────────────────────────────

type tickMsg jsonl.Tick
type snapshotMsg collector.Snapshot

// ── Legacy Model (old 2x2 grid) ────────────────────────────

type legacyModel struct {
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

func newLegacyModel(dataPath string) legacyModel {
	return legacyModel{
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

func (m legacyModel) Init() tea.Cmd {
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

func (m legacyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.deltas.Update(tick)
		m.heatmap.Update(tick)
		m.waveform.Update(m.deltas.Spawns, m.deltas.Deaths, m.deltas.Net)
		spawns := tick.Spawns()
		phase := panes.ClassifyPhase(tick.Count, spawns, m.deltas.Net)
		m.phaseRing.Push(phase)
		m.alertLog.Check(tick.Count, spawns, m.deltas.Net, phase)
		return m, waitForTick()
	}

	return m, nil
}

func (m *legacyModel) updateSizes() {
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

func (m legacyModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	topLeft := m.heatmap.View()
	topRight := m.waveform.View()
	bottomLeft := m.waterfall.View(m.lastTick)
	bottomRight := m.breakdown.View(m.lastTick)
	statusRow := fmt.Sprintf("  %s", m.phaseRing.View())
	alertView := m.alertLog.View(1)
	if alertView != "" {
		statusRow += "\n" + alertView
	}
	return ui.RenderGrid(topLeft, topRight, bottomLeft, bottomRight, statusRow, m.width, m.height)
}

// ── New Model (horizontal/vertical layouts) ─────────────────

type v2Model struct {
	width    int
	height   int
	layout   *layout.Horizontal
	done     chan struct{}
	demoMode bool
	snapChan chan collector.Snapshot
}

func newV2Model(demoMode bool) v2Model {
	return v2Model{
		layout:   layout.NewHorizontal(),
		done:     make(chan struct{}),
		demoMode: demoMode,
		snapChan: make(chan collector.Snapshot, 16),
	}
}

func (m v2Model) Init() tea.Cmd {
	if m.demoMode {
		go demo.RunV2(m.snapChan, 250*time.Millisecond, m.done)
	} else {
		go collector.Run(m.snapChan, 500*time.Millisecond, m.done)
	}
	return waitForSnapshot(m.snapChan)
}

func waitForSnapshot(ch <-chan collector.Snapshot) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return nil
		}
		return snapshotMsg(snap)
	}
}

func (m v2Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.layout.SetSize(m.width, m.height)

	case snapshotMsg:
		snap := collector.Snapshot(msg)
		m.layout.State().Update(snap)
		m.layout.Update(m.layout.State())
		return m, waitForSnapshot(m.snapChan)
	}

	return m, nil
}

func (m v2Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	return m.layout.View()
}

// ── Main ────────────────────────────────────────────────────

func main() {
	dataPath := flag.String("data", "/tmp/cc-procs.jsonl", "Path to JSONL data file")
	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	horizontal := flag.Bool("horizontal", false, "Horizontal strip layout (bottom tmux pane)")
	vertical := flag.Bool("vertical", false, "Vertical panel layout (right tmux pane)")
	flag.Parse()

	var m tea.Model

	if *horizontal || *vertical {
		// New v2 layout
		m = newV2Model(*demoMode)
	} else {
		// Legacy 2x2 grid
		if *demoMode {
			done := make(chan struct{})
			go demo.Run(*dataPath, 250*time.Millisecond, done)
			defer close(done)
			time.Sleep(200 * time.Millisecond)
		}
		m = newLegacyModel(*dataPath)
	}

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
