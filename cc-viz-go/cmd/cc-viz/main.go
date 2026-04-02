package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/collector"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/demo"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/layout"
)

// ── Messages ────────────────────────────────────────────────

type snapshotMsg collector.Snapshot

// ── Model ───────────────────────────────────────────────────

type model struct {
	width    int
	height   int
	layout   *layout.Horizontal
	done     chan struct{}
	demoMode bool
	snapChan chan collector.Snapshot
}

func newModel(demoMode bool) model {
	return model{
		layout:   layout.NewHorizontal(),
		done:     make(chan struct{}),
		demoMode: demoMode,
		snapChan: make(chan collector.Snapshot, 16),
	}
}

func (m model) Init() tea.Cmd {
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
		m.layout.SetSize(m.width, m.height)

	case snapshotMsg:
		snap := collector.Snapshot(msg)
		m.layout.State().Update(snap)
		m.layout.Update(m.layout.State())
		return m, waitForSnapshot(m.snapChan)
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	return m.layout.View()
}

// ── Main ────────────────────────────────────────────────────

func main() {
	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	_ = flag.Bool("horizontal", false, "Horizontal strip layout (accepted for backward compat, now default)")
	_ = flag.Bool("vertical", false, "Vertical panel layout (WIP)")
	flag.Parse()

	m := newModel(*demoMode)

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
