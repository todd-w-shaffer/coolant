package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/demo"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/layout"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
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
	keys      keys.KeyMap
	done      chan struct{}
	demoMode  bool
	snapChan  chan collector.Snapshot
	eventChan chan collector.GateEvent
}

func newModel(demoMode bool, th *theme.Theme, ap *anim.Profile) model {
	km := keys.Default()
	return model{
		layout:    layout.NewHorizontal(th, ap, km),
		keys:      km,
		done:      make(chan struct{}),
		demoMode:  demoMode,
		snapChan:  make(chan collector.Snapshot, 16),
		eventChan: make(chan collector.GateEvent, 32),
	}
}

func (m model) Init() tea.Cmd {
	if m.demoMode {
		go demo.RunV2(m.snapChan, m.eventChan, 250*time.Millisecond, m.done)
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
	case parentExitMsg:
		close(m.done)
		return m, tea.Quit

	case tea.KeyPressMsg:
		// Full-help mode: any key dismisses without dispatching its action.
		if m.layout.HelpMode() == layout.HelpFull {
			m.layout.DismissHelp()
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			close(m.done)
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.layout.ToggleHelp()
		case key.Matches(msg, m.keys.Collapse):
			m.layout.ToggleCollapse()
		case key.Matches(msg, m.keys.PurgeStale):
			m.layout.State().PurgeStaleAgents()
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
	themeName := flag.String("theme", "", "Color theme (classic, iron, mono, frappe)")
	animName := flag.String("animation", "", "Animation profile (default, calm, intense)")
	listThemes := flag.Bool("list-themes", false, "List available themes and exit")
	listAnims := flag.Bool("list-animations", false, "List available animation profiles and exit")
	kittHighScore := flag.Bool("kitt-highscore", false, "KITT scans completed agents instead of ghosts")
	flag.Parse()

	// Load user config: COOLANT_CONFIG env > ~/.config/coolant/config.toml > defaults
	cfgPath := os.Getenv("COOLANT_CONFIG")
	if cfgPath == "" {
		home, _ := os.UserHomeDir()
		cfgPath = filepath.Join(home, ".config", "coolant", "config.toml")
	}
	if err := config.Load(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *listThemes {
		for _, name := range theme.Names() {
			fmt.Println(name)
		}
		os.Exit(0)
	}

	if *listAnims {
		for _, name := range anim.Names() {
			fmt.Println(name)
		}
		os.Exit(0)
	}

	// Resolve theme: flag > env > default
	name := *themeName
	if name == "" {
		name = os.Getenv("COOLANT_THEME")
	}
	if name == "" {
		name = "classic"
	}
	th, err := theme.Get(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Resolve highscore mode: flag > env > off
	highScore := *kittHighScore
	if !highScore && os.Getenv("COOLANT_KITT_HIGHSCORE") == "1" {
		highScore = true
	}

	// Resolve animation: flag > env > default
	animN := *animName
	if animN == "" {
		animN = os.Getenv("COOLANT_ANIMATION")
	}
	if animN == "" {
		animN = "default"
	}
	ap, err := anim.Get(animN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m := newModel(*demoMode, th, ap)
	if highScore {
		m.layout.SetHighScoreMode(true)
	}

	p := tea.NewProgram(m)

	go watchParent(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
