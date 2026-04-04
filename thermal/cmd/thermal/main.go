package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/demo"
	"github.com/toddwshaffer/coolant/thermal/internal/layout"
)

// ── Messages ────────────────────────────────────────────────

type snapshotMsg collector.Snapshot
type animTickMsg time.Time
type gateEventMsg collector.GateEvent

// ── Model ───────────────────────────────────────────────────

type model struct {
	width     int
	height    int
	layout    *layout.Horizontal
	done      chan struct{}
	demoMode  bool
	snapChan  chan collector.Snapshot
	eventChan chan collector.GateEvent
}

func newModel(demoMode bool) model {
	return model{
		layout:    layout.NewHorizontal(),
		done:      make(chan struct{}),
		demoMode:  demoMode,
		snapChan:  make(chan collector.Snapshot, 16),
		eventChan: make(chan collector.GateEvent, 32),
	}
}

func (m model) Init() tea.Cmd {
	if m.demoMode {
		go demo.RunV2(m.snapChan, 250*time.Millisecond, m.done)
	} else {
		go collector.Run(m.snapChan, config.FastInterval, m.done)
	}

	// Start JSONL event tailer — path syncs with COOLANT_EVENTS in common.sh
	evPath := os.Getenv("COOLANT_EVENTS")
	if evPath == "" {
		tmpDir := os.Getenv("TMPDIR")
		if tmpDir == "" {
			tmpDir = "/tmp/"
		}
		evPath = tmpDir + "coolant-" + currentUser() + ".events.jsonl"
	}
	go collector.TailEvents(m.eventChan, evPath, config.EventInterval, m.done)

	return tea.Batch(waitForSnapshot(m.snapChan), waitForEvent(m.eventChan), animTick())
}

func animTick() tea.Cmd {
	return tea.Tick(config.AnimInterval, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
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

func waitForEvent(ch <-chan collector.GateEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return gateEventMsg(ev)
	}
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			close(m.done)
			return m, tea.Quit
		case "h":
			m.layout.ToggleHelp()
		case "c":
			m.layout.ToggleCollapse()
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

	case gateEventMsg:
		ev := collector.GateEvent(msg)
		m.layout.State().HandleEvent(ev)
		return m, waitForEvent(m.eventChan)

	case animTickMsg:
		m.layout.AnimTick()
		return m, animTick()
	}

	return m, nil
}

func (m model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if m.width == 0 || m.height == 0 {
		return v
	}
	v.Content = m.layout.View()
	return v
}

// ── Main ────────────────────────────────────────────────────

func main() {
	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	_ = flag.Bool("horizontal", false, "Horizontal strip layout (accepted for backward compat, now default)")
	_ = flag.Bool("vertical", false, "Vertical panel layout (WIP)")
	flag.Parse()

	m := newModel(*demoMode)

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
