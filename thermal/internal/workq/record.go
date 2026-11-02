package workq

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// RecordSchema versions the runs.jsonl envelope. Additive optional
// fields stay at 1; a rename or removal bumps it.
const RecordSchema = 1

// Record is one job's line in ~/.coolant/runs.jsonl.
type Record struct {
	Schema      int    `json:"schema"`
	TS          string `json:"ts"`
	End         string `json:"end"`
	Mode        string `json:"mode"`
	Class       string `json:"class"`
	Lane        string `json:"lane,omitempty"`
	Interactive bool   `json:"interactive"`
	Repo        string `json:"repo,omitempty"`
	Worktree    string `json:"worktree,omitempty"`
	Cwd         string `json:"cwd"`
	Cmd         string `json:"cmd"`
	Exit        int    `json:"exit"`
	// Wall time of the command itself, waiting excluded.
	WallS float64 `json:"wall_s"`
	// user+sys of the child as wait4 reports it — XNU folds each reaped
	// descendant into its parent, so this is the whole waited-for tree.
	CPUS float64 `json:"cpu_s"`
	// Peak of the summed phys_footprint over the live descendant tree,
	// sampled every MemSampleMS, max'd with the child's own ru_maxrss.
	PeakMemMB   float64 `json:"peak_mem_mb"`
	MemSamples  int     `json:"mem_samples"`
	MemSampleMS int     `json:"mem_sample_ms"`
	LoadStart   float64 `json:"load_start"`
	LoadEnd     float64 `json:"load_end"`
	WaitS       float64 `json:"wait_s"`
	// Cores handed to the child as COOLANT_GRANTED_CORES (enforce only).
	GrantedCores int `json:"granted_cores,omitempty"`
	// Whether enforce would have held this job at start (observe only).
	WouldWait     bool `json:"would_wait"`
	OthersRunning int  `json:"others_running"`
	OthersCores   int  `json:"others_cores"`
	OthersMemMB   int  `json:"others_mem_mb"`
	// Ran inside a held job's ticket; its resources are part of that
	// job's record too.
	Nested bool `json:"nested,omitempty"`
	// Ran unqueued because the queue could not be used.
	Degraded bool `json:"degraded,omitempty"`
	Seq      int  `json:"seq,omitempty"`
	PID      int  `json:"pid"`
}

// AppendRecord appends r as one JSONL line under flock, so concurrent
// jobs never splice lines however long the command is.
func AppendRecord(path string, r Record) error {
	r.Schema = RecordSchema
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	_, err = f.Write(append(b, '\n'))
	return err
}

// ReadRecent answers the records in the last maxBytes of path, oldest
// first, skipping torn lines and schemas this binary does not know.
func ReadRecent(path string, maxBytes int64) []Record {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > maxBytes {
		_, _ = f.Seek(st.Size()-maxBytes, io.SeekStart)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	var out []Record
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var r Record
		if json.Unmarshal(line, &r) != nil || r.Schema < 1 || r.Schema > RecordSchema {
			continue
		}
		out = append(out, r)
	}
	return out
}

// estimateWindow is how many of the latest matching records feed an
// estimate: recent enough to track a suite that grows, enough to damp
// one run that overlapped another.
const estimateWindow = 20

// Estimate answers the median wall time of the latest records for the
// same class in the same repo, else the same class anywhere.
func Estimate(recs []Record, class, repo string) (float64, bool) {
	if v, ok := medianWall(recs, func(r Record) bool { return r.Class == class && r.Repo == repo }); ok {
		return v, true
	}
	return medianWall(recs, func(r Record) bool { return r.Class == class })
}

func medianWall(recs []Record, match func(Record) bool) (float64, bool) {
	var walls []float64
	for i := len(recs) - 1; i >= 0 && len(walls) < estimateWindow; i-- {
		if match(recs[i]) {
			walls = append(walls, recs[i].WallS)
		}
	}
	if len(walls) == 0 {
		return 0, false
	}
	sort.Float64s(walls)
	m := len(walls) / 2
	if len(walls)%2 == 1 {
		return walls[m], true
	}
	return (walls[m-1] + walls[m]) / 2, true
}
