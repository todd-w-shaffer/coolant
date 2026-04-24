package widgets

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

var testTheme = theme.Classic()
var testAnim = anim.Default()

// TestRenderSplit_ActiveOnly — all dots active: ghosts are empty, actives
// contain the entire render.
func TestRenderSplit_ActiveOnly(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	for i := 0; i < 40; i++ {
		b.AnimTick()
	}
	ghostStr, _, ghostW, activeW := b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	if ghostW != 0 || ghostStr != "" {
		t.Errorf("all-active: ghost should be empty, got %q (w=%d)", ghostStr, ghostW)
	}
	if activeW == 0 {
		t.Errorf("all-active: expected non-zero active width")
	}
	// Expected width: 3 glyphs + 2 separators = 5 cells.
	if activeW != 5 {
		t.Errorf("all-active: activeW=%d want 5", activeW)
	}
}

// TestRenderSplit_WithStales — 3 active + 2 stale: ghosts carry stales,
// actives carry live dots, each with its own leading-separator handling.
func TestRenderSplit_WithStales(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(5)
	b.SetStaleCount(2)
	for i := 0; i < 40; i++ {
		b.AnimTick()
	}
	_, _, ghostW, activeW := b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	if ghostW == 0 {
		t.Errorf("expected non-zero ghost width with 2 stales")
	}
	if activeW == 0 {
		t.Errorf("expected non-zero active width with 3 actives")
	}
}

// TestRenderSplit_Highscore — completed dots route to the ghost side so
// the highscore KITT trail doesn't push active agents around.
func TestRenderSplit_Highscore(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetTarget(2)
	b.SetCompletedAgents([]string{"a1", "b2", "c3"})
	for i := 0; i < 40; i++ {
		b.AnimTick()
	}
	_, _, ghostW, activeW := b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	if ghostW == 0 {
		t.Errorf("highscore: expected ghost width > 0 for 3 completed dots")
	}
	if activeW == 0 {
		t.Errorf("highscore: expected non-zero active width for 2 live dots")
	}
}

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
	b.SetCompletedAgents([]string{"a1", "b2", "c3"})
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
	b.SetTarget(2)                                   // 2 active (tidal wave)
	b.SetCompletedAgents([]string{"a1", "b2", "c3"}) // 3 completed (KITT)
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

	b.SetCompletedAgents([]string{"a1"})
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w1 := b.Render("⬡", "⏣", "⬢", nil, 0)

	b.SetCompletedAgents([]string{"a1", "b2", "c3"})
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	_, w2 := b.Render("⬡", "⏣", "⬢", nil, 0)

	if w2 <= w1 {
		t.Errorf("completed dot width didn't grow: %d → %d", w1, w2)
	}
}

func TestBreatheDotHighScoreCapsAtMax(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	ids := make([]string, config.KITTMaxDots+20)
	for i := range ids {
		ids[i] = fmt.Sprintf("agent-%d", i)
	}
	b.SetCompletedAgents(ids)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}

	_, w := b.Render("⬡", "⏣", "⬢", nil, 0)
	// Each dot is 1 cell + 1 space separator (except first). Max visible = 2*max - 1.
	maxVisible := 2*config.KITTMaxDots - 1
	if w > maxVisible {
		t.Errorf("rendered width %d exceeds cap %d (max %d dots)", w, maxVisible, config.KITTMaxDots)
	}

	// Also verify RenderSplit respects the cap
	_, _, gw, _ := b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	if gw > maxVisible {
		t.Errorf("RenderSplit ghost width %d exceeds cap %d", gw, maxVisible)
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

func assertNear(t *testing.T, got, want, eps float64, label string) {
	t.Helper()
	if diff := got - want; diff > eps || diff < -eps {
		t.Errorf("%s = %v, want %v (diff %v, eps %v)", label, got, want, diff, eps)
	}
}

func TestKITTGaussian(t *testing.T) {
	tests := []struct {
		name    string
		profile *anim.Profile
		dist    float64
		center  bool // true: want = ambient+peak; false: want = ambient
	}{
		{"default center", anim.Default(), 0, true},
		{"calm center", anim.Calm(), 0, true},
		{"intense center", anim.Intense(), 0, true},
		{"default far", anim.Default(), 100, false},
		{"calm far", anim.Calm(), 100, false},
		{"intense far", anim.Intense(), 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBreatheDots(testTheme, tt.profile)
			got := b.kittGaussian(tt.dist)
			want := tt.profile.KITTAmbient
			if tt.center {
				want += tt.profile.KITTPeak
			}
			assertNear(t, got, want, 1e-9, fmt.Sprintf("kittGaussian(%v)", tt.dist))
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
		{"zero", 0, 0.5},
		{"pi/2", math.Pi / 2, 1.0},
		{"pi", math.Pi, 0.5},
		{"3pi/2", 3 * math.Pi / 2, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sinNorm(tt.x)
			assertNear(t, got, tt.want, 1e-9, fmt.Sprintf("sinNorm(%v)", tt.x))
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

func TestRenderSplitCachesRenderedAgentIDs(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedAgents([]string{"aaa", "bbb", "ccc"})
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	ids := b.RenderedAgentIDs()
	if len(ids) != 3 {
		t.Fatalf("RenderedAgentIDs len = %d, want 3", len(ids))
	}
	want := []string{"aaa", "bbb", "ccc"}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("RenderedAgentIDs[%d] = %q, want %q", i, id, want[i])
		}
	}
}

func TestRenderSplitCapsRenderedAgentIDs(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	ids := make([]string, config.KITTMaxDots+5)
	for i := range ids {
		ids[i] = fmt.Sprintf("agent-%d", i)
	}
	b.SetCompletedAgents(ids)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	rendered := b.RenderedAgentIDs()
	if len(rendered) != config.KITTMaxDots {
		t.Errorf("RenderedAgentIDs len = %d, want %d (capped)", len(rendered), config.KITTMaxDots)
	}
}

func TestRenderedAgentIDsRefreshesOnNewIDs(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedAgents([]string{"old1", "old2"})
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	ids1 := b.RenderedAgentIDs()
	if len(ids1) != 2 || ids1[0] != "old1" {
		t.Fatalf("first render: got %v, want [old1 old2]", ids1)
	}

	// Replace with new IDs and re-render.
	b.SetCompletedAgents([]string{"new1", "new2", "new3"})
	b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	ids2 := b.RenderedAgentIDs()
	if len(ids2) != 3 {
		t.Fatalf("second render: len = %d, want 3", len(ids2))
	}
	if ids2[0] != "new1" || ids2[1] != "new2" || ids2[2] != "new3" {
		t.Errorf("second render: got %v, want [new1 new2 new3]", ids2)
	}
}

func TestRenderedAgentIDsEmptyWithoutHighscore(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetTarget(3)
	for i := 0; i < 30; i++ {
		b.AnimTick()
	}
	b.RenderSplit("⬡", "⏣", "⬢", nil, 0)
	ids := b.RenderedAgentIDs()
	if len(ids) != 0 {
		t.Errorf("RenderedAgentIDs without highscore should be empty, got %d", len(ids))
	}
}

func TestHoverOverridesBrightnessToFull(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedAgents([]string{"aaa", "bbb", "ccc"})
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	// Without hover, completed brightness < 1.0 for non-center dots
	normalBright := b.highscoreCompletedBrightness(2, 0) // dot 2, sweep at 0 — should be dim
	if normalBright >= 1.0 {
		t.Fatalf("precondition: non-center dot brightness should be < 1.0, got %f", normalBright)
	}

	// With hover on "ccc" (index 2), brightness should be 1.0
	b.SetHoveredAgent("ccc")
	hoveredBright := b.highscoreCompletedBrightness(2, 0)
	if hoveredBright != 1.0 {
		t.Errorf("hovered dot brightness = %f, want 1.0", hoveredBright)
	}
}

func TestHoverNonExistentAgentIsNoOp(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedAgents([]string{"aaa", "bbb"})
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	// Hover on an ID not in completedIDs — should not affect brightness.
	b.SetHoveredAgent("zzz_nonexistent")
	bright0 := b.highscoreCompletedBrightness(0, 0)
	bright1 := b.highscoreCompletedBrightness(1, 0)
	// Neither dot should be at full intensity.
	if bright0 == 1.0 {
		t.Error("dot 0 should not be full brightness when hovering non-existent ID")
	}
	if bright1 == 1.0 {
		t.Error("dot 1 should not be full brightness when hovering non-existent ID")
	}
}

func TestHoverClearsOnEmpty(t *testing.T) {
	b := NewBreatheDots(testTheme, testAnim)
	b.SetHighScoreMode(true)
	b.SetCompletedAgents([]string{"aaa"})
	b.SetHoveredAgent("aaa")
	b.SetHoveredAgent("") // clear
	// After clearing, brightness should revert to normal
	for i := 0; i < 60; i++ {
		b.AnimTick()
	}
	bright := b.highscoreCompletedBrightness(0, 0)
	// Single dot uses KITTSingleBright, not full intensity
	if bright >= 1.0 {
		t.Errorf("after clearing hover, brightness should revert, got %f", bright)
	}
}

// TestBreatheInterpolatesFromBg asserts the ±1/channel midpoint tolerance
// (uint8 truncation) and the nil-bg → (0,0,0) fallback. Without the latter,
// every existing dark-theme breath test would regress.
func TestBreatheInterpolatesFromBg(t *testing.T) {
	ar, ag, ab := theme.Classic().AccentR, theme.Classic().AccentG, theme.Classic().AccentB

	cases := []struct {
		name      string
		bg        color.Color
		bgR, g, b uint8
	}{
		{"nil bg (pre-latte)", nil, 0, 0, 0},
		{"synthetic black", lipgloss.Color("#000000"), 0, 0, 0},
		{"synthetic white", lipgloss.Color("#ffffff"), 0xff, 0xff, 0xff},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotR, gotG, gotB := rgb8(BreatheFg(ar, ag, ab, 0, tc.bg))
			if gotR != tc.bgR || gotG != tc.g || gotB != tc.b {
				t.Errorf("brightness=0: got (%d,%d,%d), want bg (%d,%d,%d)",
					gotR, gotG, gotB, tc.bgR, tc.g, tc.b)
			}

			gotR, gotG, gotB = rgb8(BreatheFg(ar, ag, ab, 1, tc.bg))
			if gotR != uint8(ar) || gotG != uint8(ag) || gotB != uint8(ab) {
				t.Errorf("brightness=1: got (%d,%d,%d), want accent (%d,%d,%d)",
					gotR, gotG, gotB, uint8(ar), uint8(ag), uint8(ab))
			}

			midR := uint8(float64(tc.bgR) + (ar-float64(tc.bgR))*0.5)
			midG := uint8(float64(tc.g) + (ag-float64(tc.g))*0.5)
			midB := uint8(float64(tc.b) + (ab-float64(tc.b))*0.5)
			gotR, gotG, gotB = rgb8(BreatheFg(ar, ag, ab, 0.5, tc.bg))
			if absDiff(gotR, midR) > 1 || absDiff(gotG, midG) > 1 || absDiff(gotB, midB) > 1 {
				t.Errorf("brightness=0.5: got (%d,%d,%d), want (%d,%d,%d) ±1",
					gotR, gotG, gotB, midR, midG, midB)
			}
		})
	}
}

func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
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
