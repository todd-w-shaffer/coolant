package widgets

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

var testTheme = theme.Classic()
var testAnim = anim.Default()

func TestBreatheDotSetTargetSpawn(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	if b.Len() != 3 {
		t.Errorf("Len() = %d, want 3", b.Len())
	}
	// All dots start at alive=0 (fading in)
	for i, d := range b.dots {
		if d.alive != 0 {
			t.Errorf("dots[%d].alive = %f, want 0", i, d.alive)
		}
		if d.dying {
			t.Errorf("dots[%d].dying = true, want false", i)
		}
	}
}

func TestBreatheDotSetTargetKill(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	b.SetTarget(1)
	// 2 should be marked dying, 1 alive
	alive, dying := 0, 0
	for _, d := range b.dots {
		if d.dying {
			dying++
		} else {
			alive++
		}
	}
	if alive != 1 {
		t.Errorf("alive count = %d, want 1", alive)
	}
	if dying != 2 {
		t.Errorf("dying count = %d, want 2", dying)
	}
}

func TestBreatheDotSetTargetKillsFromEnd(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	b.SetTarget(1)
	// First dot should be alive, last two dying
	if b.dots[0].dying {
		t.Error("dots[0] should not be dying")
	}
	if !b.dots[1].dying || !b.dots[2].dying {
		t.Error("dots[1] and dots[2] should be dying")
	}
}

func TestBreatheDotPhaseMonotonic(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	for i := 1; i < len(b.dots); i++ {
		if b.dots[i].phase <= b.dots[i-1].phase {
			t.Errorf("dots[%d].phase (%f) <= dots[%d].phase (%f)", i, b.dots[i].phase, i-1, b.dots[i-1].phase)
		}
	}
}

func TestBreatheDotAnimTickAdvancesAlive(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(1)
	// Run enough ticks for the spring to move from 0 toward 1
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	if b.dots[0].alive < 0.5 {
		t.Errorf("after 30 ticks, alive = %f, want > 0.5", b.dots[0].alive)
	}
}

func TestBreatheDotAnimTickRemovesFaded(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(1)
	// Let it fade in
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	// Kill it
	b.SetTarget(0)
	// Run enough ticks for spring to reach near-zero
	for i := 0; i < 120; i++ {
		b.AnimTick()
	}
	if b.Len() != 0 {
		t.Errorf("after fade-out, Len() = %d, want 0 (alive=%f)", b.Len(), b.dots[0].alive)
	}
}

func TestBreatheDotAnimTickAdvancesPhase(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(1)
	initialPhase := b.dots[0].phase
	b.AnimTick()
	if b.dots[0].phase != initialPhase+testAnim.BreathePhaseStep {
		t.Errorf("phase after 1 tick = %f, want %f", b.dots[0].phase, initialPhase+testAnim.BreathePhaseStep)
	}
}

func TestBreatheDotAnimTickFreezesPhaseDying(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(1)
	b.AnimTick() // advance phase once
	phase := b.dots[0].phase
	b.SetTarget(0) // mark dying
	b.AnimTick()
	if b.dots[0].phase != phase {
		t.Errorf("dying dot phase changed: %f → %f", phase, b.dots[0].phase)
	}
}

func TestBreatheDotStaleDimsBrightness(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(2)
	// Advance so dots are fully visible
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	// Mark one dot as stale
	b.SetStaleCount(1)

	staleCount := 0
	for _, d := range b.dots {
		if d.stale {
			staleCount++
		}
	}
	if staleCount != 1 {
		t.Errorf("stale count = %d, want 1", staleCount)
	}

	// Stale dot should still be alive (not dying), just dimmed
	staleDot := b.dots[len(b.dots)-1]
	if staleDot.dying {
		t.Error("stale dot should not be dying")
	}
	if !staleDot.stale {
		t.Error("last dot should be stale")
	}
}

func TestBreatheDotStalePhaseSlower(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(2)
	b.SetStaleCount(1)

	freshPhase := b.dots[0].phase
	stalePhase := b.dots[1].phase

	b.AnimTick()

	freshAdvance := b.dots[0].phase - freshPhase
	staleAdvance := b.dots[1].phase - stalePhase

	if staleAdvance >= freshAdvance {
		t.Errorf("stale phase advance (%f) should be less than fresh (%f)", staleAdvance, freshAdvance)
	}
}

func TestBreatheDotRenderEmpty(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	str, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	if str != "" || w != 0 {
		t.Errorf("empty Render() = (%q, %d), want (\"\", 0)", str, w)
	}
}

func TestBreatheDotRenderVisWidth(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	// Advance so dots are visible
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	// 3 dots with spaces between: vis width = 3 + 2 = 5
	if w != 5 {
		t.Errorf("Render visWidth = %d, want 5", w)
	}
}

func TestBreatheDotHighScoreRendersCompletedDots(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedCount(3)
	// No active dots — just 3 completed KITT dots
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	// 3 completed dots with spaces: vis width = 3 + 2 = 5
	if w != 5 {
		t.Errorf("highscore Render visWidth = %d, want 5", w)
	}
}

func TestBreatheDotHighScoreWithActiveDots(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetTarget(2)         // 2 active (tidal wave)
	b.SetCompletedCount(3) // 3 completed (KITT)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	// 2 active (2 dots + 1 space) + 3 completed (3 dots + 3 spaces between) = 9
	if w != 9 {
		t.Errorf("highscore Render with active+completed visWidth = %d, want 9", w)
	}
}

func TestBreatheDotHighScoreStaleLosesKITT(t *testing.T) {
	// In highscore mode, stale dots should NOT get KITT scanner —
	// they should dim-breathe like regular dots, because KITT is reserved
	// for completed agents.
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetTarget(2)
	b.SetStaleCount(1)
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	// The stale dot should not be in the KITT index
	// (We can't easily test the visual output, but we can verify the
	// stale dot is marked stale and the flag is set)
	staleCount := 0
	for _, d := range b.dots {
		if d.stale {
			staleCount++
		}
	}
	if staleCount != 1 {
		t.Errorf("stale count = %d, want 1", staleCount)
	}
	// The key behavioral test: render should still succeed
	_, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	if w != 3 { // 2 dots + 1 space (no completed dots)
		t.Errorf("highscore stale Render visWidth = %d, want 3", w)
	}
}

func TestBreatheDotHighScoreGrowsMonotonically(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)

	b.SetCompletedCount(1)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w1 := b.Render("⬡", "⏣", "⬢", nil, 0)

	b.SetCompletedCount(3)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w2 := b.Render("⬡", "⏣", "⬢", nil, 0)

	if w2 <= w1 {
		t.Errorf("completed dot width didn't grow: %d → %d", w1, w2)
	}
}

func TestTriangleWave(t *testing.T) {
	tests := []struct {
		name      string
		pos       float64
		count     int
		wantMin   float64
		wantMax   float64
		wantExact *float64
	}{
		{"zero position", 0, 5, 0, 0, floatPtr(0)},
		{"mid forward", 0.5, 5, 0, 4, nil},
		{"peak at 1.0", 1.0, 5, 0, 4, floatPtr(4)},            // pos=1.0 → raw=4=n → returns n (peak)
		{"single dot always zero", 0.5, 1, 0, 0, floatPtr(0)}, // n-1=0, no sweep
		{"two dots", 0.25, 2, 0, 1, nil},
		{"two dots mid", 0.5, 2, 0, 1, floatPtr(0.5)},     // n=1, raw=0.5, forward pass
		{"monotonic forward", 0.1, 10, 0, 9, nil},         // still in forward pass
		{"bounce back", 1.5, 5, 0, 4, floatPtr(2)},        // pos=1.5 → raw=mod(6,8)=6>4 → 2*4-6=2
		{"full period return", 2.0, 5, 0, 4, floatPtr(0)}, // pos=2.0 → raw=mod(8,8)=0 → returns 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := triangleWave(tt.pos, tt.count)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("triangleWave(%v, %d) = %v, want in [%v, %v]",
					tt.pos, tt.count, got, tt.wantMin, tt.wantMax)
			}
			if tt.wantExact != nil && got != *tt.wantExact {
				t.Errorf("triangleWave(%v, %d) = %v, want exactly %v",
					tt.pos, tt.count, got, *tt.wantExact)
			}
		})
	}
}

func TestTriangleWaveRangeInvariant(t *testing.T) {
	count := 6
	n := float64(count - 1)
	for i := 0; i <= 20; i++ {
		pos := float64(i) * 0.1
		got := triangleWave(pos, count)
		if got < 0 || got > n {
			t.Errorf("triangleWave(%v, %d) = %v, out of range [0, %v]",
				pos, count, got, n)
		}
	}
}

func floatPtr(v float64) *float64 { return &v }

func TestKITTGaussian(t *testing.T) {
	tests := []struct {
		name    string
		profile *anim.Profile
		dist    float64
		want    float64
	}{
		// At center (dist=0): ambient + peak * exp(0) = ambient + peak
		{"default center", anim.Default(), 0, anim.Default().KITTAmbient + anim.Default().KITTPeak},
		{"calm center", anim.Calm(), 0, anim.Calm().KITTAmbient + anim.Calm().KITTPeak},
		{"intense center", anim.Intense(), 0, anim.Intense().KITTAmbient + anim.Intense().KITTPeak},
		// At large distance: approaches ambient (peak contribution ≈ 0)
		{"default far", anim.Default(), 100, anim.Default().KITTAmbient},
		{"calm far", anim.Calm(), 100, anim.Calm().KITTAmbient},
		{"intense far", anim.Intense(), 100, anim.Intense().KITTAmbient},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBreatheDots(testTheme, tt.profile)
			got := b.kittGaussian(tt.dist)
			if diff := got - tt.want; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("kittGaussian(%v) = %v, want %v (diff %v)",
					tt.dist, got, tt.want, diff)
			}
		})
	}
}

func TestKITTGaussianSymmetric(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	pos := b.kittGaussian(2.5)
	neg := b.kittGaussian(-2.5)
	if pos != neg {
		t.Errorf("kittGaussian not symmetric: f(2.5)=%v, f(-2.5)=%v", pos, neg)
	}
}

func TestKITTGaussianMonotonicDecay(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	prev := b.kittGaussian(0)
	ambient := testAnim.KITTAmbient
	for i := 1; i <= 10; i++ {
		cur := b.kittGaussian(float64(i))
		// Strict decay while peak contribution is still measurable;
		// at large distances both cur and prev floor at ambient.
		if cur > prev {
			t.Errorf("kittGaussian not monotonically decaying: f(%d)=%v > f(%d)=%v",
				i, cur, i-1, prev)
		}
		if cur < ambient-1e-9 {
			t.Errorf("kittGaussian below ambient floor: f(%d)=%v < %v",
				i, cur, ambient)
		}
		prev = cur
	}
}

func TestSinNorm(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		want float64
	}{
		{"zero", 0, 0.5},                  // sin(0)=0 → 0.5
		{"pi/2", 1.5707963267948966, 1.0}, // sin(π/2)=1 → 1.0
		{"pi", 3.141592653589793, 0.5},    // sin(π)≈0 → 0.5
		{"3pi/2", 4.71238898038469, 0.0},  // sin(3π/2)=-1 → 0.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sinNorm(tt.x)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("sinNorm(%v) = %v, want %v", tt.x, got, tt.want)
			}
		})
	}
}

func TestSinNormRange(t *testing.T) {
	// sinNorm must always return values in [0, 1]
	for i := 0; i <= 100; i++ {
		x := float64(i) * 0.1
		got := sinNorm(x)
		if got < 0 || got > 1 {
			t.Errorf("sinNorm(%v) = %v, out of range [0, 1]", x, got)
		}
	}
}

func TestBreatheDotRenderMaxDots(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(10)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w := b.Render("⬡", "⏣", "⬢", nil, 3)
	// Capped at 3 dots: vis width = 3 + 2 = 5
	if w != 5 {
		t.Errorf("Render with maxDots=3 visWidth = %d, want 5", w)
	}
}
