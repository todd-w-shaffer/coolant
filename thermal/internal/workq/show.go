package workq

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
)

// queueJSON is the `thermo queue --json` document, the contract the run
// board and status lines read.
type queueJSON struct {
	Mode   string `json:"mode"`
	Budget Budget `json:"budget"`
	HorizonView
}

// recordsTail bounds how much of runs.jsonl an estimate reads.
const recordsTail = 4 << 20

// ShowQueue is `thermo queue [--json]`: running jobs, waiting jobs in
// service order, and each one's estimate from past records.
func ShowQueue(s Settings, args []string, now time.Time) int {
	if len(args) > 0 && args[0] == "weights" {
		return ShowWeights(s, args[1:])
	}
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		default:
			fmt.Fprintf(s.Stderr, "coolant: unexpected argument %q\nusage: coolant queue [--json]\n", a)
			return ExitUsage
		}
	}
	q, err := Open(s.QueueDir)
	if err != nil {
		fmt.Fprintf(s.Stderr, "coolant: queue at %s unusable (%v)\n", s.QueueDir, err)
		return 1
	}
	defer q.Close()
	running, waiting, err := q.Snapshot()
	if err != nil {
		fmt.Fprintf(s.Stderr, "coolant: reading queue: %v\n", err)
		return 1
	}
	// Estimates need past records; an idle queue has nothing to estimate,
	// so a poller of the idle queue never pays for reading them.
	var recs []Record
	if len(running)+len(waiting) > 0 {
		recs = ReadRecent(s.RecordsPath, recordsTail)
	}
	est := func(t Ticket) (float64, bool) { return Estimate(recs, t.Class, t.Repo) }
	v := Horizon(running, waiting, s.Budget, est, now)

	if asJSON {
		doc := queueJSON{Mode: s.Mode, Budget: s.Budget, HorizonView: v}
		enc := json.NewEncoder(s.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return 1
		}
		return 0
	}

	header := fmt.Sprintf("coolant queue · %s · budget %d cores, %s", s.Mode, s.Budget.Cores, format.FormatBytes(int64(s.Budget.MemMB)<<20))
	if len(v.Running) == 0 && len(v.Waiting) == 0 {
		fmt.Fprintln(s.Stdout, header+" · idle")
		return 0
	}
	fmt.Fprintln(s.Stdout, header)
	tw := tabwriter.NewWriter(s.Stdout, 0, 0, 2, ' ', 0)
	if len(v.Running) > 0 {
		fmt.Fprintln(tw, "running")
		for _, e := range v.Running {
			when := seconds(e.ElapsedS)
			if e.Known {
				when += " of ~" + seconds(e.EstimateS)
			}
			row(tw, e, when)
		}
	}
	if len(v.Waiting) > 0 {
		fmt.Fprintln(tw, "waiting")
		for _, e := range v.Waiting {
			when := "start unknown"
			if e.Known {
				when = "starts in ~" + seconds(e.StartInS)
			}
			if e.EstimateS > 0 {
				when += ", runs ~" + seconds(e.EstimateS)
			}
			row(tw, e, when)
		}
	}
	_ = tw.Flush()
	return 0
}

func row(tw *tabwriter.Writer, e HorizonEntry, when string) {
	fmt.Fprintf(tw, "  #%d\tpid %d\t%s\t%s\t%s\t%s\t%s\n", e.Seq, e.PID, LaneLabel(e.Lane), e.Class, TildePath(e.Cwd), when, e.Cmd)
}

// seconds renders a duration in the shared stats vocabulary.
func seconds(sec float64) string {
	return format.FormatDuration(int64(sec + 0.5))
}
