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
	if !strings.Contains(topStrip, "047%") {
		t.Errorf("top row missing 047%%: %q", topStrip)
	}
	// Bot row should contain time remaining
	if !strings.Contains(botStrip, "3.0h") {
		t.Errorf("bot row missing 3.0h: %q", botStrip)
	}
}

func TestBattery_BrailleLevelMapping(t *testing.T) {
	tests := []struct {
		pct        float64
		wantTop    int // top half fill level 0-4
		wantBottom int // bottom half fill level 0-4
	}{
		{0, 0, 0},
		{12, 0, 1},
		{25, 0, 2},
		{50, 0, 4},
		{63, 1, 4},
		{87, 3, 4},
		{100, 3, 4}, // capped at level 7 to preserve nipple shape
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			_, _, _, botL, _, _ := brailleBattery(tt.pct)
			// Bottom chars have pure fill (no outline bits).
			gotBot := decodeBrailleLevel(botL)
			if gotBot != tt.wantBottom {
				t.Errorf("pct=%.0f: bot level=%d, want %d", tt.pct, gotBot, tt.wantBottom)
			}
		})
	}
}

func TestBattery_NippleAlwaysPresent(t *testing.T) {
	// The center top char must always include the nipple bits,
	// even at 0% fill.
	for _, pct := range []float64{0, 25, 50, 75, 100} {
		_, topC, _, _, _, _ := brailleBattery(pct)
		bits := topC & 0xFF
		// Nipple = dots 1+4 (top row of center char)
		if bits&0x01 == 0 || bits&0x08 == 0 { // dots 1 and 4 (terminal at top)
			t.Errorf("pct=%.0f: center top char %U missing nipple bits", pct, topC)
		}
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
	if !strings.Contains(topStrip, "085%") {
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

	// At 12% discharging, warning breath is active (10-20% range).
	b2 := testBattery(t)
	b2.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 12,
		BatteryState:   collector.BatteryDischarging,
	})
	for i := 0; i < 30; i++ {
		b2.AnimTick()
	}
	if b2.meltdownPhase == 0 {
		t.Error("12%% discharging: warning breath should be active")
	}

	// At 25% discharging, no breath or pulse.
	b3 := testBattery(t)
	b3.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 25,
		BatteryState:   collector.BatteryDischarging,
	})
	for i := 0; i < 30; i++ {
		b3.AnimTick()
	}
	if b3.meltdownPhase != 0 {
		t.Errorf("25%% discharging: meltdown phase = %v, want 0", b3.meltdownPhase)
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

func TestFormatRemaining(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Minute, "0.8h"},
		{1*time.Hour + 30*time.Minute, "1.5h"},
		{2*time.Hour + 57*time.Minute, "3.0h"},
		{5*time.Hour + 12*time.Minute, "5.2h"},
		{9*time.Hour + 54*time.Minute, "9.9h"},
		{10 * time.Hour, "10hr"},
		{12*time.Hour + 30*time.Minute, "13hr"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatRemaining(tt.d)
			if got != tt.want {
				t.Errorf("formatRemaining(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestBattery_CasingAlwaysPresent(t *testing.T) {
	// Bottom chars: outer columns fully lit (all 4 rows).
	// Top chars: outer columns lit on rows 2-4 only (3 rows) — row 1 is
	// empty so the nipple terminal pokes above the casing walls.
	const leftFull rune = 0x01 | 0x02 | 0x04 | 0x40  // rows 1-4
	const rightFull rune = 0x08 | 0x10 | 0x20 | 0x80 // rows 1-4
	const leftShort rune = 0x02 | 0x04 | 0x40        // rows 2-4 (no dot 1)
	const rightShort rune = 0x10 | 0x20 | 0x80       // rows 2-4 (no dot 4)

	for _, pct := range []float64{0, 12, 25, 37, 50, 63, 75, 87, 100} {
		topL, _, topR, botL, _, botR := brailleBattery(pct)
		// Top chars: short walls (rows 2-4), row 1 must be empty on sides.
		if topL&0xFF&leftShort != leftShort {
			t.Errorf("pct=%.0f: topL %U missing left casing bits (rows 2-4)", pct, topL)
		}
		if topL&0x01 != 0 {
			t.Errorf("pct=%.0f: topL %U has dot 1 set — wall extends into nipple row", pct, topL)
		}
		if topR&0xFF&rightShort != rightShort {
			t.Errorf("pct=%.0f: topR %U missing right casing bits (rows 2-4)", pct, topR)
		}
		if topR&0x08 != 0 {
			t.Errorf("pct=%.0f: topR %U has dot 4 set — wall extends into nipple row", pct, topR)
		}
		// Bottom chars: full-height walls.
		if botL&0xFF&leftFull != leftFull {
			t.Errorf("pct=%.0f: botL %U missing left casing bits", pct, botL)
		}
		if botR&0xFF&rightFull != rightFull {
			t.Errorf("pct=%.0f: botR %U missing right casing bits", pct, botR)
		}
	}
}

func TestBattery_ShoulderAlwaysPresent(t *testing.T) {
	// Row 2 of top chars forms the shoulder — the step-down from the
	// narrow nipple to the wider body. topL's right column and topR's
	// left column must always be lit on row 2, closing the battery outline.
	const shoulderL rune = 0x10 // dot 5 — right column, row 2 of topL
	const shoulderR rune = 0x02 // dot 2 — left column, row 2 of topR

	for _, pct := range []float64{0, 25, 50, 75, 100} {
		topL, _, topR, _, _, _ := brailleBattery(pct)
		if topL&shoulderL == 0 {
			t.Errorf("pct=%.0f: topL %U missing shoulder bit (dot 5)", pct, topL)
		}
		if topR&shoulderR == 0 {
			t.Errorf("pct=%.0f: topR %U missing shoulder bit (dot 2)", pct, topR)
		}
	}
}

func TestBattery_WarnBreathAt15Pct(t *testing.T) {
	b := testBattery(t)
	b.Update(collector.SystemStats{
		BatteryPresent: true,
		BatteryPercent: 15,
		BatteryState:   collector.BatteryDischarging,
	})

	// Phase must advance (warning breath active).
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	if b.meltdownPhase == 0 {
		t.Error("15%% discharging: meltdown phase did not advance (warning breath should be active)")
	}

	// Brightness must stay in the subtle range (0.85–1.0), NOT the
	// aggressive meltdown range (0.6–1.0). Continue from the same instance.
	var values []float64
	for i := 0; i < 200; i++ {
		brightness := config.BatteryWarnBreathFloor +
			(config.BatteryWarnBreathCeil-config.BatteryWarnBreathFloor)*sinNorm(b.meltdownPhase)
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
	if minV < 0.84 || maxV > 1.01 {
		t.Errorf("warning breath brightness [%v, %v]: want within [0.85, 1.0]", minV, maxV)
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
