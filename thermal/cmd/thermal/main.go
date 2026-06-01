package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
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
	"github.com/toddwshaffer/coolant/thermal/internal/otel/cc"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
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
	aggregator    *stats.Aggregator
	// checkpointDone closes after the checkpoint goroutine's final
	// flush. main() waits on it post-Run so process exit can't race
	// the fsync — without this, a quit during the 30s tick window
	// loses the unwritten delta.
	checkpointDone chan struct{}

	// ccOtelDone closes after the CC OTEL receiver/tailer/reconcile
	// goroutine's final flush — paired with m.done on tea.Quit.
	// Mirrors checkpointDone so process exit can't race a midflight
	// reconcile tick or receiver shutdown.
	ccOtelDone chan struct{}

	// animating tracks whether an animation tick is currently armed. It
	// gates the idle freeze: the tick handler stops re-arming when the
	// dashboard goes calm, and the snapshot/event handlers re-arm exactly
	// once on the calm→active transition. The flag makes re-arm idempotent
	// across both wake sources so they can't double-arm and double the frame
	// rate.
	animating bool
}

func newModel(demoMode bool, th *theme.Theme, ap *anim.Profile) model {
	km := keys.Default()
	agg := stats.New(productionStatsConfig())
	m := model{
		layout:         layout.NewHorizontal(th, ap, km),
		keys:           km,
		done:           make(chan struct{}),
		demoMode:       demoMode,
		mouseEnabled:   true,
		snapChan:       make(chan collector.Snapshot, 16),
		eventChan:      make(chan collector.GateEvent, 32),
		updateChan:     make(chan string, 1),
		aggregator:     agg,
		checkpointDone: make(chan struct{}),
		ccOtelDone:     make(chan struct{}),
		// Init's command batch arms the first animation tick, so the model
		// starts in the animating state.
		animating: true,
	}
	m.layout.State().AttachAggregator(agg)
	// Self-check the wire-up so a future refactor that drops the
	// AttachAggregator call fails at startup instead of silently
	// shipping a TUI that records zero lifetime stats forever.
	if m.layout.State().Aggregator() == nil {
		panic("thermo: stats aggregator wire-up missing — AppState.Aggregator() returned nil after AttachAggregator")
	}
	return m
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
	sessionPath := os.Getenv("COOLANT_SESSION_FILE")
	if sessionPath == "" {
		sessionPath = coolantTmpPath("session")
	}
	go collector.TailEvents(m.eventChan, evPath, sessionPath, config.EventInterval, m.done)

	if m.aggregator != nil {
		go func() {
			defer close(m.checkpointDone)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-m.done:
					_ = m.aggregator.Checkpoint()
					return
				case <-ticker.C:
					_ = m.aggregator.Checkpoint()
				}
			}
		}()
	} else {
		close(m.checkpointDone)
	}

	// CC OTEL pipeline (§0.1b startup ordering: findings writer FIRST,
	// adapter, tailer, reconcile, receiver LAST so the bind-failure
	// path goes through the same writer everyone else uses).
	startCcOtel(m.aggregator, m.done, m.ccOtelDone)

	cmds := []tea.Cmd{waitForSnapshot(m.snapChan), waitForEvent(m.eventChan), animTick()}

	if !config.C.Updates.Disabled {
		go func() {
			defer close(m.updateChan)
			cachePath := coolantTmpPath(updater.CacheFilename)
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

// wakeCmd arms one animation tick if the dashboard just transitioned from
// frozen (calm) to active, and returns nil otherwise. The m.animating guard
// makes it idempotent: the snapshot and event handlers both call it every
// message, but only the calm→active edge arms a tick — so the two wake
// sources can't spawn two concurrent timers and double the frame rate. It
// mutates the receiver, so callers must use the returned model.
//
// Liveness note: the wake depends on the collector pushing snapshots every
// ~FastInterval regardless of calm (see Init's collector.Run / demo.RunV2) so
// this runs continuously to catch the wake edge. If a future change ever
// gates snapshot delivery on activity, the freeze would have no wake source
// and could latch permanently — re-arm from an unconditional source then.
func (m *model) wakeCmd() tea.Cmd {
	if !m.animating && !m.layout.State().IsCalm() {
		m.animating = true
		return animTick()
	}
	return nil
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

// redirectLog routes the standard logger to path (created if absent,
// appended otherwise, owner-only 0o600) via bubbletea's own LogToFile
// helper and returns the open file so the caller can close it on exit.
// The TUI owns the same stdout/stderr the default logger writes to, so
// any log.Printf from the collector, stats aggregator, or checkpoint
// path would paint raw text over the render — e.g. the by_project drift
// guard bleeding "stats: by_project drift detected" into the dashboard.
//
// On open failure the logger is pointed at io.Discard rather than left
// on stderr: stderr IS the surface the TUI occupies, so a half-working
// redirect that fell back to stderr would reintroduce the very
// corruption this exists to prevent. Only the TUI path calls this;
// subcommands (stats / statsdump / cc-findings) keep stderr.
func redirectLog(path string) (*os.File, error) {
	f, err := tea.LogToFile(path, "")
	if err != nil {
		log.SetOutput(io.Discard)
		return nil, err
	}
	return f, nil
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
		case key.Matches(msg, m.keys.ClearCompleted):
			m.layout.State().ClearCompletedRecords()
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
		case key.Matches(msg, m.keys.ToggleCPU):
			m.layout.ToggleSparklineCPU()
		case key.Matches(msg, m.keys.ToggleMEM):
			m.layout.ToggleSparklineMEM()
		case key.Matches(msg, m.keys.ToggleDecomp):
			m.layout.ToggleSparklineDecomp()
		case key.Matches(msg, m.keys.ToggleToken):
			m.layout.ToggleSparklineToken()
		case key.Matches(msg, m.keys.TogglePretty):
			m.layout.ToggleSparklinePretty()
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
		return m, tea.Batch(waitForSnapshot(m.snapChan), m.wakeCmd())

	case gateEventMsg:
		ev := collector.GateEvent(msg)
		m.layout.State().HandleEvent(ev)
		if ev.Event == collector.EventCounterReset {
			m.layout.DismissIntel() // session stats become incoherent
		}
		return m, tea.Batch(waitForEvent(m.eventChan), m.wakeCmd())

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
			// Path zone click — open transcript in OS default handler.
			if id := m.layout.FocusedAgentID(); id != "" {
				if zi := zone.Get(ui.PathZoneID(id)); zi != nil && zi.InBounds(msg) {
					if path := m.layout.FocusedTranscriptPath(); path != "" {
						exec.Command("open", path).Start() //nolint:errcheck
						return m, nil
					}
				}
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
		// Stop re-arming once the dashboard goes calm — this is the freeze.
		// The snapshot/event handlers re-arm via wakeCmd when activity returns.
		if m.layout.State().IsCalm() {
			m.animating = false
			return m, nil
		}
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

// startCcOtel wires up the CC OTEL pipeline per spec §0.1b. The
// goroutine startup order is FIXED:
//  1. findings writer (NewWriter creates the path lazily on first
//     write — mkdir 0o700 ~/.coolant inside Writer.appendLine)
//  2. adapter struct
//  3. tailer goroutine
//  4. reconcile goroutine
//  5. receiver goroutine LAST — its bind-failure path emits one
//     receiver_bind_failed finding through the writer initialized in
//     step 1.
//
// `ccOtelDone` is closed after every CC OTEL goroutine has finished
// its terminal flush. main() waits on it post-Run so process exit
// can't race the receiver shutdown or a final reconcile tick.
func startCcOtel(aggregator cc.AggregatorView, done <-chan struct{}, ccOtelDone chan<- struct{}) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// No HOME → no durable findings; degrade silently.
		close(ccOtelDone)
		return
	}

	findingsPath := filepath.Join(home, ".coolant", "cc-otel-findings.jsonl")
	writer := cc.NewWriter(findingsPath, os.Stderr)

	jsonlPath := os.Getenv("COOLANT_CC_OTEL_JSONL")
	if jsonlPath == "" {
		jsonlPath = coolantTmpPath("cc-otel.jsonl")
	}

	port := 4318
	if v := os.Getenv("COOLANT_CC_OTEL_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			port = n
		}
	}

	tailer := cc.NewMetricsTailer(jsonlPath)
	tailer.Findings = writer

	adapter := cc.NewAdapter(cc.AdapterConfig{
		Findings:       writer,
		CoolantVersion: version.Version,
	})

	receiver, err := cc.NewReceiver(cc.ReceiverConfig{
		Addr:           "127.0.0.1",
		Port:           port,
		JSONLPath:      jsonlPath,
		Findings:       writer,
		CoolantVersion: version.Version,
	})
	if err != nil {
		close(ccOtelDone)
		return
	}

	reconciler := cc.NewReconciler(cc.ReconcilerConfig{
		Tailer:             tailer,
		Aggregator:         aggregator,
		Adapter:            adapter,
		Findings:           writer,
		LastReceiverPostTS: receiver.LastSuccessfulPostTS,
	})

	tailer.Start()

	reconcileTicker := time.NewTicker(60 * time.Second)
	rolloverTimer := time.NewTimer(durationToNextUTCMidnight() + 30*time.Second)

	go func() {
		defer close(ccOtelDone)
		defer reconcileTicker.Stop()
		defer rolloverTimer.Stop()

		// Receiver started LAST per §0.1b so the bind-failure path
		// flows through the writer initialized at step 1. Bind failure
		// here only logs a single finding and the loop continues —
		// reconcile no-ops cleanly when the JSONL stays empty.
		_ = receiver.Start()

		for {
			select {
			case <-done:
				_ = receiver.Shutdown(context.Background())
				tailer.Stop()
				_ = reconciler.ReconcileToday()
				return
			case <-reconcileTicker.C:
				_ = reconciler.ReconcileToday()
			case <-rolloverTimer.C:
				_ = reconciler.ReconcileWindow(1)
				tailer.PruneOlderThan(7)
				rolloverTimer.Reset(durationToNextUTCMidnight() + 30*time.Second)
			}
		}
	}()
}

// durationToNextUTCMidnight returns the time.Duration until the next
// UTC midnight tick. Reused on every rollover firing.
func durationToNextUTCMidnight() time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return next.Sub(now)
}

// productionStatsConfig falls through to in-memory-only when HOME
// can't be resolved — empty CachePath disables persistence but keeps
// the aggregator usable.
func productionStatsConfig() stats.Config {
	jsonl := os.Getenv("COOLANT_EVENTS")
	if jsonl == "" {
		jsonl = coolantTmpPath("events.jsonl")
	}
	cfg := stats.Config{
		JSONLPath:    jsonl,
		DegradedPath: coolantTmpPath("degraded.count"),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cfg.CachePath = filepath.Join(home, ".coolant", "stats.json")
	}
	return cfg
}

// ── Main ────────────────────────────────────────────────────

func main() {
	// Dispatch BEFORE flag.Parse — otherwise flag.Parse errors on
	// bare-word verbs.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "statsdump":
			folded, err := runStatsdump(os.Stdout, productionStatsConfig())
			if err != nil {
				fmt.Fprintf(os.Stderr, "statsdump: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "statsdump: folded %d schema:1 events\n", folded)
			os.Exit(0)
		case "stats":
			os.Exit(runStats(os.Stdout, os.Stderr, os.Args[2:], productionStatsConfig()))
		case "cc-findings":
			os.Exit(runCcFindings(os.Stdout, os.Stderr, os.Args[2:], productionStatsConfig()))
		case upgradeVerb, upgradeFlag:
			os.Exit(runUpgrade(os.Stdout, os.Stderr))
		}
	}

	demoMode := flag.Bool("demo", false, "Generate synthetic data")
	themeName := flag.String("theme", "", "Color theme (classic, iron, mono, frappe, latte)")
	animName := flag.String("animation", "", "Animation profile (default, calm, intense)")
	listThemes := flag.Bool("list-themes", false, "List available themes and exit")
	listAnims := flag.Bool("list-animations", false, "List available animation profiles and exit")
	kittHighScore := flag.Bool("kitt-highscore", true, "KITT scans completed agents instead of ghosts")
	showVersion := flag.Bool("version", false, "Print version and exit")
	ccOtelPort := flag.Int("cc-otel-port", 0, "CC OTEL receiver port override (default 4318; env COOLANT_CC_OTEL_PORT)")
	flag.Parse()

	if *ccOtelPort > 0 {
		os.Setenv("COOLANT_CC_OTEL_PORT", strconv.Itoa(*ccOtelPort))
	}

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

	// Keep diagnostics off the TUI's stdout/stderr — see redirectLog. A
	// failure here is non-fatal: redirectLog has already pointed the
	// logger at io.Discard, so the dashboard renders cleanly regardless.
	if logFile, err := redirectLog(coolantTmpPath("thermo.log")); err == nil {
		defer logFile.Close()
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
	// Block on the checkpoint goroutine's final flush so process exit
	// can't race the fsync. parentExitMsg / Quit closes m.done, the
	// goroutine then runs its terminal Checkpoint and closes
	// checkpointDone. Bounded by the disk I/O — typically <50ms.
	<-m.checkpointDone
	if m.ccOtelDone != nil {
		<-m.ccOtelDone
	}
}
