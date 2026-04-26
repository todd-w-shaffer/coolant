package stats

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// loadResult bundles the loaded snapshot with internal signals New()
// needs but that don't belong on the public Snapshot type.
type loadResult struct {
	Snap               Snapshot
	OnDiskSchema       int  // raw schema_version as written, before any rewrite
	RecordsParseFailed bool // permissive path saw a records block but couldn't parse it
}

// loadCache returns (loadResult, true) on success, (zero, false) on
// any failure. Schema-mismatch falls through to a permissive partial
// parse that preserves primary fields (records, daily, first_seen,
// by_type, by_project) so historical high-scores survive a future
// schema bump.
func loadCache(path string) (loadResult, bool) {
	if path == "" {
		return loadResult{}, false
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		return loadResult{}, false
	}

	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	_ = json.Unmarshal(buf, &probe)

	var snap Snapshot
	if err := json.Unmarshal(buf, &snap); err == nil && snap.SchemaVersion == CurrentSchemaVersion {
		return loadResult{Snap: snap, OnDiskSchema: probe.SchemaVersion}, true
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf, &raw); err != nil {
		return loadResult{}, false
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
	res := loadResult{Snap: out, OnDiskSchema: probe.SchemaVersion}
	if v, ok := raw["records"]; ok {
		// Records are a primary field; an unmarshal error would silently
		// zero a user's leaderboards. Surface the failure via the
		// degraded counter so it's observable instead of silent.
		if err := json.Unmarshal(v, &res.Snap.Records); err != nil {
			res.RecordsParseFailed = true
		}
	}
	if v, ok := raw["today_distinct"]; ok {
		_ = json.Unmarshal(v, &res.Snap.TodayDistinct)
	}
	return res, true
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
	candidateDaily := copyDaily(a.daily)
	candidateToday := TodayDistinctSets{}
	if a.currentDayKey != "" {
		candidateToday.Date = a.currentDayKey
		candidateToday.Projects = setKeys(a.dailyDistinctProjects[a.currentDayKey])
		candidateToday.Sessions = setKeys(a.dailyDistinctSessions[a.currentDayKey])
	}
	a.mu.RUnlock()

	// Re-read disk under processLock so a concurrent writer's contribution
	// gets folded into our merge instead of being clobbered.
	loaded, _ := loadCache(a.cfg.CachePath)
	// Schema-downgrade refusal: in a rolling deploy, an old binary
	// might race against a new binary's checkpoint. Aborting the
	// write keeps the newer schema's records intact on disk; the
	// older process degrades quietly via the degraded counter and
	// a stderr log line, instead of clobbering.
	if loaded.OnDiskSchema > CurrentSchemaVersion {
		bumpDegraded(a.cfg.DegradedPath)
		log.Printf("stats: refusing checkpoint — on-disk schema v%d newer than this build's v%d (rolling deploy?)",
			loaded.OnDiskSchema, CurrentSchemaVersion)
		return nil
	}
	fresh := loaded.Snap
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
	// Per-day shape fields (PeakConcurrentDay, DistinctProjectsDay,
	// DistinctSessionsDay) are NOT additive — Counters.Add sums them
	// for uniformity, but the semantically-correct merge is max-per-bucket
	// against the candidate's in-memory value. Apply that override now.
	for day, cand := range candidateDaily {
		merged := fresh.Daily[day]
		if cand.PeakConcurrentDay > merged.PeakConcurrentDay {
			merged.PeakConcurrentDay = cand.PeakConcurrentDay
		}
		if cand.DistinctProjectsDay > merged.DistinctProjectsDay {
			merged.DistinctProjectsDay = cand.DistinctProjectsDay
		}
		if cand.DistinctSessionsDay > merged.DistinctSessionsDay {
			merged.DistinctSessionsDay = cand.DistinctSessionsDay
		}
		fresh.Daily[day] = merged
	}
	fresh.Records = maxMergeRecords(fresh.Records, candidateRecords)
	// Today distinct sets: union memory + disk so a concurrent
	// checkpointer's contributions aren't lost. Recompute the bucket
	// count from len(union) so the fields stay consistent with the
	// persisted set.
	fresh.TodayDistinct = mergeTodayDistinct(fresh.TodayDistinct, candidateToday)
	if fresh.TodayDistinct.Date != "" {
		bucket := fresh.Daily[fresh.TodayDistinct.Date]
		if n := int64(len(fresh.TodayDistinct.Projects)); n > bucket.DistinctProjectsDay {
			bucket.DistinctProjectsDay = n
		}
		if n := int64(len(fresh.TodayDistinct.Sessions)); n > bucket.DistinctSessionsDay {
			bucket.DistinctSessionsDay = n
		}
		fresh.Daily[fresh.TodayDistinct.Date] = bucket
	}
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

// maxMergeRecords leaderboard-merges every Records field. Each slot
// is a RecordList (or BurstRecordList) — Merge unions, dedupes by
// composite key keeping higher value, sorts desc, and truncates to
// recordListCap (preserving boundary ties).
func maxMergeRecords(disk, cand Records) Records {
	return Records{
		PeakConcurrent:    disk.PeakConcurrent.Merge(cand.PeakConcurrent),
		LongestAgentS:     disk.LongestAgentS.Merge(cand.LongestAgentS),
		LongestSessionS:   disk.LongestSessionS.Merge(cand.LongestSessionS),
		MostAgentsSession: disk.MostAgentsSession.Merge(cand.MostAgentsSession),
		BiggestBurst:      disk.BiggestBurst.Merge(cand.BiggestBurst),
	}
}

// mergeTodayDistinct unions two TodayDistinctSets. Different dates:
// newer wins (older sets are stale). dayKey() format "2006-01-02" is
// fixed-width zero-padded, so lexicographic > equals chronological.
// Same dates: deduped string-set union of Projects and Sessions.
func mergeTodayDistinct(disk, cand TodayDistinctSets) TodayDistinctSets {
	if disk.Date == "" {
		return cand
	}
	if cand.Date == "" {
		return disk
	}
	if disk.Date != cand.Date {
		if cand.Date > disk.Date {
			return cand
		}
		return disk
	}
	return TodayDistinctSets{
		Date:     disk.Date,
		Projects: unionStrings(disk.Projects, cand.Projects),
		Sessions: unionStrings(disk.Sessions, cand.Sessions),
	}
}

// unionStrings returns a deduped union of two string slices. Result
// order is map-iteration order (unstable); callers don't depend on it.
func unionStrings(a, b []string) []string {
	merged := make(map[string]struct{}, len(a)+len(b))
	for _, x := range a {
		merged[x] = struct{}{}
	}
	for _, x := range b {
		merged[x] = struct{}{}
	}
	return setKeys(merged)
}

// bumpDegraded appends one byte to the degraded counter file. Mirrors
// the bash side's degraded-write signal so a refused checkpoint is
// observable via the same `wc -l` consumer that surfaces other
// degraded-path bumps.
func bumpDegraded(path string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write([]byte{'\n'})
	_ = f.Close()
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
