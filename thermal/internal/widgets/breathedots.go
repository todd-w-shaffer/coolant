package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// breatheDot tracks one dot's animation state.
type breatheDot struct {
	alive float64 // spring position: 0→1 fading in, 1→0 fading out
	vel   float64
	phase float64 // breathing phase accumulator (radians)
	dying bool
	stale bool // orphaned agent — breathes slower and dimmer
}

// BreatheDots manages a set of spring-animated breathing dots that track
// an integer count. Dots fade in on increase and fade out on decrease.
type BreatheDots struct {
	spring           harmonica.Spring
	dots             []breatheDot
	nextPhase        float64
	lastStale        int     // dirty check for SetStaleCount
	staleSweep       float64 // KITT scanner position — continuous, bounces across stale/completed dots
	tidalPhase       float64 // tidal wave phase for active dots — slow rolling swell
	theme            *theme.Theme
	anim             *anim.Profile
	highScore        bool     // when true, KITT scans completed agents instead of stale ones
	completedIDs     []string // agent IDs for completed agents (highscore KITT dots)
	renderedAgentIDs []string // cached snapshot of IDs zone-marked in last RenderSplit
	hoveredAgentID   string   // when non-empty, this dot renders at full brightness
}

func NewBreatheDots(th *theme.Theme, ap *anim.Profile) *BreatheDots {
	return &BreatheDots{
		spring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), ap.SpringFreq, ap.SpringDamping),
		theme:  th,
		anim:   ap,
	}
}

// SetTarget adjusts the dot count — spawns new dots or marks excess as dying.
func (b *BreatheDots) SetTarget(count int) {
	aliveCount := 0
	for _, d := range b.dots {
		if !d.dying {
			aliveCount++
		}
	}

	if count > aliveCount {
		for i := 0; i < count-aliveCount; i++ {
			b.nextPhase += 0.7
			b.dots = append(b.dots, breatheDot{phase: b.nextPhase})
		}
		b.lastStale = -1 // invalidate dirty check
	} else if count < aliveCount {
		toKill := aliveCount - count
		for i := len(b.dots) - 1; i >= 0 && toKill > 0; i-- {
			if !b.dots[i].dying {
				b.dots[i].dying = true
				toKill--
			}
		}
		b.lastStale = -1 // invalidate dirty check
	}
}

// AnimTick advances spring physics and breathing phases.
func (b *BreatheDots) AnimTick() {
	for i := range b.dots {
		target := 1.0
		if b.dots[i].dying {
			target = 0.0
		}
		b.dots[i].alive, b.dots[i].vel = b.spring.Update(
			b.dots[i].alive, b.dots[i].vel, target,
		)
		if !b.dots[i].dying {
			if b.dots[i].stale {
				b.dots[i].phase += b.anim.BreathePhaseStep * b.anim.BreatheStaleRate
			} else {
				b.dots[i].phase += b.anim.BreathePhaseStep
			}
		}
	}

	b.staleSweep += b.anim.KITTSweepRate
	b.tidalPhase += b.anim.TidalPhaseStep

	// Remove fully faded dots
	n := 0
	for _, d := range b.dots {
		if !(d.dying && d.alive < config.BreatheFadeEps) {
			b.dots[n] = d
			n++
		}
	}
	b.dots = b.dots[:n]
}

// dotFrame holds per-dot precomputed state: its position among stale/active
// peers and its live/stale classification. Building these up front avoids
// O(N²) index lookups inside the render loop.
type dotFrame struct {
	dot       *breatheDot
	staleIdx  int // -1 if active
	activeIdx int // -1 if stale
}

// prepareDots returns per-dot frames (skipping dying) and the KITT sweep
// position. The same slice of frames feeds Render and RenderSplit.
func (b *BreatheDots) prepareDots(maxDots int) (frames []dotFrame, sweepPos float64, staleCount int) {
	dots := b.dots
	if maxDots > 0 && len(dots) > maxDots {
		dots = dots[:maxDots]
	}
	frames = make([]dotFrame, 0, len(dots))
	for i := range dots {
		d := &dots[i]
		if d.dying {
			continue
		}
		f := dotFrame{dot: d, staleIdx: -1, activeIdx: -1}
		if d.stale {
			f.staleIdx = staleCount
			staleCount++
		} else {
			f.activeIdx = len(frames) - staleCount
		}
		frames = append(frames, f)
	}
	kittCount := staleCount
	if b.highScore {
		kittCount = len(b.completedIDs)
		if kittCount > config.KITTMaxDots {
			kittCount = config.KITTMaxDots
		}
	}
	sweepPos = triangleWave(b.staleSweep, kittCount)
	return frames, sweepPos, staleCount
}

// computeDot returns the brightness and glyph for one prepared frame.
func (b *BreatheDots) computeDot(f dotFrame, sweepPos float64, hasStales bool, glyphHollow, glyphMid, glyphFilled string) (glyph string, brightness float64) {
	d := f.dot
	var wave float64
	switch {
	case d.stale && !b.highScore && hasStales:
		if f.staleIdx >= 0 {
			dist := math.Abs(float64(f.staleIdx) - sweepPos)
			brightness = d.alive * b.anim.BreatheStaleDim * b.kittGaussian(dist)
		}
	case d.stale && b.highScore:
		brightness = d.alive * b.anim.BreatheStaleDim * sinNorm(d.phase)
	default:
		wave = sinNorm(b.tidalPhase - float64(f.activeIdx)*b.anim.TidalPhaseSpread)
		mixed := b.anim.TidalWaveMix*wave + b.anim.TidalBreathMix*sinNorm(d.phase)
		brightness = d.alive * (b.anim.TidalBrightFloor + b.anim.TidalBrightFloor*mixed)
	}

	if brightness < 0 {
		brightness = 0
	}
	if brightness > 1 {
		brightness = 1
	}

	glyph = glyphHollow
	if !d.stale {
		switch {
		case wave > b.anim.GlyphFilledThresh:
			glyph = glyphFilled
		case wave > b.anim.GlyphMidThresh:
			glyph = glyphMid
		}
	}
	return glyph, brightness
}

// highscoreCompletedBrightness computes the KITT brightness for the ci'th
// completed-agent dot rendered in highscore mode. When the dot's agent ID
// matches hoveredAgentID, returns full intensity (bypass KITT gaussian).
func (b *BreatheDots) highscoreCompletedBrightness(ci int, sweepPos float64) float64 {
	if b.hoveredAgentID != "" && ci < len(b.completedIDs) && b.completedIDs[ci] == b.hoveredAgentID {
		return 1.0
	}
	if len(b.completedIDs) == 1 {
		return b.anim.BreatheStaleDim * b.anim.KITTSingleBright
	}
	dist := math.Abs(float64(ci) - sweepPos)
	return b.anim.BreatheStaleDim * b.kittGaussian(dist)
}

// SetHoveredAgent sets the agent ID that should render at full brightness
// (hover feedback). Pass "" to clear.
func (b *BreatheDots) SetHoveredAgent(id string) {
	b.hoveredAgentID = id
}

// Render produces the styled dot string and its visible cell width.
// glyphHollow and glyphFilled alternate based on the breathing phase —
// the dot "fills up" at peak brightness and "empties" at the trough.
// bg is the cell background for transparency (nil = no background).
// maxDots caps visible dots (0 = unlimited).
func (b *BreatheDots) Render(glyphHollow, glyphMid, glyphFilled string, bg color.Color, maxDots int) (string, int) {
	if len(b.dots) == 0 && (!b.highScore || len(b.completedIDs) == 0) {
		return "", 0
	}

	frames, sweepPos, staleCount := b.prepareDots(maxDots)
	var buf strings.Builder
	visWidth := 0

	for idx, f := range frames {
		glyph, brightness := b.computeDot(f, sweepPos, staleCount > 0, glyphHollow, glyphMid, glyphFilled)
		b.writeDot(&buf, &visWidth, glyph, brightness, bg, idx > 0, "")
	}

	if b.highScore && len(b.completedIDs) > 0 {
		renderCount := len(b.completedIDs)
		if renderCount > config.KITTMaxDots {
			renderCount = config.KITTMaxDots
		}
		hasPrior := len(frames) > 0
		for ci := 0; ci < renderCount; ci++ {
			brightness := b.highscoreCompletedBrightness(ci, sweepPos)
			needSep := hasPrior || ci > 0
			b.writeDot(&buf, &visWidth, glyphFilled, brightness, bg, needSep, "")
		}
	}

	return buf.String(), visWidth
}

// RenderSplit renders active dots and ghost dots (stale + highscore completed)
// into two independent strings so layout code can place them in different
// columns. Each side handles its own first-dot separator so the fragments
// stand alone.
func (b *BreatheDots) RenderSplit(glyphHollow, glyphMid, glyphFilled string, bg color.Color, maxDots int) (ghostStr, activeStr string, ghostWidth, activeWidth int) {
	if len(b.dots) == 0 && (!b.highScore || len(b.completedIDs) == 0) {
		return "", "", 0, 0
	}

	frames, sweepPos, staleCount := b.prepareDots(maxDots)
	var ghostBuf, activeBuf strings.Builder
	ghostSeen, activeSeen := false, false

	for _, f := range frames {
		glyph, brightness := b.computeDot(f, sweepPos, staleCount > 0, glyphHollow, glyphMid, glyphFilled)
		if f.dot.stale {
			b.writeDot(&ghostBuf, &ghostWidth, glyph, brightness, bg, ghostSeen, "")
			ghostSeen = true
		} else {
			b.writeDot(&activeBuf, &activeWidth, glyph, brightness, bg, activeSeen, "")
			activeSeen = true
		}
	}

	// Reset rendered agent ID cache for this frame.
	b.renderedAgentIDs = b.renderedAgentIDs[:0]

	if b.highScore && len(b.completedIDs) > 0 {
		renderCount := len(b.completedIDs)
		if renderCount > config.KITTMaxDots {
			renderCount = config.KITTMaxDots
		}
		for ci := 0; ci < renderCount; ci++ {
			agentID := b.completedIDs[ci]
			brightness := b.highscoreCompletedBrightness(ci, sweepPos)
			b.writeDot(&ghostBuf, &ghostWidth, glyphFilled, brightness, bg, ghostSeen, agentID)
			ghostSeen = true
			b.renderedAgentIDs = append(b.renderedAgentIDs, agentID)
		}
	}

	return ghostBuf.String(), activeBuf.String(), ghostWidth, activeWidth
}

// writeDot appends a single styled dot to the buffer and updates visWidth.
// When agentID is non-empty, the separator space + glyph are wrapped in a
// zone.Mark for click targeting (2-cell hit area). Width accounting stays
// manual — zone markers are zero-width to lipgloss.Width but inflate len().
func (b *BreatheDots) writeDot(buf *strings.Builder, visWidth *int, glyph string, brightness float64, bg color.Color, addSpace bool, agentID string) {
	// Hot path (30fps): write directly to buf when no zone wrapping needed.
	dst := buf
	var local strings.Builder
	if agentID != "" {
		dst = &local
	}

	if addSpace {
		*visWidth++
		if bg != nil {
			dst.WriteString(lipgloss.NewStyle().Background(bg).Render(" "))
		} else {
			dst.WriteByte(' ')
		}
	}

	fg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		uint8(b.theme.AccentR*brightness),
		uint8(b.theme.AccentG*brightness),
		uint8(b.theme.AccentB*brightness),
	))
	style := lipgloss.NewStyle().Foreground(fg)
	if bg != nil {
		style = style.Background(bg)
	}
	dst.WriteString(style.Render(glyph))
	*visWidth++

	if agentID != "" {
		buf.WriteString(zone.Mark(ui.AgentZoneID(agentID), local.String()))
	}
}

// SetStaleCount marks the last n non-dying dots as stale (orphaned).
// Stale dots breathe slower and render dimmer.
func (b *BreatheDots) SetStaleCount(n int) {
	if n == b.lastStale {
		return
	}
	b.lastStale = n
	// Clear all stale flags then re-mark
	for i := range b.dots {
		b.dots[i].stale = false
	}
	marked := 0
	for i := len(b.dots) - 1; i >= 0 && marked < n; i-- {
		if !b.dots[i].dying {
			b.dots[i].stale = true
			marked++
		}
	}
}

// SetHighScoreMode toggles KITT scanner behavior.
// When true, KITT scans completed agents (earned achievement); stale dots dim-breathe.
// When false (default), KITT scans stale/ghost dots.
func (b *BreatheDots) SetHighScoreMode(on bool) {
	b.highScore = on
}

// SetCompletedAgents sets the completed agent IDs for highscore KITT scanning.
// Count is derived from len(ids) — one method, no desync.
func (b *BreatheDots) SetCompletedAgents(ids []string) {
	b.completedIDs = ids
}

// triangleWave returns a position that bounces linearly between 0 and count-1.
// pos is a continuous input (e.g. staleSweep * n); count is the number of dots.
// Returns 0 when count <= 1.
func triangleWave(pos float64, count int) float64 {
	if count <= 1 {
		return 0
	}
	n := float64(count - 1)
	raw := math.Mod(pos*n, 2*n)
	if raw < 0 {
		raw += 2 * n
	}
	if raw > n {
		return 2*n - raw
	}
	return raw
}

// kittGaussian returns the KITT scanner brightness at distance from sweep center.
func (b *BreatheDots) kittGaussian(dist float64) float64 {
	return b.anim.KITTAmbient + b.anim.KITTPeak*math.Exp(-dist*dist/b.anim.KITTSigmaSq)
}

// sinNorm maps a sine wave to [0, 1].
func sinNorm(x float64) float64 {
	return 0.5 + 0.5*math.Sin(x)
}

// RenderedAgentIDs returns the cached snapshot of completed-agent IDs that
// were zone-marked in the most recent RenderSplit call. The click handler
// iterates only these (at most KITTMaxDots), not the full ring buffer.
func (b *BreatheDots) RenderedAgentIDs() []string {
	return b.renderedAgentIDs
}

// Len returns the current number of dots (including dying).
func (b *BreatheDots) Len() int {
	return len(b.dots)
}
