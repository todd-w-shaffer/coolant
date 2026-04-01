package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Waveform renders overlaid spawn/death braille waveforms.
type Waveform struct {
	spawnHistory []int
	deathHistory []int
	maxHistory   int
	currSpawns   int
	currDeaths   int
	currNet      int
	width        int
	height       int
}

func NewWaveform() *Waveform {
	return &Waveform{
		maxHistory: 240,
	}
}

func (w *Waveform) SetSize(width, height int) {
	w.width = width
	w.height = height
}

func (w *Waveform) Update(spawns, deaths, net int) {
	w.currSpawns = spawns
	w.currDeaths = deaths
	w.currNet = net
	w.spawnHistory = append(w.spawnHistory, spawns)
	w.deathHistory = append(w.deathHistory, deaths)
	if len(w.spawnHistory) > w.maxHistory {
		w.spawnHistory = w.spawnHistory[len(w.spawnHistory)-w.maxHistory:]
		w.deathHistory = w.deathHistory[len(w.deathHistory)-w.maxHistory:]
	}
}

func (w *Waveform) View() string {
	if len(w.spawnHistory) == 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render("  waiting for data...")
	}

	// Header
	spawnColor := ui.ThresholdColor(w.currSpawns, ui.SpawnWarn, ui.SpawnCrit)
	netAbs := w.currNet
	if netAbs < 0 {
		netAbs = -netAbs
	}
	netColor := ui.ThresholdColor(netAbs, ui.NetWarn, ui.NetCrit)
	netSign := "+"
	if w.currNet < 0 {
		netSign = ""
	}

	header := fmt.Sprintf("  %s  %s  %s",
		lipgloss.NewStyle().Foreground(spawnColor).Render(fmt.Sprintf("+%d/s", w.currSpawns)),
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("-%d/s", w.currDeaths)),
		lipgloss.NewStyle().Foreground(netColor).Render(fmt.Sprintf("net:%s%d", netSign, w.currNet)),
	)

	// Chart area
	chartRows := w.height - 3 // header + legend + padding
	if chartRows < 1 {
		chartRows = 1
	}

	// Chart width in braille characters (each = 2 data points)
	chartWidth := w.width - 2 // margins
	if chartWidth < 1 {
		chartWidth = 1
	}
	capacity := chartWidth * 2

	// Get visible window of data
	spawnData := w.spawnHistory
	deathData := w.deathHistory
	if len(spawnData) > capacity {
		spawnData = spawnData[len(spawnData)-capacity:]
		deathData = deathData[len(deathData)-capacity:]
	}

	// Find peak for auto-scaling
	peak := 1
	for _, v := range spawnData {
		if v > peak {
			peak = v
		}
	}
	for _, v := range deathData {
		if v > peak {
			peak = v
		}
	}

	totalDots := chartRows * 4

	// Braille bit positions
	leftBits := [4]int{1, 2, 4, 64}
	rightBits := [4]int{8, 16, 32, 128}

	// Right-align data
	paddedSpawn := make([]int, capacity)
	paddedDeath := make([]int, capacity)
	offset := capacity - len(spawnData)
	for i, v := range spawnData {
		paddedSpawn[offset+i] = v
	}
	for i, v := range deathData {
		paddedDeath[offset+i] = v
	}

	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	dim := "\033[2m"
	reset := "\033[0m"

	var chartLines []string
	for r := 0; r < chartRows; r++ {
		var row strings.Builder
		row.WriteString(" ")
		for c := 0; c < chartWidth; c++ {
			sL := paddedSpawn[c*2]
			sR := 0
			if c*2+1 < capacity {
				sR = paddedSpawn[c*2+1]
			}
			dL := paddedDeath[c*2]
			dR := 0
			if c*2+1 < capacity {
				dR = paddedDeath[c*2+1]
			}

			// Map to dot heights (filled area)
			sDotsL := sL * totalDots / peak
			sDotsR := sR * totalDots / peak
			dDotsL := dL * totalDots / peak
			dDotsR := dR * totalDots / peak

			base := (chartRows - 1 - r) * 4
			bits := 0
			hasSpawn := false
			hasDeath := false

			for dot := 0; dot < 4; dot++ {
				dotRow := base + dot
				if dotRow < sDotsL {
					bits |= leftBits[dot]
					hasSpawn = true
				}
				if dotRow < dDotsL {
					bits |= leftBits[dot]
					hasDeath = true
				}
				if dotRow < sDotsR {
					bits |= rightBits[dot]
					hasSpawn = true
				}
				if dotRow < dDotsR {
					bits |= rightBits[dot]
					hasDeath = true
				}
			}

			codepoint := 0x2800 + bits

			// Pick color
			var color string
			if bits == 0 {
				color = dim
				if r == chartRows-1 {
					codepoint = 0x2802 // subtle canvas marker
				}
			} else if hasSpawn && hasDeath {
				color = yellow
			} else if hasSpawn {
				color = green
			} else {
				color = red
			}

			row.WriteString(color)
			row.WriteString(string(rune(codepoint)))
			row.WriteString(reset)
		}
		chartLines = append(chartLines, row.String())
	}

	// Legend
	legend := fmt.Sprintf("  %s●%s spawns  %s●%s deaths", green, reset, red, reset)

	parts := []string{header}
	parts = append(parts, chartLines...)
	parts = append(parts, legend)

	// Pad remaining height
	for len(parts) < w.height {
		parts = append(parts, "")
	}

	return strings.Join(parts, "\n")
}
