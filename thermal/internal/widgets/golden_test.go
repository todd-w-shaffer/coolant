package widgets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// ── Fixture data ─────────────────────────────────────────────

// fixtureSnapshot returns a realistic snapshot with known values.
func fixtureSnapshot() collector.Snapshot {
	return collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:     42.5,
			MemUsedBytes:   8 * 1024 * 1024 * 1024,  // 8GB
			MemTotalBytes:  16 * 1024 * 1024 * 1024, // 16GB
			SwapUsedBytes:  512 * 1024 * 1024,       // 512MB
			Decompressions: 1234,
			GPUPercent:     35.0,
			NCPUs:          10,
		},
		Sessions: []collector.SessionTree{
			{
				RootPID:  1000,
				RootComm: "claude",
				Descendants: []collector.ProcessInfo{
					{PID: 1001, TypeCode: "N", Comm: "node"},
					{PID: 1002, TypeCode: "N", Comm: "node"},
					{PID: 1003, TypeCode: "S", Comm: "bash"},
				},
			},
		},
		AllProcs: []collector.ProcessInfo{
			{PID: 1001, TypeCode: "N", Comm: "node"},
			{PID: 1002, TypeCode: "N", Comm: "node"},
			{PID: 1003, TypeCode: "S", Comm: "bash"},
		},
		Online:    true,
		Timestamp: time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC),
	}
}

// fixtureState returns an AppState fed with the fixture snapshot.
func fixtureState() *model.AppState {
	s := model.NewAppState()
	snap := fixtureSnapshot()
	s.Update(snap)
	return s
}

// ── Helpers ──────────────────────────────────────────────────

func goldenPath(name string) string {
	return filepath.Join("testdata", name+".golden")
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	path := goldenPath(name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing golden file %s: %v", path, err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(content))
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := goldenPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file %s: %v (run UPDATE_GOLDEN=1 to generate)", path, err)
	}
	return string(data)
}

func assertMatchesGolden(t *testing.T, name, got string) {
	t.Helper()
	want := readGolden(t, name)
	if got != want {
		t.Errorf("%s mismatch:\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

// ── Capture golden files ─────────────────────────────────────

func TestCaptureGoldenFiles(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") != "1" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate golden files")
	}

	t.Run("severity_gradient", func(t *testing.T) {
		writeGolden(t, "severity_gradient", captureSeverityGradient())
	})

	t.Run("sparkline", func(t *testing.T) {
		writeGolden(t, "sparkline_classic", captureSparkline())
	})

	t.Run("thermal_levels", func(t *testing.T) {
		writeGolden(t, "thermal_levels", captureThermalLevels())
	})

	t.Run("rates_line", func(t *testing.T) {
		writeGolden(t, "rates_line", captureRatesLine())
	})

	t.Run("alerts_line", func(t *testing.T) {
		writeGolden(t, "alerts_line", captureAlertsLine())
	})

	t.Run("breathedots", func(t *testing.T) {
		writeGolden(t, "breathedots", captureBreatheDots())
	})

	t.Run("gauges", func(t *testing.T) {
		writeGolden(t, "gauges", captureGauges())
	})

	t.Run("gauges_alt_visible", func(t *testing.T) {
		writeGolden(t, "gauges_alt_visible", captureGaugesAltVisible())
	})

	t.Run("headline_rails_idle", func(t *testing.T) {
		writeGolden(t, "headline_rails_idle", captureHeadlineRails(0, 0, 0))
	})

	t.Run("headline_rails_warm", func(t *testing.T) {
		writeGolden(t, "headline_rails_warm", captureHeadlineRails(2, 0, 0))
	})

	t.Run("headline_rails_critical", func(t *testing.T) {
		writeGolden(t, "headline_rails_critical", captureHeadlineRails(9, 4, 0))
	})

	t.Run("headline_rails_ember", func(t *testing.T) {
		// Seed: build warms to peak level (count=5 → critical), then drops
		// to zero halfway through the ember window. Use a fixed decay=0.5.
		writeGolden(t, "headline_rails_ember", captureHeadlineRailsEmber())
	})
}

// ── Match golden files ───────────────────────────────────────

func TestClassicMatchesGolden(t *testing.T) {
	tests := []struct {
		name    string
		capture func() string
	}{
		{"severity_gradient", captureSeverityGradient},
		{"sparkline_classic", captureSparkline},
		{"thermal_levels", captureThermalLevels},
		{"rates_line", captureRatesLine},
		{"alerts_line", captureAlertsLine},
		{"breathedots", captureBreatheDots},
		{"gauges", captureGauges},
		{"gauges_alt_visible", captureGaugesAltVisible},
		{"headline_rails_idle", func() string { return captureHeadlineRails(0, 0, 0) }},
		{"headline_rails_warm", func() string { return captureHeadlineRails(2, 0, 0) }},
		{"headline_rails_critical", func() string { return captureHeadlineRails(9, 4, 0) }},
		{"headline_rails_ember", captureHeadlineRailsEmber},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertMatchesGolden(t, tt.name, tt.capture())
		})
	}
}

// ── Capture functions (widgets) ──────────────────────────────

// captureBreatheDots renders BreatheDots in three modes with deterministic state.
func captureBreatheDots() string {
	th := testTheme
	ap := testAnim
	var sb strings.Builder

	// Mode 1: Active dots (tidal wave)
	b := NewBreatheDots(th, ap)
	b.SetTarget(3)
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	s, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	sb.WriteString(fmt.Sprintf("active:w=%d:%s\n", w, s))

	// Mode 2: Stale dots (KITT scanner)
	b2 := NewBreatheDots(th, ap)
	b2.SetTarget(3)
	b2.SetStaleCount(2)
	for i := 0; i < 60; i++ {
		b2.AnimTick()
	}
	s, w = b2.Render("⬡", "⏣", "⬢", nil, 0)
	sb.WriteString(fmt.Sprintf("stale:w=%d:%s\n", w, s))

	// Mode 3: Highscore (completed KITT + active)
	b3 := NewBreatheDots(th, ap)
	b3.SetHighScoreMode(true)
	b3.SetTarget(2)
	b3.SetCompletedAgents([]string{"a1", "b2", "c3"})
	for i := 0; i < 60; i++ {
		b3.AnimTick()
	}
	s, w = b3.Render("⬡", "⏣", "⬢", nil, 0)
	sb.WriteString(fmt.Sprintf("highscore:w=%d:%s\n", w, s))

	return sb.String()
}

// captureGauges renders Gauges.View() with the fixture snapshot at the
// default visible set (CPU + MEM + Token).
func captureGauges() string {
	th := testTheme
	ap := testAnim
	g := NewGauges(th, ap)
	g.SetSize(60, 6)

	state := fixtureState()
	g.Update(state)

	// Pump enough ticks for spring to settle and renderHistory to fill
	for i := 0; i < 90; i++ {
		g.AnimTick()
	}

	return g.View() + "\n"
}

// captureGaugesAltVisible exercises the visibility-mask path: toggle off
// Token and toggle on Decomp so the rendered set is {CPU, MEM, Decomp} —
// the pre-token "classic" three-up arrangement. Catches regressions where
// toggling drops or duplicates rows.
func captureGaugesAltVisible() string {
	th := testTheme
	ap := testAnim
	g := NewGauges(th, ap)
	g.SetSize(60, 6)

	state := fixtureState()
	g.Update(state)
	for i := 0; i < 90; i++ {
		g.AnimTick()
	}

	g.ToggleVisible(SlotToken)  // 3→2 visible
	g.ToggleVisible(SlotDecomp) // 2→3 visible (Decomp now in)
	return g.View() + "\n"
}

// ── Capture functions (existing) ────────────────────────────

// captureSeverityGradient renders SeverityColor at 0,10,...,100 with CPU thresholds.
func captureSeverityGradient() string {
	th := testTheme
	thresh := &theme.SparkThresholds{Warn: 70, Crit: 90}
	var sb strings.Builder
	for v := 0; v <= 100; v += 10 {
		ansi := th.SeverityColor(float64(v), thresh)
		// Write "VALUE=ANSI_ESCAPE\n" so we capture the exact escape sequence.
		sb.WriteString(fmt.Sprintf("%03d=%s###%s\n", v, ansi, sparkReset))
	}
	return sb.String()
}

// captureSparkline renders RenderSparkline with a known ascending data series.
func captureSparkline() string {
	th := testTheme
	width := 20
	buf := NewSparkBufs(width)
	data := make([]float64, width+1)
	online := make([]bool, width+1)
	for i := range data {
		data[i] = float64(i) * (100.0 / float64(width))
		online[i] = true
	}
	thresh := &theme.SparkThresholds{Warn: 70, Crit: 90}
	pair := RenderSparkline(data, online, width, 100, thresh, 0, buf, th, false)
	return fmt.Sprintf("TOP:%s\nBOT:%s\n", pair.Top, pair.Bottom)
}

// captureHeadlineRails renders renderBuildShellStack at a fixed wall
// clock (no ember decay in-flight — first update arms peak at the given
// level). Returns "TOP:...\nBOT:...\n" so diffs line up readably.
func captureHeadlineRails(buildCount, shellCount int, _ int) string {
	th := testTheme
	ap := testAnim
	h := NewHeadline(th, ap)
	t0 := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return t0 }
	iconBg := th.OverallGradient[1].Bg
	smoothed := map[string]float64{
		"build": float64(buildCount),
		"shell": float64(shellCount),
	}
	rp := h.renderBuildShellStack(smoothed, iconBg)
	return fmt.Sprintf("TOP:%s\nBOT:%s\n", rp.top, rp.bot)
}

// captureHeadlineRailsEmber warms the build rail to its critical peak,
// then drops live count to zero and advances the injected clock to half
// the ember duration so the captured rail color sits at decay=0.5.
func captureHeadlineRailsEmber() string {
	th := testTheme
	ap := testAnim
	h := NewHeadline(th, ap)
	t0 := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	wallclock := t0
	h.now = func() time.Time { return wallclock }
	iconBg := th.OverallGradient[1].Bg

	// Tick 1: warm build to 5 (critical level for build thresholds [1,3]).
	h.renderBuildShellStack(map[string]float64{"build": 5, "shell": 0}, iconBg)

	// Tick 2: count drops to 0 — arms decayStart at wallclock.
	wallclock = t0.Add(1 * time.Millisecond)
	h.renderBuildShellStack(map[string]float64{"build": 0, "shell": 0}, iconBg)

	// Tick 3: advance clock to decayStart + half emberDuration for
	// decay==0.5. Render the final stack at this moment.
	wallclock = t0.Add(1*time.Millisecond + config.BuildShellEmberDecay/2)
	rp := h.renderBuildShellStack(map[string]float64{"build": 0, "shell": 0}, iconBg)
	return fmt.Sprintf("TOP:%s\nBOT:%s\n", rp.top, rp.bot)
}

// zoneMarkerRE matches bubblezone's marker escape (ESC [ <n> z). The numeric
// ID comes from a process-global atomic counter (bubblezone manager.go
// markerCounter), so its value depends on how many Mark calls ran before this
// one — which differs between an isolated test run and the full suite. We
// normalize it to a fixed token so the golden stays stable; the presence of
// the zone wrapper itself is asserted separately in headline_test.go.
var zoneMarkerRE = regexp.MustCompile("\x1b\\[\\d+z")

// captureThermalLevels renders renderCatCell for "build" at counts 0, 1, 3, 5, 10.
func captureThermalLevels() string {
	th := testTheme
	cat := collector.Category{Name: "build", Label: "build", Order: 0}
	counts := []int{0, 1, 3, 5, 10}
	var sb strings.Builder
	for _, count := range counts {
		smoothed := map[string]float64{"build": float64(count)}
		cell := renderCatCell(cat, smoothed, fixedCellWidth, th, "")
		cell = zoneMarkerRE.ReplaceAllString(cell, "\x1b[Zz")
		sb.WriteString(fmt.Sprintf("count=%02d:%s\n", count, cell))
	}
	return sb.String()
}

// captureRatesLine renders Rates.View() with the fixture snapshot.
func captureRatesLine() string {
	th := testTheme
	state := fixtureState()
	r := NewRates(th, keys.Default())
	r.SetSize(200, 1)
	r.Update(state)
	return r.View() + "\n"
}

// captureAlertsLine renders Alerts.View() with known alerts.
func captureAlertsLine() string {
	state := model.NewAppState()
	ts := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	state.HandleEvent(collector.GateEvent{
		Event:     collector.EventAgentStart,
		AgentType: "research",
		AgentID:   "abc12345def",
		Timestamp: ts,
	})
	state.HandleEvent(collector.GateEvent{
		Event:     collector.EventGateSuppress,
		Command:   "tsc --noEmit",
		Reason:    "build suppressed",
		Timestamp: ts.Add(1 * time.Second),
	})
	state.HandleEvent(collector.GateEvent{
		Event:     collector.EventParallelEngaged,
		Timestamp: ts.Add(2 * time.Second),
	})

	th := testTheme
	a := NewAlerts(th)
	a.SetSize(200, 10)
	a.Update(state)
	return a.View() + "\n"
}
