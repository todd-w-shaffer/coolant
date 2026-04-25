package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// loadCache returns (Snapshot, true) on success, (zero, false) on any
// failure. Schema-mismatch falls through to a permissive partial parse
// that preserves primary fields (records, daily, first_seen, by_type,
// by_project) so historical high-scores survive a future schema bump.
func loadCache(path string) (Snapshot, bool) {
	if path == "" {
		return Snapshot{}, false
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, false
	}

	var snap Snapshot
	if err := json.Unmarshal(buf, &snap); err == nil && snap.SchemaVersion == CurrentSchemaVersion {
		return snap, true
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf, &raw); err != nil {
		return Snapshot{}, false
	}
	out := Snapshot{SchemaVersion: CurrentSchemaVersion}
	if v, ok := raw["first_seen"]; ok {
		_ = json.Unmarshal(v, &out.FirstSeen)
	}
	if v, ok := raw["by_type"]; ok {
		_ = json.Unmarshal(v, &out.ByType)
	}
	if v, ok := raw["by_project"]; ok {
		_ = json.Unmarshal(v, &out.ByProject)
	}
	if v, ok := raw["daily"]; ok {
		_ = json.Unmarshal(v, &out.Daily)
	}
	if v, ok := raw["records"]; ok {
		_ = json.Unmarshal(v, &out.Records)
	}
	return out, true
}

// processLock serializes Checkpoint within a process. Cross-process
// flock is §13 future hardening — the delta-merge math handles the
// in-process case without a system-level lock.
var processLock sync.Mutex

// Checkpoint writes the aggregate state to stats.json via the §7.2
// delta-merge dance: re-read disk, per-key additive merge for the
// counter maps, max-merge records, atomic-write the result.
func (a *Aggregator) Checkpoint() error {
	if a.cfg.CachePath == "" {
		return nil
	}

	processLock.Lock()
	defer processLock.Unlock()

	a.mu.RLock()
	delta := computeDelta(a.byType, a.byProject, a.daily, a.baseline)
	candidateRecords := a.records
	firstSeen := a.firstSeen
	a.mu.RUnlock()

	// Re-read disk under processLock so a concurrent writer's contribution
	// gets folded into our merge instead of being clobbered.
	fresh, _ := loadCache(a.cfg.CachePath)
	if fresh.ByType == nil {
		fresh.ByType = map[string]int64{}
	}
	if fresh.ByProject == nil {
		fresh.ByProject = map[string]int64{}
	}
	if fresh.Daily == nil {
		fresh.Daily = map[string]Counters{}
	}

	for k, v := range delta.ByType {
		fresh.ByType[k] += v
	}
	for k, v := range delta.ByProject {
		fresh.ByProject[k] += v
	}
	for k, v := range delta.Daily {
		fresh.Daily[k] = fresh.Daily[k].Add(v)
	}
	fresh.Records = maxMergeRecords(fresh.Records, candidateRecords)
	fresh.SchemaVersion = CurrentSchemaVersion
	fresh.LastUpdated = time.Now().UTC()
	fresh.FirstSeen = earliestNonZero(fresh.FirstSeen, firstSeen)

	if err := atomicWriteJSON(a.cfg.CachePath, fresh); err != nil {
		return err
	}

	a.mu.Lock()
	a.baseline = fresh
	a.pruneStale(time.Now())
	a.mu.Unlock()
	return nil
}

func computeDelta(byType, byProject map[string]int64, daily map[string]Counters, baseline Snapshot) Snapshot {
	d := Snapshot{
		ByType:    map[string]int64{},
		ByProject: map[string]int64{},
		Daily:     map[string]Counters{},
	}
	for k, v := range byType {
		if diff := v - baseline.ByType[k]; diff != 0 {
			d.ByType[k] = diff
		}
	}
	for k, v := range byProject {
		if diff := v - baseline.ByProject[k]; diff != 0 {
			d.ByProject[k] = diff
		}
	}
	for k, v := range daily {
		base := baseline.Daily[k]
		diff := Counters{
			AgentsStarted:        v.AgentsStarted - base.AgentsStarted,
			AgentsCompleted:      v.AgentsCompleted - base.AgentsCompleted,
			AgentsOrphaned:       v.AgentsOrphaned - base.AgentsOrphaned,
			Sessions:             v.Sessions - base.Sessions,
			TranscriptBytesTotal: v.TranscriptBytesTotal - base.TranscriptBytesTotal,
			GateCapEvents:        v.GateCapEvents - base.GateCapEvents,
		}
		if diff != (Counters{}) {
			d.Daily[k] = diff
		}
	}
	return d
}

// maxMergeRecords keeps the higher of two record entries per slot.
// Equal-value tie: candidate (newer At) wins per spec §0.
func maxMergeRecords(disk, cand Records) Records {
	out := disk
	out.PeakConcurrent = pickRecord(disk.PeakConcurrent, cand.PeakConcurrent)
	out.LongestAgentS = pickRecord(disk.LongestAgentS, cand.LongestAgentS)
	out.LongestSessionS = pickRecord(disk.LongestSessionS, cand.LongestSessionS)
	out.MostAgentsSession = pickRecord(disk.MostAgentsSession, cand.MostAgentsSession)
	out.BiggestBurst = pickBurst(disk.BiggestBurst, cand.BiggestBurst)
	return out
}

func pickRecord(a, b RecordEntry) RecordEntry {
	if b.Value > a.Value {
		return b
	}
	if b.Value == a.Value && b.At.After(a.At) {
		return b
	}
	return a
}

func pickBurst(a, b BurstRecord) BurstRecord {
	if b.Count > a.Count {
		return b
	}
	if b.Count == a.Count && b.At.After(a.At) {
		return b
	}
	return a
}

func earliestNonZero(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.Before(b) {
		return a
	}
	return b
}

// atomicWriteJSON marshals snap and writes it via the
// tempfile-fsync-rename-parent-fsync dance. APFS rename is atomic
// but not durable without parent-dir fsync — without it, power loss
// can resurrect the old file even after rename returned successfully.
func atomicWriteJSON(path string, snap Snapshot) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	buf, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
