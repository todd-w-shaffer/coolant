package main

import (
	"fmt"
	"math/rand"
	"time"
)

// ── Tunables ──────────────────────────────────────────────────
const (
	width     = 80 // characters wide
	height    = 25 // characters tall
	numComets = 10 // simultaneous comets
	trailLen  = 6  // trail length in characters
	fps       = 30
)

// Trail brightness ramp: index 0 = the comet head (full), rest = tail fading out.
var trailRamp = [trailLen]float64{1.0, 0.70, 0.45, 0.28, 0.16, 0.08}

// Comet colors (R, G, B).
var cometColors = [][3]int{
	{34, 197, 94},  // green
	{234, 179, 8},  // yellow
	{239, 68, 68},  // red
	{56, 189, 248}, // cyan
	{168, 85, 247}, // purple
}

// ── Comet state ───────────────────────────────────────────────

type comet struct {
	row   int    // y position (0 = top)
	col   int    // x position of head (moves right to left)
	dot   rune   // braille pattern
	color [3]int // RGB
}

func newComet() comet {
	dots := []rune{0x2840, 0x2880, 0x2820, 0x2810, 0x2844, 0x28C0} // various single/double dot patterns
	c := cometColors[rand.Intn(len(cometColors))]
	return comet{
		row:   rand.Intn(height),
		col:   width - 1 + rand.Intn(trailLen), // start off-screen right
		dot:   dots[rand.Intn(len(dots))],
		color: c,
	}
}

// ── Render ────────────────────────────────────────────────────

func main() {
	fmt.Print("\033[?25l") // hide cursor
	fmt.Print("\033[2J")   // clear screen
	defer fmt.Print("\033[?25h\n")

	comets := make([]comet, numComets)
	for i := range comets {
		comets[i] = newComet()
		comets[i].col = rand.Intn(width) // scatter initial positions
	}

	// Screen buffer: [row][col] = {char, r, g, b}
	type cell struct {
		ch      rune
		r, g, b int
	}

	for frame := 0; frame < fps*15; frame++ { // 15 seconds
		// Clear buffer
		buf := make([][]cell, height)
		for y := range buf {
			buf[y] = make([]cell, width)
		}

		// Stamp each comet + trail into buffer
		for i := range comets {
			c := &comets[i]
			for t := 0; t < trailLen; t++ {
				x := c.col + t // head is at col, trail extends rightward (behind it)
				if x < 0 || x >= width {
					continue
				}
				alpha := trailRamp[t]
				cr := int(float64(c.color[0]) * alpha)
				cg := int(float64(c.color[1]) * alpha)
				cb := int(float64(c.color[2]) * alpha)
				buf[c.row][x] = cell{ch: c.dot, r: cr, g: cg, b: cb}
			}

			// Move comet left
			c.col--

			// Respawn when fully off-screen
			if c.col+trailLen < 0 {
				*c = newComet()
			}
		}

		// Render buffer
		fmt.Print("\033[H") // home cursor
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				c := buf[y][x]
				if c.ch != 0 {
					fmt.Printf("\033[38;2;%d;%d;%dm%c", c.r, c.g, c.b, c.ch)
				} else {
					fmt.Print("\033[0m ")
				}
			}
			fmt.Print("\033[0m\n")
		}

		time.Sleep(time.Second / fps)
	}
}
