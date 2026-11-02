package workq

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// HorizonEntry is one job on the horizon. Times are seconds; an
// estimate of a job's length is Known only when past records exist,
// and a start is Known only when every job it waits behind is.
type HorizonEntry struct {
	Ticket
	ElapsedS   float64 `json:"elapsed_s,omitempty"`
	EstimateS  float64 `json:"estimate_s,omitempty"`
	RemainingS float64 `json:"remaining_s,omitempty"`
	StartInS   float64 `json:"start_in_s,omitempty"`
	Known      bool    `json:"known"`
}

// HorizonView is the whole queue as `thermo queue` shows it.
type HorizonView struct {
	Running []HorizonEntry `json:"running"`
	Waiting []HorizonEntry `json:"waiting"`
}

// Horizon simulates the waiting line against the running jobs'
// remaining estimates under budget b: each waiter, in order, starts
// once enough earlier jobs have finished for it to be admitted.
func Horizon(running, waiting []Ticket, b Budget, est func(Ticket) (float64, bool), now time.Time) HorizonView {
	type active struct {
		t     Ticket
		fin   float64
		known bool
	}
	v := HorizonView{Running: []HorizonEntry{}, Waiting: []HorizonEntry{}}
	var act []active
	for _, r := range running {
		e := HorizonEntry{Ticket: r}
		if st, err := time.Parse(time.RFC3339, r.StartedAt); err == nil {
			e.ElapsedS = math.Max(0, math.Round(now.Sub(st).Seconds()))
		}
		e.EstimateS, e.Known = est(r)
		if e.Known {
			e.RemainingS = math.Max(0, e.EstimateS-e.ElapsedS)
		}
		v.Running = append(v.Running, e)
		act = append(act, active{r, e.RemainingS, e.Known})
	}
	clock, known := 0.0, true
	for _, w := range waiting {
		for {
			ts := make([]Ticket, len(act))
			for i, a := range act {
				ts[i] = a.t
			}
			if len(Admit(ts, []Ticket{w}, b)) == 1 {
				break
			}
			first := 0
			for i, a := range act {
				if a.fin < act[first].fin {
					first = i
				}
			}
			clock = math.Max(clock, act[first].fin)
			known = known && act[first].known
			act = append(act[:first], act[first+1:]...)
		}
		e := HorizonEntry{Ticket: w, StartInS: clock, Known: known}
		var ok bool
		e.EstimateS, ok = est(w)
		v.Waiting = append(v.Waiting, e)
		act = append(act, active{w, clock + e.EstimateS, known && ok})
	}
	return v
}

// waitLineMax is how many jobs a wait line names before "+N more".
const waitLineMax = 3

// WaitLine is the one line a waiting job prints, naming what is ahead
// of it: pid, lane, class, working directory and state of each.
func WaitLine(ahead []Ticket) string {
	parts := make([]string, 0, waitLineMax+1)
	for i, t := range ahead {
		if i == waitLineMax {
			parts = append(parts, fmt.Sprintf("+%d more", len(ahead)-waitLineMax))
			break
		}
		parts = append(parts, fmt.Sprintf("pid %d (%s, %s) in %s [%s]", t.PID, LaneLabel(t.Lane), t.Class, TildePath(t.Cwd), t.State))
	}
	return fmt.Sprintf("coolant: waiting, %d ahead: %s", len(ahead), strings.Join(parts, "; "))
}

// LaneLabel names a caller's lane; an unmarked caller is interactive.
func LaneLabel(lane string) string {
	if lane == "" {
		return "interactive"
	}
	return lane
}

// TildePath shortens a path under $HOME to ~/….
func TildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}
