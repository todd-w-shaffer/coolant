// Package stats persists cross-session agent aggregates derived from the
// coolant JSONL event log. The on-disk shape lives in ~/.coolant/stats.json.
//
// JSONL is the streaming source of truth; the cache is durable history.
// Several fields are *primary* — they cannot be reconstructed from a
// post-rotation JSONL and must survive every cache-discard / migration
// path: records, daily buckets, first_seen, by_type, by_project. Lifetime
// counters are derived from sum(daily) — never stored separately.
package stats

import "time"

// MaxKnownSchema is the highest envelope schema this aggregator
// understands. Events with Schema > MaxKnownSchema are dropped to
// prevent an old binary from misinterpreting future-shaped events.
const MaxKnownSchema = 1

// CurrentSchemaVersion is the on-disk cache shape this build writes.
// Bumped only on field renames, removals, or semantic changes —
// pure-additive optional fields stay on the current version.
//
// v2 retypes every Records field from a single RecordEntry/BurstRecord
// to a leaderboard-shaped RecordList/BurstRecordList. Migration is
// transparent via custom UnmarshalJSON on the list types — a v1 cache
// loads with each field becoming a 1-element slice. Rollback to v1
// after a v2 write WIPES Records (the inverse silently zeros under v1's
// permissive partial parse) — accept the one-shot loss; document in
// release notes.
const CurrentSchemaVersion = 2

// Snapshot is a deep-copy view of the aggregator's state at a point
// in time. Returned by Aggregator.Snapshot() under read lock.
type Snapshot struct {
	SchemaVersion int                 `json:"schema_version"`
	FirstSeen     time.Time           `json:"first_seen"`
	LastUpdated   time.Time           `json:"last_updated"`
	Records       Records             `json:"records"`
	ByType        map[string]int64    `json:"by_type"`
	ByProject     map[string]int64    `json:"by_project"`
	Daily         map[string]Counters `json:"daily"`
	TodayDistinct TodayDistinctSets   `json:"today_distinct,omitempty"`
}

// TodayDistinctSets persists the in-memory distinct-project and
// distinct-session sets for the *current* UTC day so a thermo
// restart mid-day doesn't send DistinctProjectsDay backwards.
// Older days carry only the integer count in their bucket; on
// load, if Date doesn't match today's UTC day, the block is
// discarded — the previous day's count was frozen into its bucket
// at the last pre-rollover Checkpoint.
type TodayDistinctSets struct {
	Date     string   `json:"date,omitempty"`
	Projects []string `json:"projects,omitempty"`
	Sessions []string `json:"sessions,omitempty"`
}

// Counters are the windowable totals carried on each daily bucket.
// Most fields are additive across days; a few (PeakConcurrentDay,
// DistinctProjectsDay, DistinctSessionsDay) are per-day max/distinct
// — Lifetime/Window summing them isn't semantically meaningful; the
// `BestDay*` Snapshot helpers provide the correct access pattern.
// DegradedWritesTotal is read from the bash $COOLANT_DEGRADED_COUNT
// file at Snapshot time (not folded).
type Counters struct {
	AgentsStarted        int64 `json:"agents_started,omitempty"`
	AgentsCompleted      int64 `json:"agents_completed,omitempty"`
	AgentsOrphaned       int64 `json:"agents_orphaned,omitempty"`
	Sessions             int64 `json:"sessions,omitempty"`
	TranscriptBytesTotal int64 `json:"transcript_bytes,omitempty"`
	GateCapEvents        int64 `json:"gate_cap_events,omitempty"`
	DegradedWritesTotal  int64 `json:"degraded_writes,omitempty"`
	PeakConcurrentDay    int64 `json:"peak_concurrent_day,omitempty"`
	DistinctProjectsDay  int64 `json:"distinct_projects_day,omitempty"`
	DistinctSessionsDay  int64 `json:"distinct_sessions_day,omitempty"`
}

// Add returns the per-key sum of two Counters. Used by Window/Lifetime
// rollups and by the checkpoint delta-merge for additive fields. Note:
// PeakConcurrentDay/DistinctProjectsDay/DistinctSessionsDay are touched
// here so the field-coverage drift test passes, but these are not
// semantically additive — Checkpoint applies max-merge for them per
// bucket separately.
func (c Counters) Add(o Counters) Counters {
	return Counters{
		AgentsStarted:        c.AgentsStarted + o.AgentsStarted,
		AgentsCompleted:      c.AgentsCompleted + o.AgentsCompleted,
		AgentsOrphaned:       c.AgentsOrphaned + o.AgentsOrphaned,
		Sessions:             c.Sessions + o.Sessions,
		TranscriptBytesTotal: c.TranscriptBytesTotal + o.TranscriptBytesTotal,
		GateCapEvents:        c.GateCapEvents + o.GateCapEvents,
		DegradedWritesTotal:  c.DegradedWritesTotal + o.DegradedWritesTotal,
		PeakConcurrentDay:    c.PeakConcurrentDay + o.PeakConcurrentDay,
		DistinctProjectsDay:  c.DistinctProjectsDay + o.DistinctProjectsDay,
		DistinctSessionsDay:  c.DistinctSessionsDay + o.DistinctSessionsDay,
	}
}

// Records are the eternal high-score leaderboards. Each field carries
// up to recordListCap entries, sorted desc by Value (or Count for
// BurstRecord), deduped by composite key. Preserved across cache
// discards via the custom UnmarshalJSON on each list type.
type Records struct {
	PeakConcurrent    RecordList      `json:"peak_concurrent"`
	LongestAgentS     RecordList      `json:"longest_agent_s"`
	LongestSessionS   RecordList      `json:"longest_session_s"`
	MostAgentsSession RecordList      `json:"most_agents_session"`
	BiggestBurst      BurstRecordList `json:"biggest_burst"`
}

// RecordEntry covers all non-burst records. Optional fields stay zero
// when not applicable (e.g., AgentID is empty for session-scoped records).
type RecordEntry struct {
	Value     int64     `json:"value"`
	SessionID string    `json:"session_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	AgentType string    `json:"agent_type,omitempty"`
	Project   string    `json:"project,omitempty"`
	At        time.Time `json:"at"`
}

// BurstRecord captures most-starts-in-a-sliding-window. WindowS is
// hardcoded to 2 in v1; field exists so a future bump can be traced.
type BurstRecord struct {
	Count     int64     `json:"count"`
	WindowS   int       `json:"window_s"`
	SessionID string    `json:"session_id,omitempty"`
	At        time.Time `json:"at"`
}

// nonAdditiveCounterFields names Counters fields whose semantics are
// NOT additive across days/processes. Counters.Add still touches them
// (uniformity for the drift test) but the per-day delta math in
// computeDelta and the per-bucket merge in Checkpoint treat them
// specially: max-merge for PeakConcurrentDay and set-derived recompute
// for the Distinct*Day pair. DegradedWritesTotal is sourced from the
// bash side at Snapshot time and isn't part of the delta either.
//
// When a future spec adds a non-additive Counters field, add its name
// here; the coverage_test field walk uses this same set so adding to
// it both excludes the field from delta math AND keeps the drift
// guard passing.
var nonAdditiveCounterFields = map[string]struct{}{
	"DegradedWritesTotal": {},
	"PeakConcurrentDay":   {},
	"DistinctProjectsDay": {},
	"DistinctSessionsDay": {},
}

// Lifetime sums every daily bucket into one Counters. The aggregator
// never stores lifetime totals separately — sum(daily) is the source
// of truth, eliminating regression risk from cache-vs-live drift.
//
// Note: PeakConcurrentDay, DistinctProjectsDay, and DistinctSessionsDay
// are NOT semantically additive — Lifetime sums them for uniformity
// but the result is meaningless. Use BestDayPeakConcurrent /
// BestDayDistinctProjects / BestDayDistinctSessions for the
// max-across-days access pattern.
func (s Snapshot) Lifetime() Counters {
	var total Counters
	for _, c := range s.Daily {
		total = total.Add(c)
	}
	return total
}

// BestDayPeakConcurrent returns the day-key + value of the highest
// PeakConcurrentDay across all daily buckets. Empty Daily map returns
// ("", 0). Method-per-field instead of stringly-typed lookup so a
// renamed field becomes a compile error, not a silent zero-return.
func (s Snapshot) BestDayPeakConcurrent() (string, int64) {
	return bestDay(s.Daily, func(c Counters) int64 { return c.PeakConcurrentDay })
}

// BestDayDistinctProjects returns the day-key + value of the highest
// DistinctProjectsDay across all daily buckets. Empty Daily returns
// ("", 0).
func (s Snapshot) BestDayDistinctProjects() (string, int64) {
	return bestDay(s.Daily, func(c Counters) int64 { return c.DistinctProjectsDay })
}

// BestDayDistinctSessions returns the day-key + value of the highest
// DistinctSessionsDay across all daily buckets. Empty Daily returns
// ("", 0).
func (s Snapshot) BestDayDistinctSessions() (string, int64) {
	return bestDay(s.Daily, func(c Counters) int64 { return c.DistinctSessionsDay })
}

func bestDay(daily map[string]Counters, pick func(Counters) int64) (string, int64) {
	var (
		bestK string
		bestV int64
	)
	for k, c := range daily {
		v := pick(c)
		if v > bestV || (v == bestV && bestK == "") {
			bestV = v
			bestK = k
		}
	}
	return bestK, bestV
}
