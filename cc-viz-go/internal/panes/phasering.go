package panes

import (
	"fmt"
	"strings"

	"github.com/toddwshaffer/coolant/cc-viz-go/internal/ui"
)

// Phase represents a system state classification.
type Phase int

const (
	PhaseCalm Phase = iota
	PhaseCooling
	PhaseRamping
	PhaseExploding
)

func (p Phase) String() string {
	switch p {
	case PhaseCalm:
		return "CALM"
	case PhaseCooling:
		return "COOLING"
	case PhaseRamping:
		return "RAMPING"
	case PhaseExploding:
		return "EXPLODING"
	}
	return "UNKNOWN"
}

func (p Phase) Color() string {
	switch p {
	case PhaseCalm:
		return "\033[32m" // green
	case PhaseCooling:
		return "\033[36m" // cyan
	case PhaseRamping:
		return "\033[33m" // yellow
	case PhaseExploding:
		return "\033[31m" // red
	}
	return "\033[37m"
}

// ClassifyPhase determines the system phase from current metrics.
func ClassifyPhase(total, spawns, net int) Phase {
	// Priority: EXPLODING > RAMPING > COOLING > CALM
	if spawns >= ui.SpawnCrit || net >= ui.NetCrit || total >= ui.TotalCrit {
		return PhaseExploding
	}
	if spawns >= ui.SpawnWarn || net >= ui.NetWarn {
		return PhaseRamping
	}
	if total > ui.TotalWarn && net < 0 {
		return PhaseCooling
	}
	return PhaseCalm
}

// PhaseRing tracks phase history and renders the dot ring.
type PhaseRing struct {
	ring     []Phase
	ringSize int
}

func NewPhaseRing(size int) *PhaseRing {
	return &PhaseRing{
		ringSize: size,
	}
}

func (pr *PhaseRing) Push(p Phase) {
	pr.ring = append(pr.ring, p)
	if len(pr.ring) > pr.ringSize {
		pr.ring = pr.ring[len(pr.ring)-pr.ringSize:]
	}
}

func (pr *PhaseRing) Current() Phase {
	if len(pr.ring) == 0 {
		return PhaseCalm
	}
	return pr.ring[len(pr.ring)-1]
}

// View renders the phase ring as a single line: ● ● ● ● ● LABEL
func (pr *PhaseRing) View() string {
	reset := "\033[0m"
	dim := "\033[2m"
	dot := "●"

	var buf strings.Builder

	for i, p := range pr.ring {
		if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(p.Color())
		buf.WriteString(dot)
		buf.WriteString(reset)
	}

	// Pad with dim dots
	for i := len(pr.ring); i < pr.ringSize; i++ {
		if i > 0 || len(pr.ring) > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(dim)
		buf.WriteString(dot)
		buf.WriteString(reset)
	}

	current := pr.Current()
	buf.WriteString("  ")
	buf.WriteString(fmt.Sprintf("\033[1m%s%s%s", current.Color(), current.String(), reset))

	return buf.String()
}
