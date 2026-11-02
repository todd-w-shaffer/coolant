// Package workq is coolant's machine-wide work queue: the door every
// CPU- or memory-heavy job on the machine goes through (`thermo run`),
// and the horizon view of it (`thermo queue`).
//
// The queue is a directory of ticket files under ~/.coolant/queue/.
// Every read-modify-write happens under flock on queue.lock; each holder
// keeps flock on its own ticket for its whole life, so a ticket whose
// lock is free belongs to a dead holder and is removed by whoever sees
// it next. No wait has a clock: a wait ends when the work ahead ends.
package workq

import "sort"

// Priorities: lower is served first. An unmarked caller is interactive;
// a launcher marks its sessions with COOLANT_LANE.
const (
	PriorityInteractive = 0
	PriorityLane        = 1
)

// Ticket states.
const (
	StateWaiting = "waiting"
	StateRunning = "running"
)

// Budget is cores and memory: the enforce-mode admission limit for the
// sum of running jobs, and what one admitted job draws from it.
type Budget struct {
	Cores int `json:"cores"`
	MemMB int `json:"memory_mb"`
}

// Need answers the cores and memory this ticket takes from budget b.
// A whole class takes the whole core budget; any other class takes its
// declared cores, at least 1 and at most the budget.
func (t Ticket) Need(b Budget) Budget {
	c := t.Cores
	if t.Whole || c > b.Cores {
		c = b.Cores
	}
	if c < 1 {
		c = 1
	}
	return Budget{Cores: c, MemMB: t.MemMB}
}

// OrderWaiting answers the waiting tickets in service order: by
// priority (interactive first), then first come, first served.
func OrderWaiting(ts []Ticket) []Ticket {
	out := make([]Ticket, 0, len(ts))
	for _, t := range ts {
		if t.State == StateWaiting {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// Admit answers the prefix of waiting (already in service order) that
// may start now alongside running. It admits the head while the running
// total plus the head's need fits the budget, or when nothing is running
// at all — so a job bigger than the budget runs alone rather than never.
// It stops at the first job that does not fit: strict head-of-line, so a
// whole-suite job is not starved by a stream of small ones.
func Admit(running, waiting []Ticket, b Budget) []Ticket {
	var usedC, usedM int
	for _, r := range running {
		need := r.Need(b)
		usedC += need.Cores
		usedM += need.MemMB
	}
	var out []Ticket
	for _, w := range waiting {
		need := w.Need(b)
		busy := len(running)+len(out) > 0
		if busy && (usedC+need.Cores > b.Cores || usedM+need.MemMB > b.MemMB) {
			break
		}
		out = append(out, w)
		usedC += need.Cores
		usedM += need.MemMB
	}
	return out
}
