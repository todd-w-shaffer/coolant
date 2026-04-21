package widgets

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func testBattery(t *testing.T) *Battery {
	t.Helper()
	th := theme.Classic()
	th.Init()
	return NewBattery(th, anim.Default())
}

func TestBattery_HiddenWhenAbsent(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{BatteryPresent: false})
	top, bot, w := b.ViewLines(theme.Classic().OverallGradient[1].Bg)
	if top != "" || bot != "" || w != 0 {
		t.Errorf("absent battery: got top=%q bot=%q w=%d, want empty/0", top, bot, w)
	}
}

func TestBattery_DischargingRender(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent:       true,
		BatteryPercent:       47,
		BatteryState:         collector.BatteryDischarging,
		BatteryTimeRemaining: 2*time.Hour + 57*time.Minute,
	})
	th := theme.Classic()
	th.Init()
	top, bot, w := b.ViewLines(th.OverallGradient[1].Bg)
	if w != config.BatteryCellWidth {
		t.Errorf("width = %d, want %d", w, config.BatteryCellWidth)
	}
	topStrip := ansi.Strip(top)
	botStrip := ansi.Strip(bot)
	if len(topStrip) == 0 {
		t.Fatal("top row empty")
	}
	// Top row should contain percent text
	if !strings.Contains(topStrip, "47%") {
		t.Errorf("top row missing 47%%: %q", topStrip)
	}
	// Bot row should contain time remaining
	if !strings.Contains(botStrip, "2h57m") {
		t.Errorf("bot row missing 2h57m: %q", botStrip)
	}
}

func TestBattery_BrailleLevelMapping(t *testing.T) {
	tests := []struct {
		pct        float64
		wantTop    int // top half level 0-4
		wantBottom int // bottom half level 0-4
	}{
		{0, 0, 0},
		{12, 0, 1},
		{25, 0, 2},
		{50, 0, 4},
		{63, 1, 4},
		{87, 3, 4},
		{100, 4, 4},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			topR, botR := brailleGauge(tt.pct)
			gotTop := decodeBrailleLevel(topR)
			gotBot := decodeBrailleLevel(botR)
			if gotTop != tt.wantTop || gotBot != tt.wantBottom {
				t.Errorf("pct=%.0f: got top=%d bot=%d, want top=%d bot=%d",
					tt.pct, gotTop, gotBot, tt.wantTop, tt.wantBottom)
			}
		})
	}
}

// decodeBrailleLevel returns how many rows are filled in a braille char
// that uses both left+right columns (solid bar). Counts bits set in
// the standard braille positions.
func decodeBrailleLevel(r rune) int {
	bits := r & 0xFF
	level := 0
	// Bottom row: dots 7+8 (bits 0x40+0x80)
	if bits&0x40 != 0 && bits&0x80 != 0 {
		level++
	}
	// Row 3: dots 3+6 (bits 0x04+0x20)
	if bits&0x04 != 0 && bits&0x20 != 0 {
		level++
	}
	// Row 2: dots 2+5 (bits 0x02+0x10)
	if bits&0x02 != 0 && bits&0x10 != 0 {
		level++
	}
	// Top row: dots 1+4 (bits 0x01+0x08)
	if bits&0x01 != 0 && bits&0x08 != 0 {
		level++
	}
	return level
}

func TestBattery_SeverityBucketGreen(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 80,
		BatteryState:   collector.BatteryDischarging,
	})
	fg := b.severityFg()
	th := b.theme
	want := th.OverallGradient[1].Fg
	if fg != want {
		t.Errorf("80%% should be green: got %v, want %v", fg, want)
	}
}

func TestBattery_SeverityBucketAmber(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 35,
		BatteryState:   collector.BatteryDischarging,
	})
	fg := b.severityFg()
	th := b.theme
	want := th.OverallGradient[2].Fg
	if fg != want {
		t.Errorf("35%% should be amber: got %v, want %v", fg, want)
	}
}

func TestBattery_SeverityBucketRed(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 10,
		BatteryState:   collector.BatteryDischarging,
	})
	fg := b.severityFg()
	th := b.theme
	want := th.OverallGradient[3].Fg
	if fg != want {
		t.Errorf("10%% should be red: got %v, want %v", fg, want)
	}
}

func TestBattery_SeverityBucketBoundaryExact(t *testing.T) {
	b := testBattery(t)
	th := b.theme

	// 20% is exactly at BatteryCritPct → red (< uses strict less-than)
	b.Update(collector.SystemStats{BatteryPresent: true, BatteryPercent: 20, BatteryState: collector.BatteryDischarging})
	if got := b.severityFg(); got != th.OverallGradient[2].Fg {
		t.Errorf("20%% should be amber (>=critPct): got %v", got)
	}

	// 50% is exactly at BatteryWarnPct → green (>= warnPct means green)
	b.Update(collector.SystemStats{BatteryPresent: true, BatteryPercent: 50, BatteryState: collector.BatteryDischarging})
	if got := b.severityFg(); got != th.OverallGradient[1].Fg {
		t.Errorf("50%% should be green (>=warnPct): got %v", got)
	}
}

func TestBattery_ChargingShowsBolt(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent:       true,
		BatteryPercent:       85,
		BatteryState:         collector.BatteryCharging,
		BatteryTimeRemaining: 42 * time.Minute,
	})
	th := theme.Classic()
	th.Init()
	top, bot, _ := b.ViewLines(th.OverallGradient[1].Bg)
	topStrip := ansi.Strip(top)
	botStrip := ansi.Strip(bot)
	// Bolt is on the bottom row in the time slot; top row has gauge + percent.
	if strings.Contains(topStrip, "⚡") {
		t.Errorf("charging: bolt should not be on top row: %q", topStrip)
	}
	if !strings.Contains(botStrip, "⚡") {
		t.Errorf("charging bot row missing ⚡: %q", botStrip)
	}
	if !strings.Contains(topStrip, "85%") {
		t.Errorf("charging top row missing 85%%: %q", topStrip)
	}
}

func TestBattery_ChargedShowsSolidBolt(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 100,
		BatteryState:   collector.BatteryCharged,
	})
	th := theme.Classic()
	th.Init()
	_, bot, _ := b.ViewLines(th.OverallGradient[1].Bg)
	botStrip := ansi.Strip(bot)
	if !strings.Contains(botStrip, "⚡") {
		t.Errorf("charged bot row missing ⚡: %q", botStrip)
	}

	// Phase should not advance on AnimTick for charged state.
	b.AnimTick()
	if b.phase != 0 {
		t.Errorf("charged: phase advanced to %v, want 0", b.phase)
	}
}

func TestBattery_ChargingAnimTickAdvancesPhase(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 85,
		BatteryState:   collector.BatteryCharging,
	})
	b.AnimTick()
	if b.phase == 0 {
		t.Error("charging: AnimTick did not advance phase")
	}
	wantStep := b.anim.BreathePhaseStep * config.BatteryBreathRate
	if math.Abs(b.phase-wantStep) > 1e-9 {
		t.Errorf("phase = %v, want %v (one BreathePhaseStep * BatteryBreathRate)", b.phase, wantStep)
	}
}

func TestBattery_ChargingBrightnessOscillates(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 85,
		BatteryState:   collector.BatteryCharging,
	})

	// At BatteryBreathRate=0.35, full cycle is ~130 ticks. Sample enough
	// to see oscillation.
	var values []float64
	for i := 0; i < 100; i++ {
		brightness := config.BatteryChargeBreathFloor +
			(config.BatteryChargeBreathCeil-config.BatteryChargeBreathFloor)*sinNorm(b.phase)
		values = append(values, brightness)
		b.AnimTick()
	}

	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if minV < 0.34 || maxV > 0.81 {
		t.Errorf("brightness range [%v, %v] outside expected [0.35, 0.8]", minV, maxV)
	}
	// Must not be monotonic — verify it oscillates.
	monotonic := true
	for i := 1; i < len(values); i++ {
		if values[i] < values[i-1] {
			monotonic = false
			break
		}
	}
	if monotonic {
		t.Error("brightness values are monotonically increasing — not oscillating")
	}
}

func TestBattery_NoEstimateShowsBlank(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent:       true,
		BatteryPercent:       47,
		BatteryState:         collector.BatteryDischarging,
		BatteryTimeRemaining: 0,
	})
	th := theme.Classic()
	th.Init()
	_, bot, w := b.ViewLines(th.OverallGradient[1].Bg)
	botStrip := ansi.Strip(bot)
	if w != config.BatteryCellWidth {
		t.Errorf("no-estimate: width=%d want %d", w, config.BatteryCellWidth)
	}
	// Bot row should have braille gauge only — no dash, no bolt, no time.
	if strings.Contains(botStrip, "\u2014") {
		t.Errorf("no-estimate: bot row should not contain em-dash: %q", botStrip)
	}
	if strings.Contains(botStrip, "⚡") {
		t.Errorf("no-estimate: bot row should not contain bolt: %q", botStrip)
	}
}

func TestBattery_FixedCellWidth(t *testing.T) {
	b := testBattery(t)
	th := theme.Classic()
	th.Init()
	bg := th.OverallGradient[1].Bg

	// Present: width == BatteryCellWidth
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 65,
		BatteryState:   collector.BatteryDischarging,
	})
	_, _, w := b.ViewLines(bg)
	if w != config.BatteryCellWidth {
		t.Errorf("present: width=%d want %d", w, config.BatteryCellWidth)
	}

	// Absent: width == 0
	b.Update(collector.SystemStats{BatteryPresent: false})
	_, _, w = b.ViewLines(bg)
	if w != 0 {
		t.Errorf("absent: width=%d want 0", w)
	}
}

func TestBattery_MeltdownPulseAtCrit(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 5,
		BatteryState:   collector.BatteryDischarging,
	})

	var phases []float64
	for i := 0; i < 30; i++ {
		phases = append(phases, b.meltdownPhase)
		b.AnimTick()
	}

	// At 5% discharging, meltdown phase must advance.
	if phases[29] == 0 {
		t.Error("5%% discharging: meltdown phase did not advance")
	}

	// Verify the phase produces a brightness range that's detectable.
	minB := 1.0
	maxB := 0.0
	for _, p := range phases {
		brightness := 0.6 + 0.4*(math.Sin(p)+1)/2
		if brightness < minB {
			minB = brightness
		}
		if brightness > maxB {
			maxB = brightness
		}
	}
	if minB > 0.7 || maxB < 0.95 {
		t.Errorf("meltdown brightness [%v, %v]: want min<=0.7, max>=0.95", minB, maxB)
	}

	// At 12% discharging, no meltdown pulse.
	b2 := testBattery(t)
	b2.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 12,
		BatteryState:   collector.BatteryDischarging,
	})
	for i := 0; i < 30; i++ {
		b2.AnimTick()
	}
	if b2.meltdownPhase != 0 {
		t.Errorf("12%% discharging: meltdown phase = %v, want 0", b2.meltdownPhase)
	}
}

func TestBattery_MeltdownPulseIndependentOfHeadline(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 5,
		BatteryState:   collector.BatteryDischarging,
	})

	b.AnimTick()
	p1 := b.meltdownPhase
	b.AnimTick()
	p2 := b.meltdownPhase

	if p2 <= p1 {
		t.Errorf("meltdown phase not monotonically advancing: p1=%v p2=%v", p1, p2)
	}
}

func TestBattery_ChargingAboveCritDoesNotPulse(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 8,
		BatteryState:   collector.BatteryCharging,
	})
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	if b.meltdownPhase != 0 {
		t.Errorf("8%% charging: meltdown phase = %v, want 0 (charging suppresses crit pulse)", b.meltdownPhase)
	}
}
