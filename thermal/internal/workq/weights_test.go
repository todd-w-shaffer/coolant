package workq

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

var weighT0 = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// wrec is an observe-mode top-level record of class c, ticket seq,
// starting off seconds after weighT0 and running wall seconds at
// cores×wall CPU seconds and mem MB peak.
func wrec(c string, seq int, off, wall, cores, mem float64) Record {
	st := weighT0.Add(time.Duration(off * float64(time.Second)))
	return Record{
		Schema: RecordSchema, Mode: ModeObserve, Class: c, Seq: seq,
		TS:    st.Format(time.RFC3339),
		End:   st.Add(time.Duration(wall * float64(time.Second))).Format(time.RFC3339),
		WallS: wall, CPUS: cores * wall, PeakMemMB: mem,
	}
}

func rowFor(t *testing.T, rep WeightsReport, class string) WeightRow {
	t.Helper()
	for _, r := range rep.Classes {
		if r.Class == class {
			return r
		}
	}
	t.Fatalf("no row for class %q in %+v", class, rep.Classes)
	return WeightRow{}
}

func TestPercentileIsNearestRank(t *testing.T) {
	xs := []float64{10, 1, 9, 2, 8, 3, 7, 4, 6, 5}
	if got := percentile(xs, 0.5); got != 5 {
		t.Errorf("p50 = %v, want 5", got)
	}
	if got := percentile(xs, 0.9); got != 9 {
		t.Errorf("p90 = %v, want 9", got)
	}
	if got := percentile([]float64{42}, 0.9); got != 42 {
		t.Errorf("p90 of one = %v, want 42", got)
	}
}

func TestWeighLeavesOutNestedEnforceAndDegraded(t *testing.T) {
	nested := wrec("files", 0, 0, 5, 1, 100)
	nested.Nested = true
	enforced := wrec("files", 3, 0, 5, 1, 100)
	enforced.Mode = ModeEnforce
	degraded := wrec("files", 0, 0, 5, 1, 100)
	degraded.Degraded = true
	recs := []Record{wrec("files", 1, 0, 5, 1, 100), wrec("files", 2, 100, 5, 1, 100), nested, enforced, degraded}

	rep := Weigh(recs, DefaultClassesForTest(), budget8)
	if rep.Records != 5 || rep.Jobs != 2 || rep.Nested != 1 || rep.Enforce != 1 || rep.Degraded != 1 {
		t.Errorf("counts = records %d jobs %d nested %d enforce %d degraded %d, want 5/2/1/1/1",
			rep.Records, rep.Jobs, rep.Nested, rep.Enforce, rep.Degraded)
	}
	if r := rowFor(t, rep, "files"); r.Jobs != 2 {
		t.Errorf("files jobs = %d, want 2", r.Jobs)
	}
}

func TestWeighMeasuresCoresOnlyOverOneSecond(t *testing.T) {
	recs := []Record{
		wrec("build", 1, 0, 0.5, 8, 100), // too short to measure: left out
		wrec("build", 2, 10, 4, 2, 100),
		wrec("build", 3, 20, 4, 3, 300),
	}
	r := rowFor(t, Weigh(recs, DefaultClassesForTest(), budget8), "build")
	if r.Jobs != 3 || r.CoresUsed.N != 2 {
		t.Fatalf("jobs %d, cores sample %d, want 3 and 2", r.Jobs, r.CoresUsed.N)
	}
	if r.CoresUsed.P50 != 2 || r.CoresUsed.P90 != 3 {
		t.Errorf("cores used = %+v, want p50 2 p90 3", r.CoresUsed)
	}
	if r.PeakMemMB.P50 != 100 || r.PeakMemMB.Max != 300 {
		t.Errorf("peak = %+v, want p50 100 max 300", r.PeakMemMB)
	}
}

func TestSuggestRule(t *testing.T) {
	cases := []struct {
		name      string
		cores     []float64
		mem       float64
		wantCores int
		wantMem   int
	}{
		{"rounds p90 down", []float64{1, 1, 1, 1, 1, 1, 1, 1, 2.4, 9}, 1766, 2, 1792},
		{"rounds p90 half up", []float64{1, 1, 1, 1, 1, 1, 1, 1, 2.5, 9}, 1792, 3, 1792},
		{"clamps to the budget", []float64{9, 9, 9, 9, 9}, 4000, 8, 4096},
		{"at least one core", []float64{0.1, 0.1, 0.1, 0.1, 0.1}, 10, 1, 256},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var recs []Record
			for i, cu := range c.cores {
				recs = append(recs, wrec("check", i+1, float64(i*1000), 10, cu, c.mem))
			}
			r := rowFor(t, Weigh(recs, DefaultClassesForTest(), budget8), "check")
			if r.Suggested == nil {
				t.Fatal("no suggestion")
			}
			if r.Suggested.Cores != c.wantCores || r.Suggested.MemoryMB != c.wantMem || r.Suggested.Whole {
				t.Errorf("suggested = %+v, want %d cores %d MB, not whole", *r.Suggested, c.wantCores, c.wantMem)
			}
		})
	}
}

func TestSuggestNeedsFiveMeasuredJobs(t *testing.T) {
	var recs []Record
	for i := range 4 {
		recs = append(recs, wrec("lint", i+1, float64(i*100), 20, 2, 3000))
	}
	for i := range 10 {
		recs = append(recs, wrec("lint", i+10, float64(1000+i*100), 0.3, 1, 3000))
	}
	r := rowFor(t, Weigh(recs, DefaultClassesForTest(), budget8), "lint")
	if r.Suggested != nil {
		t.Errorf("suggested %+v from 4 jobs over 1 s, want none", *r.Suggested)
	}
}

func TestWeighReportsConfiguredWeightAndUnknownClass(t *testing.T) {
	recs := []Record{wrec("suite", 1, 0, 10, 4, 3000), wrec("mystery", 2, 100, 10, 1, 100)}
	rep := Weigh(recs, DefaultClassesForTest(), budget8)
	if c := rowFor(t, rep, "suite").Configured; !c.Whole || c.MemoryMB != 3584 || c.Unknown {
		t.Errorf("suite configured = %+v", c)
	}
	if c := rowFor(t, rep, "mystery").Configured; !c.Unknown || c.Cores != unknownClass.Cores {
		t.Errorf("mystery configured = %+v, want the unknown-class weight", c)
	}
}

func TestWeighBoundsTheWaitOfWouldWaitJobs(t *testing.T) {
	check := wrec("suite", 1, 0, 300, 4, 3000)
	build := wrec("build", 2, 0, 100, 1, 100)
	files := wrec("files", 3, 50, 7, 1, 500)
	files.WouldWait = true
	later := wrec("files", 4, 400, 7, 1, 500) // alone: no wait

	r := rowFor(t, Weigh([]Record{check, build, files, later}, DefaultClassesForTest(), budget8), "files")
	ww := r.WouldWait
	if ww.Jobs != 1 || ww.Share != 0.5 {
		t.Fatalf("would wait = %+v, want 1 job, share 0.5", ww)
	}
	if ww.OwnWallP50 != 7 || ww.FirstEnd.P50 != 50 || ww.LastEnd.P50 != 250 || ww.LastEnd.Max != 250 {
		t.Errorf("bounds = %+v, want own 7, first end 50, last end 250", ww)
	}
}

func TestWeighReplaysSuggestedWeights(t *testing.T) {
	// Five suites at ~4 cores give suite a 4-core suggestion; a files job
	// that started beside one would have waited behind a whole suite but
	// fits beside a 4-core one.
	var recs []Record
	for i := range 5 {
		recs = append(recs, wrec("suite", i+1, float64(i*1000), 300, 4, 3000))
	}
	for i := range 5 {
		f := wrec("files", i+10, float64(i*1000+10), 7, 1, 500)
		f.WouldWait = true
		recs = append(recs, f)
	}
	rep := Weigh(recs, DefaultClassesForTest(), budget8)
	if s := rowFor(t, rep, "suite").Suggested; s == nil || s.Cores != 4 {
		t.Fatalf("suite suggested = %+v, want 4 cores", s)
	}
	f := rowFor(t, rep, "files")
	if f.WouldWait.Jobs != 5 {
		t.Errorf("recorded would wait = %d, want 5", f.WouldWait.Jobs)
	}
	if f.SuggestedWouldWait.Jobs != 0 {
		t.Errorf("would wait under suggested = %+v, want none", f.SuggestedWouldWait)
	}
}

func TestReplayBreaksSameSecondStartsBySeq(t *testing.T) {
	a := wrec("suite", 1, 0, 300, 4, 3000)
	b := wrec("suite", 2, 0, 300, 4, 3000)
	w := map[string]Class{"suite": {Whole: true, MemoryMB: 3000}}
	if waits(a, []Record{a, b}, w, budget8) {
		t.Error("seq 1 waits behind seq 2, which took its ticket after it")
	}
	if !waits(b, []Record{a, b}, w, budget8) {
		t.Error("seq 2 does not wait behind seq 1, which took its ticket first")
	}
}

// waits reports (test helper) whether enforce would have held j under weights w, given
// the jobs running at its start. It treats j as the head of the line.
func waits(j Record, holders []Record, w map[string]Class, b Budget) bool {
	tj, ok := timeRecord(j)
	if !ok {
		return false
	}
	var hs []timed
	for _, h := range holders {
		if th, ok := timeRecord(h); ok {
			hs = append(hs, th)
		}
	}
	return waitsTimed(tj, runningAt(tj, hs), w, b)
}

func writeRecords(t *testing.T, path string, recs []Record) {
	t.Helper()
	for _, r := range recs {
		if err := AppendRecord(path, r); err != nil {
			t.Fatal(err)
		}
	}
}

func fiveChecks() []Record {
	var recs []Record
	for i := range 5 {
		recs = append(recs, wrec("check", i+1, float64(i*1000), 250, 3.6, 3600))
	}
	return recs
}

func TestShowWeightsJSON(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	s.Classes["check"] = Class{Cores: 8, MemoryMB: 4608, Whole: true}
	writeRecords(t, s.RecordsPath, fiveChecks())
	if code := ShowQueue(s, []string{"weights", "--json"}, time.Now()); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var doc struct {
		Budget  Budget `json:"budget"`
		Jobs    int    `json:"jobs"`
		Rule    string `json:"rule"`
		Classes []struct {
			Class      string   `json:"class"`
			Jobs       int      `json:"jobs"`
			CoresUsed  Spread   `json:"cores_used"`
			Configured Weight   `json:"configured"`
			Suggested  *Weight  `json:"suggested"`
			Wall       Spread   `json:"wall_s"`
			Peak       Spread   `json:"peak_mem_mb"`
			WouldWait  WaitView `json:"would_wait"`
		} `json:"classes"`
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if doc.Budget != budget8 || doc.Jobs != 5 || doc.Rule == "" {
		t.Errorf("header = %+v", doc)
	}
	var found bool
	for _, c := range doc.Classes {
		if c.Class != "check" {
			continue
		}
		found = true
		if c.Jobs != 5 || c.Wall.P50 != 250 || c.Peak.Max != 3600 || !c.Configured.Whole {
			t.Errorf("check row = %+v", c)
		}
		if c.Suggested == nil || c.Suggested.Cores != 4 || c.Suggested.MemoryMB != 3840 {
			t.Errorf("check suggested = %+v, want 4 cores 3840 MB", c.Suggested)
		}
	}
	if !found {
		t.Errorf("no check row in %s", out.String())
	}
}

func TestShowWeightsHuman(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	writeRecords(t, s.RecordsPath, fiveChecks())
	if code := ShowQueue(s, []string{"weights"}, time.Now()); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"check", "5", "4m 10s", "3600", "4 cores", "3840 MB", "p90"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestShowWeightsTOMLLoadsAsClassesAndNoMode(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	writeRecords(t, s.RecordsPath, fiveChecks())
	if code := ShowQueue(s, []string{"weights", "--toml"}, time.Now()); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "mode") {
		t.Errorf("--toml output names a mode; switching modes is the user's edit:\n%s", out.String())
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatalf("output does not load as config: %v\n%s", err, out.String())
	}
	t.Cleanup(func() { config.C = config.Defaults() })
	c := config.C.Queue.Classes["check"]
	if c.Cores != 4 || c.MemoryMB != 3840 || c.Whole {
		t.Errorf("loaded check = %+v, want 4 cores 3840 MB not whole", c)
	}
	if config.C.Queue.Mode != config.QueueModeObserve {
		t.Errorf("mode = %q after loading suggestions, want observe", config.C.Queue.Mode)
	}
}

func TestShowWeightsWindow(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	writeRecords(t, s.RecordsPath, fiveChecks()) // starts at 0, 1000 … 4000 s
	args := []string{"weights", "--json",
		"--since", weighT0.Add(500 * time.Second).Format(time.RFC3339),
		"--until", weighT0.Add(3500 * time.Second).Format(time.RFC3339)}
	if code := ShowQueue(s, args, time.Now()); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var doc struct {
		Records int `json:"records"`
		Jobs    int `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Records != 3 || doc.Jobs != 3 {
		t.Errorf("window kept %d records / %d jobs, want 3/3", doc.Records, doc.Jobs)
	}
}

func TestShowWeightsIsReadOnly(t *testing.T) {
	s, _, _ := testSettings(t, ModeObserve)
	writeRecords(t, s.RecordsPath, fiveChecks())
	before, _ := os.ReadFile(s.RecordsPath)
	ShowQueue(s, []string{"weights"}, time.Now())
	after, _ := os.ReadFile(s.RecordsPath)
	if string(before) != string(after) {
		t.Error("weights changed runs.jsonl")
	}
	if _, err := os.Stat(s.QueueDir); !os.IsNotExist(err) {
		t.Errorf("weights touched the queue directory (stat err %v)", err)
	}
}

func TestShowWeightsRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"weights", "--bogus"},
		{"weights", "--since"},
		{"weights", "--since", "yesterday"},
		{"weights", "--json", "--toml"},
	} {
		s, _, errb := testSettings(t, ModeObserve)
		if code := ShowQueue(s, args, time.Now()); code != ExitUsage {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitUsage)
		}
		if !strings.Contains(errb.String(), "usage") {
			t.Errorf("%v: stderr = %q, want a usage line", args, errb.String())
		}
	}
}

func TestWeighCountsARecordedWaitWithNoOverlapFound(t *testing.T) {
	// The job it waited behind left no record (killed before writing one):
	// the wait still counts, it just has no bounds.
	f := wrec("files", 2, 0, 7, 1, 500)
	f.WouldWait = true
	r := rowFor(t, Weigh([]Record{f}, DefaultClassesForTest(), budget8), "files")
	if r.WouldWait.Jobs != 1 || r.WouldWait.Unbounded != 1 || r.WouldWait.Share != 1 {
		t.Errorf("would wait = %+v, want 1 job, 1 unbounded, share 1", r.WouldWait)
	}
}

func TestWeighReplaysConfiguredWeights(t *testing.T) {
	// A candidate config is tried against history by pointing
	// COOLANT_CONFIG at it: the configured replay uses its weights, not
	// the ones recorded at the time.
	suite := wrec("suite", 1, 0, 300, 4, 3000)
	f := wrec("files", 2, 10, 7, 1, 500)
	f.WouldWait = true // recorded under a whole suite
	candidate := DefaultClassesForTest()
	candidate["suite"] = Class{Cores: 6, MemoryMB: 4096}
	r := rowFor(t, Weigh([]Record{suite, f}, candidate, budget8), "files")
	if r.WouldWait.Jobs != 1 {
		t.Errorf("recorded = %+v, want 1", r.WouldWait)
	}
	if r.ConfiguredWouldWait.Jobs != 0 {
		t.Errorf("configured replay = %+v, want none (6+2 cores fit)", r.ConfiguredWouldWait)
	}
}
