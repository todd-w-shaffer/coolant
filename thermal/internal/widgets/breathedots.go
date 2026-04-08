package widgets

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
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
	spring    harmonica.Spring
	dots      []breatheDot
	nextPhase float64
	lastStale int // dirty check for SetStaleCount
}

func NewBreatheDots() *BreatheDots {
	return &BreatheDots{
		spring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), config.SpringFreq, config.SpringDamping),
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
				b.dots[i].phase += config.BreathePhaseStep * config.BreatheStaleRate
			} else {
				b.dots[i].phase += config.BreathePhaseStep
			}
		}
	}

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
func (b *BreatheDots) Render(glyphHollow, glyphFilled string, bg color.Color, maxDots int) (string, int) {
	if len(b.dots) == 0 {
		return "", 0
	}

	dots := b.dots
	if maxDots > 0 && len(dots) > maxDots {
		dots = dots[:maxDots]
	}

	var buf strings.Builder
	visWidth := 0

	for i, d := range dots {
		breathT := 0.5 + 0.5*math.Sin(d.phase)
		brightness := d.alive * (config.BreatheMinBright + (config.BreatheMaxBright-config.BreatheMinBright)*breathT)
		if d.stale {
			brightness *= config.BreatheStaleDim
		}
		if brightness < 0 {
			brightness = 0
		}
		if brightness > 1 {
			brightness = 1
		}

		// Pick glyph: filled above midpoint, hollow below
		glyph := glyphHollow
		if breathT > 0.5 {
			glyph = glyphFilled
		}

		fg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
			uint8(config.BreatheBaseR*brightness),
			uint8(config.BreatheBaseG*brightness),
			uint8(config.BreatheBaseB*brightness),
		))

		if i > 0 {
			visWidth++
			if bg != nil {
				buf.WriteString(lipgloss.NewStyle().Background(bg).Render(" "))
			} else {
				buf.WriteByte(' ')
			}
		}

		style := lipgloss.NewStyle().Foreground(fg)
		if bg != nil {
			style = style.Background(bg)
		}
		buf.WriteString(style.Render(glyph))
		visWidth++
	}

	return buf.String(), visWidth
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

// Len returns the current number of dots (including dying).
func (b *BreatheDots) Len() int {
	return len(b.dots)
}
