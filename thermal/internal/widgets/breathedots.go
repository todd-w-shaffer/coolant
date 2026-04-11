package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
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
	spring         harmonica.Spring
	dots           []breatheDot
	nextPhase      float64
	lastStale      int     // dirty check for SetStaleCount
	staleSweep     float64 // KITT scanner position — continuous, bounces across stale/completed dots
	tidalPhase     float64 // tidal wave phase for active dots — slow rolling swell
	theme          *theme.Theme
	anim           *anim.Profile
	highScore      bool // when true, KITT scans completed agents instead of stale ones
	completedCount int  // number of completed agents (highscore KITT dots)
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

// Render produces the styled dot string and its visible cell width.
// glyphHollow and glyphFilled alternate based on the breathing phase —
// the dot "fills up" at peak brightness and "empties" at the trough.
// bg is the cell background for transparency (nil = no background).
// maxDots caps visible dots (0 = unlimited).
func (b *BreatheDots) Render(glyphHollow, glyphMid, glyphFilled string, bg color.Color, maxDots int) (string, int) {
	if len(b.dots) == 0 && (!b.highScore || b.completedCount == 0) {
		return "", 0
	}

	dots := b.dots
	if maxDots > 0 && len(dots) > maxDots {
		dots = dots[:maxDots]
	}

	var buf strings.Builder
	visWidth := 0

	// Build index mappings for stale and active dots
	var staleIndices []int
	var activeIndices []int
	for i, d := range dots {
		if d.dying {
			continue
		}
		if d.stale {
			staleIndices = append(staleIndices, i)
		} else {
			activeIndices = append(activeIndices, i)
		}
	}

	// KITT sweep — used for stale dots (default mode) or completed dots (highscore mode)
	kittCount := len(staleIndices)
	if b.highScore {
		kittCount = b.completedCount
	}
	var sweepPos float64
	if kittCount > 1 {
		n := float64(kittCount - 1)
		raw := math.Mod(b.staleSweep*n, 2*n)
		if raw > n {
			sweepPos = 2*n - raw
		} else {
			sweepPos = raw
		}
	}

	for i, d := range dots {
		var brightness float64
		var wave float64

		if d.stale && !d.dying && !b.highScore && len(staleIndices) > 0 {
			// Default mode: KITT scanner on stale/ghost dots
			staleIdx := -1
			for si, idx := range staleIndices {
				if idx == i {
					staleIdx = si
					break
				}
			}
			if staleIdx >= 0 {
				dist := math.Abs(float64(staleIdx) - sweepPos)
				brightness = d.alive * b.anim.BreatheStaleDim * b.kittGaussian(dist)
			}
		} else if d.stale && !d.dying && b.highScore {
			// Highscore mode: stale dots dim-breathe (no KITT)
			brightness = d.alive * b.anim.BreatheStaleDim * sinNorm(d.phase)
		} else if !d.dying {
			// Tidal wave: slow rolling swell tuned for 3-5 visible dots.
			// Phase spread set by profile — wide enough for clear direction even with 3.
			// Narrow brightness range keeps all dots visible — the
			// glyph swap (⬡→⏣→⬢) is the primary visual signal.
			activeIdx := 0
			for ai, idx := range activeIndices {
				if idx == i {
					activeIdx = ai
					break
				}
			}
			wave = sinNorm(b.tidalPhase - float64(activeIdx)*b.anim.TidalPhaseSpread)
			individualBreath := sinNorm(d.phase)
			mixed := b.anim.TidalWaveMix*wave + b.anim.TidalBreathMix*individualBreath
			brightness = d.alive * (b.anim.TidalBrightFloor + b.anim.TidalBrightFloor*mixed)
		}

		if brightness < 0 {
			brightness = 0
		}
		if brightness > 1 {
			brightness = 1
		}

		// Glyph: three states track the tidal wave.
		// Peak → filled, shoulder → benzene, trough → hollow.
		// Stale dots stay hollow always.
		glyph := glyphHollow
		if !d.stale && !d.dying {
			switch {
			case wave > b.anim.GlyphFilledThresh:
				glyph = glyphFilled
			case wave > b.anim.GlyphMidThresh:
				glyph = glyphMid
			}
		}

		b.writeDot(&buf, &visWidth, glyph, brightness, bg, i > 0)
	}

	// Highscore mode: append completed agent KITT dots after active/stale dots
	if b.highScore && b.completedCount > 0 {
		hasPrior := len(dots) > 0
		for ci := 0; ci < b.completedCount; ci++ {
			var brightness float64
			if b.completedCount == 1 {
				brightness = b.anim.BreatheStaleDim * b.anim.KITTSingleBright
			} else {
				dist := math.Abs(float64(ci) - sweepPos)
				brightness = b.anim.BreatheStaleDim * b.kittGaussian(dist)
			}

			needSep := hasPrior || ci > 0
			b.writeDot(&buf, &visWidth, glyphFilled, brightness, bg, needSep)
		}
	}

	return buf.String(), visWidth
}

// writeDot appends a single styled dot to the buffer and updates visWidth.
func (b *BreatheDots) writeDot(buf *strings.Builder, visWidth *int, glyph string, brightness float64, bg color.Color, addSpace bool) {
	if addSpace {
		*visWidth++
		if bg != nil {
			buf.WriteString(lipgloss.NewStyle().Background(bg).Render(" "))
		} else {
			buf.WriteByte(' ')
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
	buf.WriteString(style.Render(glyph))
	*visWidth++
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

// SetCompletedCount sets the number of completed agents for highscore KITT scanning.
func (b *BreatheDots) SetCompletedCount(n int) {
	if n == b.completedCount {
		return
	}
	b.completedCount = n
}

// kittGaussian returns the KITT scanner brightness at distance from sweep center.
func (b *BreatheDots) kittGaussian(dist float64) float64 {
	return b.anim.KITTAmbient + b.anim.KITTPeak*math.Exp(-dist*dist/b.anim.KITTSigmaSq)
}

// sinNorm maps a sine wave to [0, 1].
func sinNorm(x float64) float64 {
	return 0.5 + 0.5*math.Sin(x)
}

// Len returns the current number of dots (including dying).
func (b *BreatheDots) Len() int {
	return len(b.dots)
}
