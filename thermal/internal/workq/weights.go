package workq

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
)

// `thermo queue weights` reads runs.jsonl and answers, per class, what
// the jobs actually cost, the weight config gives them, and a weight
// suggested from the records. It is read-only: it never writes config,
// the records or the queue. Choosing weights, and choosing to enforce
// them, stay the user's edits to config.toml.

// Suggestion rule. A class's suggested weight is its p90 cores used
// (cpu_s / wall_s over jobs longer than minMeasuredWallS), rounded to a
// whole core and kept within 1..the core budget, and its p90 peak
// memory rounded up to memRoundMB. It is never whole. A class with
// fewer than minMeasuredJobs jobs over minMeasuredWallS gets none.
const (
	minMeasuredWallS = 1.0
	minMeasuredJobs  = 5
	memRoundMB       = 256
	// WeightsRule states the rule in the --json document and the footer.
	WeightsRule = "cores = p90 of cpu_s/wall_s over jobs longer than 1 s, rounded, within 1..budget; " +
		"memory_mb = p90 peak memory rounded up to 256 MB; never whole; " +
		"no suggestion with fewer than 5 jobs longer than 1 s"
)

// Spread is a sample's percentiles (nearest rank). N is the sample size
// where it differs from the class's job count.
type Spread struct {
	N   int     `json:"n,omitempty"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	Max float64 `json:"max"`
}

// Weight is a class weight as config states it or as suggested.
type Weight struct {
	Cores    int  `json:"cores"`
	MemoryMB int  `json:"memory_mb"`
	Whole    bool `json:"whole"`
	// Unknown: the class is not configured; the door weighs it as the
	// unknown-class weight.
	Unknown bool `json:"unknown,omitempty"`
}

// WaitView is how often a class's jobs would have waited, and for how
// long. A wait lies between when the first and when the last of the
// jobs running at its start ended: FirstEnd and LastEnd, in seconds
// after the start. Both read the observed timeline, where nothing
// waited, so they are rough bounds, not a simulation. Unbounded counts
// recorded waits with no overlapping record to bound them (the job ahead
// left no record, e.g. it was killed); they count in Jobs and Share.
type WaitView struct {
	Jobs       int     `json:"jobs"`
	Share      float64 `json:"share"`
	OwnWallP50 float64 `json:"own_wall_p50_s"`
	FirstEnd   Spread  `json:"first_end_s"`
	LastEnd    Spread  `json:"last_end_s"`
	Unbounded  int     `json:"unbounded,omitempty"`
}

// WeightRow is one class.
type WeightRow struct {
	Class      string   `json:"class"`
	Jobs       int      `json:"jobs"`
	WallS      Spread   `json:"wall_s"`
	CoresUsed  Spread   `json:"cores_used"`
	PeakMemMB  Spread   `json:"peak_mem_mb"`
	WouldWait  WaitView `json:"would_wait"`
	Configured Weight   `json:"configured"`
	Suggested  *Weight  `json:"suggested"`
	// The same jobs replayed against the same overlaps, under the
	// configured weights and under the suggested ones (configured where
	// there is no suggestion). The configured replay is how a candidate
	// config is tried against history before it is used.
	ConfiguredWouldWait WaitView `json:"configured_would_wait"`
	SuggestedWouldWait  WaitView `json:"suggested_would_wait"`
}

// WeightsReport is the `thermo queue weights --json` document.
type WeightsReport struct {
	Budget  Budget `json:"budget"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Records int    `json:"records"`
	// Jobs are the top-level observe-mode records the figures come from.
	// Nested records are inside their parent's figures; degraded ones held
	// no ticket; enforce ones ran at their grant, which would feed the
	// weight back into its own measurement.
	Jobs     int         `json:"jobs"`
	Nested   int         `json:"nested"`
	Enforce  int         `json:"enforce"`
	Degraded int         `json:"degraded"`
	Rule     string      `json:"rule"`
	Classes  []WeightRow `json:"classes"`
}

// timed is a record with its interval parsed.
type timed struct {
	Record
	start, end time.Time
}

func timeRecord(r Record) (timed, bool) {
	st, err1 := time.Parse(time.RFC3339, r.TS)
	en, err2 := time.Parse(time.RFC3339, r.End)
	return timed{r, st, en}, err1 == nil && err2 == nil
}

// before reports whether h took its ticket before j: an earlier start,
// or the same second and a lower ticket number.
func (h timed) before(j timed) bool {
	return h.start.Before(j.start) || (h.start.Equal(j.start) && h.Seq < j.Seq)
}

// runningAt answers the ticket holders still running when j started.
func runningAt(j timed, holders []timed) []timed {
	var out []timed
	for _, h := range holders {
		if h.before(j) && h.end.After(j.start) {
			out = append(out, h)
		}
	}
	return out
}

func weightTicket(class string, w map[string]Class) Ticket {
	c, ok := w[class]
	if !ok {
		c = unknownClass
	}
	return Ticket{Class: class, Cores: c.Cores, MemMB: c.MemoryMB, Whole: c.Whole}
}

// waitsTimed reports whether enforce would have held j under weights w,
// given the jobs running at its start. It treats j as the head of the line.
func waitsTimed(j timed, running []timed, w map[string]Class, b Budget) bool {
	ts := make([]Ticket, len(running))
	for i, r := range running {
		ts[i] = weightTicket(r.Class, w)
	}
	return len(Admit(ts, []Ticket{weightTicket(j.Class, w)}, b)) == 0
}

// percentile answers the nearest-rank p-quantile of xs (0 when empty).
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(math.Ceil(p*float64(len(s)))) - 1
	return s[max(0, min(i, len(s)-1))]
}

func spread(xs []float64) Spread {
	if len(xs) == 0 {
		return Spread{}
	}
	return Spread{P50: percentile(xs, 0.5), P90: percentile(xs, 0.9), Max: percentile(xs, 1)}
}

// suggest applies the rule to one class's figures.
func suggest(cores Spread, peak Spread, jobs int, b Budget) *Weight {
	if cores.N < minMeasuredJobs || jobs < minMeasuredJobs {
		return nil
	}
	c := int(math.Round(cores.P90))
	c = max(1, min(c, b.Cores))
	m := int(math.Ceil(peak.P90/memRoundMB)) * memRoundMB
	return &Weight{Cores: c, MemoryMB: max(m, memRoundMB)}
}

// waitView summarizes the jobs of one class that wait, by whichever
// test decides it.
func waitView(jobs []timed, holders []timed, wait func(j timed, running []timed) bool) WaitView {
	var v WaitView
	var own, first, last []float64
	for _, j := range jobs {
		running := runningAt(j, holders)
		if !wait(j, running) {
			continue
		}
		v.Jobs++
		own = append(own, j.WallS)
		if len(running) == 0 {
			v.Unbounded++
			continue
		}
		lo, hi := math.Inf(1), 0.0
		for _, r := range running {
			left := r.end.Sub(j.start).Seconds()
			lo, hi = math.Min(lo, left), math.Max(hi, left)
		}
		first = append(first, lo)
		last = append(last, hi)
	}
	if len(jobs) > 0 {
		v.Share = float64(v.Jobs) / float64(len(jobs))
	}
	v.OwnWallP50 = percentile(own, 0.5)
	v.FirstEnd, v.LastEnd = spread(first), spread(last)
	return v
}

// Weigh builds the report from recs under the configured classes and
// budget b.
func Weigh(recs []Record, classes map[string]Class, b Budget) WeightsReport {
	rep := WeightsReport{Budget: b, Records: len(recs), Rule: WeightsRule, Classes: []WeightRow{}}
	var holders []timed
	byClass := map[string][]timed{}
	var from, to time.Time
	for _, r := range recs {
		t, ok := timeRecord(r)
		if !ok {
			continue
		}
		if from.IsZero() || t.start.Before(from) {
			from = t.start
		}
		if t.end.After(to) {
			to = t.end
		}
		switch {
		case r.Nested:
			rep.Nested++
			continue
		case r.Degraded:
			rep.Degraded++
			continue
		}
		holders = append(holders, t)
		if r.Mode == ModeEnforce {
			rep.Enforce++
			continue
		}
		rep.Jobs++
		byClass[r.Class] = append(byClass[r.Class], t)
	}
	if !from.IsZero() {
		rep.From, rep.To = from.Format(time.RFC3339), to.Format(time.RFC3339)
	}

	names := make([]string, 0, len(byClass)+len(classes))
	for n := range byClass {
		names = append(names, n)
	}
	for n := range classes {
		if _, ok := byClass[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	suggested := map[string]Class{}
	for n, c := range classes {
		suggested[n] = c
	}
	for _, n := range names {
		jobs := byClass[n]
		row := WeightRow{Class: n, Jobs: len(jobs)}
		var walls, cores, peaks []float64
		for _, j := range jobs {
			walls = append(walls, j.WallS)
			peaks = append(peaks, j.PeakMemMB)
			if j.WallS > minMeasuredWallS {
				cores = append(cores, round2(j.CPUS/j.WallS))
			}
		}
		row.WallS, row.PeakMemMB = spread(walls), spread(peaks)
		row.CoresUsed = spread(cores)
		row.CoresUsed.N = len(cores)
		if c, ok := classes[n]; ok {
			row.Configured = Weight{Cores: c.Cores, MemoryMB: c.MemoryMB, Whole: c.Whole}
		} else {
			row.Configured = Weight{Cores: unknownClass.Cores, MemoryMB: unknownClass.MemoryMB, Unknown: true}
		}
		row.WouldWait = waitView(jobs, holders, func(j timed, _ []timed) bool { return j.WouldWait })
		if row.Suggested = suggest(row.CoresUsed, row.PeakMemMB, len(jobs), b); row.Suggested != nil {
			suggested[n] = Class{Cores: row.Suggested.Cores, MemoryMB: row.Suggested.MemoryMB}
		}
		rep.Classes = append(rep.Classes, row)
	}
	replay := func(w map[string]Class) func(timed, []timed) bool {
		return func(j timed, running []timed) bool { return waitsTimed(j, running, w, b) }
	}
	for i := range rep.Classes {
		jobs := byClass[rep.Classes[i].Class]
		rep.Classes[i].ConfiguredWouldWait = waitView(jobs, holders, replay(classes))
		rep.Classes[i].SuggestedWouldWait = waitView(jobs, holders, replay(suggested))
	}
	return rep
}

const weightsUsage = "usage: coolant queue weights [--json | --toml] [--since <RFC3339>] [--until <RFC3339>]"

// ShowWeights is `thermo queue weights`.
func ShowWeights(s Settings, args []string) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(s.Stderr, "coolant: "+format+"\n%s\n", append(a, weightsUsage)...)
		return ExitUsage
	}
	var asJSON, asTOML bool
	var since, until time.Time
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--json":
			asJSON = true
		case "--toml":
			asTOML = true
		case "--since", "--until":
			if i+1 >= len(args) {
				return usage("%s needs a time", a)
			}
			i++
			t, err := time.Parse(time.RFC3339, args[i])
			if err != nil {
				return usage("%s %q is not an RFC3339 time", a, args[i])
			}
			if a == "--since" {
				since = t
			} else {
				until = t
			}
		default:
			return usage("unexpected argument %q", a)
		}
	}
	if asJSON && asTOML {
		return usage("--json and --toml are exclusive")
	}

	var recs []Record
	for _, r := range ReadRecent(s.RecordsPath, math.MaxInt64) {
		st, err := time.Parse(time.RFC3339, r.TS)
		if err != nil || (!since.IsZero() && st.Before(since)) || (!until.IsZero() && !st.Before(until)) {
			continue
		}
		recs = append(recs, r)
	}
	rep := Weigh(recs, s.Classes, s.Budget)

	switch {
	case asJSON:
		enc := json.NewEncoder(s.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return 1
		}
	case asTOML:
		writeWeightsTOML(s.Stdout, rep)
	default:
		writeWeightsTable(s.Stdout, rep)
	}
	return 0
}

// writeWeightsTOML prints the suggested classes as config tables, and
// nothing else: the queue's mode and budget are not this command's.
func writeWeightsTOML(w io.Writer, rep WeightsReport) {
	fmt.Fprintf(w, "# Suggested by `coolant queue weights` from %d jobs, %s to %s.\n", rep.Jobs, rep.From, rep.To)
	fmt.Fprintf(w, "# Rule: %s.\n", rep.Rule)
	fmt.Fprintln(w, "# Weights only. Holding jobs to them (enforce) is a separate edit to [queue].")
	for _, r := range rep.Classes {
		if r.Suggested == nil {
			continue
		}
		fmt.Fprintf(w, "\n[queue.classes.%s]\ncores = %d\nmemory_mb = %d\nwhole = false\n", r.Class, r.Suggested.Cores, r.Suggested.MemoryMB)
	}
}

func writeWeightsTable(w io.Writer, rep WeightsReport) {
	window := "no records"
	if rep.From != "" {
		window = shortTime(rep.From) + " – " + shortTime(rep.To)
	}
	fmt.Fprintf(w, "coolant queue weights · %s · %d jobs (left out: %d nested, %d enforce, %d degraded) · budget %d cores, %s\n\n",
		window, rep.Jobs, rep.Nested, rep.Enforce, rep.Degraded, rep.Budget.Cores, format.FormatBytes(int64(rep.Budget.MemMB)<<20))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "class\tjobs\twall p50\tp90\tcores used p50\tp90\tpeak MB p50\tp90\tmax\twould wait\tconfigured\tsuggested")
	for _, r := range rep.Classes {
		cores := "–\t–"
		if r.CoresUsed.N > 0 {
			cores = fmt.Sprintf("%.1f\t%.1f", r.CoresUsed.P50, r.CoresUsed.P90)
		}
		sugg := "too few jobs over 1 s"
		if r.Suggested != nil {
			sugg = weightLabel(*r.Suggested)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%.0f\t%.0f\t%.0f\t%s\t%s\t%s\n",
			r.Class, r.Jobs, seconds(r.WallS.P50), seconds(r.WallS.P90), cores,
			r.PeakMemMB.P50, r.PeakMemMB.P90, r.PeakMemMB.Max, percent(r.WouldWait.Share),
			weightLabel(r.Configured), sugg)
	}
	_ = tw.Flush()

	waitTable(w, rep, "\nwould wait, as recorded under the weights configured at the time", func(r WeightRow) WaitView { return r.WouldWait })
	waitTable(w, rep, "\nwould wait, replayed against the same overlaps under the configured weights", func(r WeightRow) WaitView { return r.ConfiguredWouldWait })
	waitTable(w, rep, "\nwould wait, replayed against the same overlaps under the suggested weights", func(r WeightRow) WaitView { return r.SuggestedWouldWait })
	fmt.Fprintln(w, "\nA wait lies between when the first and the last job running at its start ended (rough: the observed timeline, nothing actually held).")
	fmt.Fprintln(w, "Try other weights against this history with COOLANT_CONFIG=<candidate.toml>: the configured replay uses them.")
	fmt.Fprintf(w, "Suggested weight: %s. Percentiles are nearest-rank.\n", rep.Rule)
	fmt.Fprintln(w, "Nothing is written: copy weights into ~/.config/coolant/config.toml (`--toml` prints them); enforce is a separate [queue] edit.")
}

func waitTable(w io.Writer, rep WeightsReport, title string, pick func(WeightRow) WaitView) {
	fmt.Fprintln(w, title)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  class\tjobs\tshare\town wall p50\twait ≥ p50\tmax\twait ≤ p50\tmax")
	shown := false
	for _, r := range rep.Classes {
		v := pick(r)
		if v.Jobs == 0 {
			continue
		}
		shown = true
		jobs := fmt.Sprintf("%d of %d", v.Jobs, r.Jobs)
		if v.Unbounded > 0 {
			jobs += fmt.Sprintf(" (%d unbounded)", v.Unbounded)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Class, jobs, percent(v.Share),
			seconds(v.OwnWallP50), seconds(v.FirstEnd.P50), seconds(v.FirstEnd.Max), seconds(v.LastEnd.P50), seconds(v.LastEnd.Max))
	}
	if !shown {
		fmt.Fprintln(tw, "  none")
	}
	_ = tw.Flush()
}

func weightLabel(c Weight) string {
	s := fmt.Sprintf("%d %s, %d MB", c.Cores, plural(c.Cores, "core", "cores"), c.MemoryMB)
	switch {
	case c.Whole:
		s = fmt.Sprintf("whole, %d MB", c.MemoryMB)
	case c.Unknown:
		s = "unknown: " + s
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func percent(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

// shortTime renders an RFC3339 UTC time as 2006-01-02 15:04Z.
func shortTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return strings.TrimSuffix(t.UTC().Format("2006-01-02 15:04Z07:00"), "+00:00")
}
