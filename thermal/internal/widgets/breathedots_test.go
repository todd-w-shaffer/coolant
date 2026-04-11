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
