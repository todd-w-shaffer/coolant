package workq

import (
	"reflect"
	"testing"
)

func tk(seq int, prio int, class string, cores, mem int, whole bool, state string) Ticket {
	return Ticket{Seq: seq, Priority: prio, Class: class, Cores: cores, MemMB: mem, Whole: whole, State: state}
}

func seqs(ts []Ticket) []int {
	out := []int{}
	for _, t := range ts {
		out = append(out, t.Seq)
	}
	return out
}

var budget8 = Budget{Cores: 8, MemMB: 8192}

func TestOrderWaitingInteractiveJumpsAhead(t *testing.T) {
	in := []Ticket{
		tk(1, PriorityLane, "suite", 8, 3000, true, StateWaiting),
		tk(2, PriorityLane, "typecheck", 1, 1000, false, StateWaiting),
		tk(3, PriorityInteractive, "typecheck", 1, 1000, false, StateWaiting),
		tk(4, PriorityInteractive, "files", 2, 1000, false, StateWaiting),
	}
	got := seqs(OrderWaiting(in))
	want := []int{3, 4, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestOrderWaitingDropsRunning(t *testing.T) {
	in := []Ticket{
		tk(1, PriorityLane, "suite", 8, 3000, true, StateRunning),
		tk(2, PriorityLane, "typecheck", 1, 1000, false, StateWaiting),
	}
	got := seqs(OrderWaiting(in))
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("order = %v, want [2]", got)
	}
}

func TestAdmitFirstComeFirstServedWithinPriority(t *testing.T) {
	waiting := []Ticket{
		tk(5, PriorityLane, "typecheck", 1, 1000, false, StateWaiting),
		tk(6, PriorityLane, "typecheck", 1, 1000, false, StateWaiting),
	}
	got := seqs(Admit(nil, waiting, budget8))
	if !reflect.DeepEqual(got, []int{5, 6}) {
		t.Fatalf("admitted = %v, want [5 6]", got)
	}
}

func TestAdmitHoldsWhenBudgetFull(t *testing.T) {
	running := []Ticket{tk(1, PriorityLane, "files", 7, 1000, false, StateRunning)}
	waiting := []Ticket{tk(2, PriorityLane, "files", 2, 1000, false, StateWaiting)}
	if got := Admit(running, waiting, budget8); len(got) != 0 {
		t.Fatalf("admitted %v, want none (7+2 > 8 cores)", seqs(got))
	}
}

func TestAdmitAdmitsWhileFits(t *testing.T) {
	running := []Ticket{tk(1, PriorityLane, "files", 6, 1000, false, StateRunning)}
	waiting := []Ticket{tk(2, PriorityLane, "files", 2, 1000, false, StateWaiting)}
	got := seqs(Admit(running, waiting, budget8))
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("admitted = %v, want [2]", got)
	}
}

func TestAdmitHoldsOnMemoryCeiling(t *testing.T) {
	running := []Ticket{tk(1, PriorityLane, "lint", 1, 7000, false, StateRunning)}
	waiting := []Ticket{tk(2, PriorityLane, "typecheck", 1, 1500, false, StateWaiting)}
	if got := Admit(running, waiting, budget8); len(got) != 0 {
		t.Fatalf("admitted %v, want none (7000+1500 > 8192 MB)", seqs(got))
	}
}

func TestAdmitWholeClassWaitsForEmptyMachine(t *testing.T) {
	running := []Ticket{tk(1, PriorityLane, "build", 1, 500, false, StateRunning)}
	waiting := []Ticket{tk(2, PriorityLane, "suite", 2, 3000, true, StateWaiting)}
	if got := Admit(running, waiting, budget8); len(got) != 0 {
		t.Fatalf("admitted %v, want none (whole class needs the whole budget)", seqs(got))
	}
	if got := seqs(Admit(nil, waiting, budget8)); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("admitted = %v on empty machine, want [2]", got)
	}
}

func TestAdmitWholeClassBlocksEverythingBehindIt(t *testing.T) {
	waiting := []Ticket{
		tk(2, PriorityLane, "suite", 8, 3000, true, StateWaiting),
		tk(3, PriorityLane, "build", 1, 500, false, StateWaiting),
	}
	got := seqs(Admit(nil, waiting, budget8))
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("admitted = %v, want [2] only", got)
	}
}

func TestAdmitStrictHeadOfLine(t *testing.T) {
	// A small job behind a head that does not fit waits too, so the head
	// (often a whole suite) is never starved by a stream of small jobs.
	running := []Ticket{tk(1, PriorityLane, "files", 4, 1000, false, StateRunning)}
	waiting := []Ticket{
		tk(2, PriorityLane, "suite", 8, 3000, true, StateWaiting),
		tk(3, PriorityLane, "build", 1, 500, false, StateWaiting),
	}
	if got := Admit(running, waiting, budget8); len(got) != 0 {
		t.Fatalf("admitted %v, want none (head-of-line)", seqs(got))
	}
}

func TestAdmitOversizedJobRunsAloneOnEmptyMachine(t *testing.T) {
	waiting := []Ticket{tk(2, PriorityLane, "custom", 12, 20000, false, StateWaiting)}
	got := seqs(Admit(nil, waiting, budget8))
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("admitted = %v, want [2]", got)
	}
}

func TestAdmitInteractiveGoesFirstButNeverPreempts(t *testing.T) {
	running := []Ticket{tk(1, PriorityLane, "suite", 8, 3000, true, StateRunning)}
	waiting := OrderWaiting([]Ticket{
		tk(2, PriorityLane, "typecheck", 1, 1000, false, StateWaiting),
		tk(3, PriorityInteractive, "typecheck", 1, 1000, false, StateWaiting),
	})
	if got := Admit(running, waiting, budget8); len(got) != 0 {
		t.Fatalf("admitted %v while a whole suite runs, want none", seqs(got))
	}
	got := seqs(Admit(nil, waiting, budget8))
	if !reflect.DeepEqual(got, []int{3, 2}) {
		t.Fatalf("admitted = %v, want [3 2]", got)
	}
}

func TestGrantCores(t *testing.T) {
	cases := []struct {
		t    Ticket
		want int
	}{
		{tk(1, 0, "suite", 2, 0, true, ""), 8},
		{tk(1, 0, "files", 2, 0, false, ""), 2},
		{tk(1, 0, "custom", 20, 0, false, ""), 8},
		{tk(1, 0, "custom", 0, 0, false, ""), 1},
	}
	for _, c := range cases {
		if got := c.t.Need(budget8).Cores; got != c.want {
			t.Errorf("%s cores=%d whole=%v: need %d, want %d", c.t.Class, c.t.Cores, c.t.Whole, got, c.want)
		}
	}
}
