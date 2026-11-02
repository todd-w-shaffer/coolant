package workq

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppendRecordWritesOneJSONLinePerJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "runs.jsonl")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AppendRecord(path, Record{Class: "suite", Cmd: strings.Repeat("x", 6000), Seq: i}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 20 {
		t.Fatalf("lines = %d, want 20", len(lines))
	}
	for _, l := range lines {
		var r Record
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("torn line: %v", err)
		}
		if r.Schema != RecordSchema {
			t.Fatalf("schema = %d", r.Schema)
		}
	}
}

func TestReadRecentSkipsTornAndForeignLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	body := `{"schema":1,"class":"suite","wall_s":10}
not json
{"schema":99,"class":"suite","wall_s":999}
{"schema":1,"class":"files","wall_s":3}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	recs := ReadRecent(path, 1<<20)
	if len(recs) != 2 || recs[0].Class != "suite" || recs[1].Class != "files" {
		t.Fatalf("recs = %+v", recs)
	}
}

func TestEstimatePrefersSameRepoThenSameClass(t *testing.T) {
	recs := []Record{
		{Class: "suite", Repo: "/a", WallS: 100},
		{Class: "suite", Repo: "/a", WallS: 300},
		{Class: "suite", Repo: "/a", WallS: 200},
		{Class: "suite", Repo: "/b", WallS: 900},
		{Class: "files", Repo: "/a", WallS: 5},
	}
	if got, ok := Estimate(recs, "suite", "/a"); !ok || got != 200 {
		t.Errorf("same repo = %v %v, want 200 (median)", got, ok)
	}
	if got, ok := Estimate(recs, "suite", "/c"); !ok || got != 250 {
		t.Errorf("class fallback = %v %v, want 250 (median of 100,200,300,900)", got, ok)
	}
	if _, ok := Estimate(recs, "lint", "/a"); ok {
		t.Error("no records for lint should be unknown")
	}
}

func TestEstimateUsesOnlyTheLatestTwenty(t *testing.T) {
	var recs []Record
	for range 30 {
		recs = append(recs, Record{Class: "build", Repo: "/a", WallS: 1000})
	}
	for range 20 {
		recs = append(recs, Record{Class: "build", Repo: "/a", WallS: 10})
	}
	if got, _ := Estimate(recs, "build", "/a"); got != 10 {
		t.Errorf("estimate = %v, want 10", got)
	}
}

func TestHorizonEstimatesStartsAgainstTheBudget(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	started := now.Add(-60 * time.Second).Format(time.RFC3339)
	running := []Ticket{{Seq: 1, Class: "suite", Whole: true, MemMB: 3000, State: StateRunning, StartedAt: started}}
	waiting := []Ticket{
		{Seq: 2, Class: "typecheck", Cores: 1, MemMB: 1000, State: StateWaiting},
		{Seq: 3, Class: "build", Cores: 1, MemMB: 500, State: StateWaiting},
		{Seq: 4, Class: "suite", Whole: true, MemMB: 3000, State: StateWaiting},
	}
	est := func(t Ticket) (float64, bool) {
		switch t.Class {
		case "suite":
			return 240, true
		case "typecheck":
			return 30, true
		case "build":
			return 10, true
		}
		return 0, false
	}
	h := Horizon(running, waiting, budget8, est, now)
	if h.Running[0].ElapsedS != 60 || h.Running[0].RemainingS != 180 {
		t.Errorf("running = %+v", h.Running[0])
	}
	want := []float64{180, 180, 210} // typecheck+build start together; suite after typecheck ends
	for i, w := range h.Waiting {
		if !w.Known || w.StartInS != want[i] {
			t.Errorf("waiting[%d] start = %v (known %v), want %v", i, w.StartInS, w.Known, want[i])
		}
	}
}

func TestHorizonMarksStartsAfterAnUnknownJobUnknown(t *testing.T) {
	now := time.Now()
	running := []Ticket{{Seq: 1, Class: "mystery", Whole: true, State: StateRunning, StartedAt: now.Format(time.RFC3339)}}
	waiting := []Ticket{{Seq: 2, Class: "suite", Whole: true, State: StateWaiting}}
	h := Horizon(running, waiting, budget8, func(Ticket) (float64, bool) { return 0, false }, now)
	if h.Waiting[0].Known {
		t.Error("a start behind a job of unknown length must be unknown")
	}
}

func TestWaitLineNamesWhatIsAhead(t *testing.T) {
	ahead := []Ticket{
		{PID: 51234, Lane: "nightly", Class: "suite", Cwd: "/x/myapp", State: StateRunning},
		{PID: 51300, Class: "typecheck", Cwd: "/y/coolant", State: StateWaiting},
	}
	line := WaitLine(ahead)
	for _, want := range []string{"2 ahead", "pid 51234", "nightly", "suite", "/x/myapp", "running", "pid 51300", "interactive", "typecheck", "/y/coolant", "waiting"} {
		if !strings.Contains(line, want) {
			t.Errorf("wait line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "\n") {
		t.Error("wait line must be one line")
	}
}

func TestWaitLineTruncatesLongQueues(t *testing.T) {
	var ahead []Ticket
	for i := range 6 {
		ahead = append(ahead, Ticket{PID: 100 + i, Class: "build", Cwd: "/r", State: StateWaiting})
	}
	line := WaitLine(ahead)
	if !strings.Contains(line, "+3 more") || strings.Contains(line, "pid 105") {
		t.Errorf("line = %q", line)
	}
}
