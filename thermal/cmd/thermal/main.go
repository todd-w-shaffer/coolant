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
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"

	"github.com/toddwshaffer/coolant/thermal/internal/demo"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/layout"
	appmodel "github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
	"github.com/toddwshaffer/coolant/thermal/internal/updater"
	"github.com/toddwshaffer/coolant/thermal/internal/version"
)

// ── Messages ────────────────────────────────────────────────

type snapshotMsg collector.Snapshot
type animTickMsg time.Time
type gateEventMsg collector.GateEvent
type updateAvailableMsg string

// ── Model ───────────────────────────────────────────────────

type model struct {
	width         int
	height        int
	layout        *layout.Horizontal
	keys          keys.KeyMap
	done          chan struct{}
	demoMode      bool
	mouseEnabled  bool
	tmuxHintShown bool
	lastFocusTime time.Time // debounce: swallow agent-zone clicks within 300ms
	snapChan      chan collector.Snapshot
	eventChan     chan collector.GateEvent
	updateChan    chan string
}

func newModel(demoMode bool, th *theme.Theme, ap *anim.Profile) model {
	km := keys.Default()
	return model{
		layout:       layout.NewHorizontal(th, ap, km),
		keys:         km,
		done:         make(chan struct{}),
		demoMode:     demoMode,
		mouseEnabled: true,
		snapChan:     make(chan collector.Snapshot, 16),
		eventChan:    make(chan collector.GateEvent, 32),
		updateChan:   make(chan string, 1),
	}
}

func (m model) Init() tea.Cmd {
	if m.demoMode {
		go demo.RunV2(m.snapChan, m.eventChan, 250*time.Millisecond, m.done)
	} else {
		go collector.Run(m.snapChan, config.FastInterval, m.done)
	}

	evPath := os.Getenv("COOLANT_EVENTS")
	if evPath == "" {
		evPath = coolantTmpPath("events.jsonl")
	}
	go collector.TailEvents(m.eventChan, evPath, config.EventInterval, m.done)

	cmds := []tea.Cmd{waitForSnapshot(m.snapChan), waitForEvent(m.eventChan), animTick()}

	if !config.C.Updates.Disabled {
		go func() {
			defer close(m.updateChan)
			cachePath := coolantTmpPath("latest-version")
			latest, avail := updater.Check(version.Version, cachePath, config.C.Updates.CheckIntervalSec)
			if avail {
				m.updateChan <- latest
			}
		}()
		cmds = append(cmds, waitForUpdate(m.updateChan))
	}

	return tea.Batch(cmds...)
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

func waitForUpdate(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		v, ok := <-ch
		if !ok {
			return nil
		}
		return updateAvailableMsg(v)
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

func coolantTmpPath(suffix string) string {
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = "/tmp/"
	}
	return tmpDir + "coolant-" + currentUser() + "." + suffix
}

// agentUnderCursor returns the agent ID whose zone contains the mouse
// position, or "" if none match. Shared by hover and click handlers.
func agentUnderCursor(ids []string, msg tea.MouseMsg) string {
	for _, id := range ids {
		if zi := zone.Get(ui.AgentZoneID(id)); zi != nil && zi.InBounds(msg) {
			return id
		}
	}
	return ""
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case parentExitMsg:
		close(m.done)
		return m, tea.Quit

	case tea.KeyPressMsg:
		// Intel mode: `i` cycles depth, any other key dismisses entirely.
		if m.layout.IntelMode() {
			if key.Matches(msg, m.keys.Intel) {
				m.layout.ToggleIntel()
			} else {
				m.layout.DismissIntel()
			}
			return m, nil
		}
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
		case key.Matches(msg, m.keys.CategoryNext):
			m.layout.State().CycleCategoryFilter(+1)
		case key.Matches(msg, m.keys.CategoryPrev):
			m.layout.State().CycleCategoryFilter(-1)
		case key.Matches(msg, m.keys.CategoryClear):
			m.layout.State().SetCategoryFilter("")
		case key.Matches(msg, m.keys.MouseToggle):
			m.mouseEnabled = !m.mouseEnabled
		case key.Matches(msg, m.keys.Intel):
			if !m.layout.State().IsIdle() {
				m.layout.ToggleIntel()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout.SetSize(m.width, m.height)

	case snapshotMsg:
		snap := collector.Snapshot(msg)
		m.layout.State().Update(snap)
		m.layout.Update(m.layout.State())
		if !m.tmuxHintShown && os.Getenv("TMUX") != "" {
			m.tmuxHintShown = true
			m.layout.State().PushAlert(appmodel.AlertEntry{
				Time:    snap.Timestamp,
				Message: "mouse mode active; if clicks don't register run: tmux set -g mouse on",
				Level:   appmodel.ThreatCool,
			})
		}
		return m, waitForSnapshot(m.snapChan)

	case gateEventMsg:
		ev := collector.GateEvent(msg)
		m.layout.State().HandleEvent(ev)
		if ev.Event == collector.EventCounterReset {
			m.layout.DismissIntel() // session stats become incoherent
		}
		return m, waitForEvent(m.eventChan)

	case updateAvailableMsg:
		m.layout.State().UpdateAvailable = true

	case tea.MouseMotionMsg:
		m.layout.SetHoveredAgent(agentUnderCursor(m.layout.RenderedAgentIDs(), msg))

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Agent zone click — debounced to prevent double-click flicker.
			if hit := agentUnderCursor(m.layout.RenderedAgentIDs(), msg); hit != "" && time.Since(m.lastFocusTime) >= config.ClickDebounce {
				m.layout.FocusAgent(hit)
				m.lastFocusTime = time.Now()
				return m, nil
			}
			// Category zone click.
			for _, cat := range collector.Categories {
				if zi := zone.Get(ui.CatZoneID(cat.Name)); zi != nil && zi.InBounds(msg) {
					m.layout.State().ToggleCategoryFilter(cat.Name)
					break
				}
			}
		}

	case animTickMsg:
		m.layout.AnimTick()
		return m, animTick()
	}

	return m, nil
}

func (m model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	if m.width == 0 || m.height == 0 {
		return v
	}
	content := m.layout.View()
	if m.mouseEnabled {
		v.MouseMode = tea.MouseModeAllMotion
		v.Content = zone.Scan(content)
	} else {
		v.MouseMode = tea.MouseModeNone
		v.Content = content
	}
	return v
}

// ── Main ────────────────────────────────────────────────────

func main() {
	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	themeName := flag.String("theme", "", "Color theme (classic, iron, mono, frappe)")
	animName := flag.String("animation", "", "Animation profile (default, calm, intense)")
	listThemes := flag.Bool("list-themes", false, "List available themes and exit")
	listAnims := flag.Bool("list-animations", false, "List available animation profiles and exit")
	kittHighScore := flag.Bool("kitt-highscore", true, "KITT scans completed agents instead of ghosts")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

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

	// Resolve highscore mode: flag > env > on
	highScore := *kittHighScore
	if highScore && os.Getenv("COOLANT_KITT_HIGHSCORE") == "0" {
		highScore = false
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

	zone.NewGlobal()

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
