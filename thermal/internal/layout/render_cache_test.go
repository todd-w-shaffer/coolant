package layout

import (
	"testing"

	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
)

// TestRenderContentMatchesFreshScan pins correctness of the scan cache: the
// content RenderContent returns in mouse mode must be byte-identical to a fresh
// zone.Scan of the pure View() string. The cache may only ever skip the scan
// when the composed bytes are unchanged — it must never return a stale frame.
func TestRenderContentMatchesFreshScan(t *testing.T) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)

	// Mouse mode: RenderContent zone-scans. Must equal a fresh scan of View().
	got := h.RenderContent(true)
	want := zone.Scan(h.View())
	if got != want {
		t.Errorf("RenderContent(true) != zone.Scan(View()):\n got=%q\nwant=%q", got, want)
	}

	// No-mouse mode returns the raw frame untouched.
	if raw := h.RenderContent(false); raw != h.View() {
		t.Errorf("RenderContent(false) != View():\n got=%q\nwant=%q", raw, h.View())
	}
}

// TestRenderContentScanCacheSurvivesAnimacrossCalm proves the scan cache stays
// correct as frames advance: after a category-filter mutation done directly on
// AppState (the path main.go uses, which never calls layout.Update), the next
// RenderContent must reflect the change — i.e. the cache must not be keyed on a
// dirty flag that this mutation would bypass.
func TestRenderContentReflectsStateMutationBypassingUpdate(t *testing.T) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)

	before := h.RenderContent(true)
	// Mutate the filter straight on AppState, as a keypress handler does — no
	// layout.Update / AnimTick / SetSize in between.
	h.State().ToggleCategoryFilter("build")
	after := h.RenderContent(true)

	if before == after {
		t.Errorf("RenderContent did not reflect a category-filter change applied "+
			"directly on AppState — the cache is keyed on a dirty flag that this "+
			"mutation bypasses (stale-frame bug):\n%q", after)
	}
}

// TestRenderContentMemoHitReturnsFreshScan exercises the memo-hit branch
// (scanValid && rawSig unchanged): a second identical-byte mouse render must
// return content byte-equal to a fresh zone.Scan of the frame (not a stale
// cache) and must still advance the stability counter.
func TestRenderContentMemoHitReturnsFreshScan(t *testing.T) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)

	first := h.RenderContent(true) // scan miss → scans + caches
	c0 := h.FrameStableCount()
	second := h.RenderContent(true) // identical frame → memo hit
	if second != zone.Scan(h.View()) {
		t.Errorf("memo-hit content is not byte-equal to a fresh scan (stale cache)")
	}
	if second != first {
		t.Errorf("memo-hit content differs from the first scan of the same frame")
	}
	if h.FrameStableCount() <= c0 {
		t.Errorf("memo hit did not advance the stability counter: %d → %d", c0, h.FrameStableCount())
	}
}

// TestRenderContentMouseToggleInvalidatesScan proves the !mouse path invalidates
// the scan cache: if the frame changes while mouse is off, the next mouse-on
// render must re-scan the new frame, not return the scan captured before the
// off interval.
func TestRenderContentMouseToggleInvalidatesScan(t *testing.T) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)

	_ = h.RenderContent(true) // mouse on: scans frame A, caches it
	// Change the frame while mouse is OFF (filter dims category cells).
	h.State().ToggleCategoryFilter("build")
	_ = h.RenderContent(false)   // mouse off: raw frame B, must invalidate scan cache
	got := h.RenderContent(true) // mouse on again: must re-scan frame B
	if got != zone.Scan(h.View()) {
		t.Errorf("mouse on→off→on returned a stale scan; expected a fresh scan of the changed frame")
	}
}

// TestFrameStableCountTracksByteStability proves the consecutive-identical-frame
// counter, driven by RenderContent, rises while the composed frame is byte-stable
// (calm) and resets to zero the moment the frame changes (token burst). This is
// the settle signal the model's tick-stop gates on.
func TestFrameStableCountTracksByteStability(t *testing.T) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)

	// Fill + settle into calm with a byte-stable frame.
	for i := 0; i < config.MaxRenderHistory+config.CalmStableSnapshots+5; i++ {
		h.State().Update(calmSteadySnap())
		h.Update(h.State())
		h.AnimTick()
		h.RenderContent(true)
	}
	if !h.State().IsCalm() {
		t.Fatalf("expected calm after fill+settle")
	}

	// Drive a stretch of calm frames; the counter must climb every frame.
	start := h.FrameStableCount()
	for i := 0; i < config.AnimFPS; i++ {
		h.AnimTick()
		h.RenderContent(true)
	}
	if got := h.FrameStableCount(); got < start+config.AnimFPS {
		t.Errorf("FrameStableCount did not climb across a calm stretch: start=%d got=%d (want ≥ %d)",
			start, got, start+config.AnimFPS)
	}

	// A token burst changes the frame → counter resets.
	burst := calmSteadySnap()
	burst.Tokens.IOTokensPerSec = 1500
	h.State().Update(burst)
	h.Update(h.State())
	h.AnimTick()
	h.RenderContent(true)
	if got := h.FrameStableCount(); got != 0 {
		t.Errorf("FrameStableCount did not reset on a changed frame: got=%d want=0", got)
	}
}
