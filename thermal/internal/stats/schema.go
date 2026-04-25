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
const CurrentSchemaVersion = 1

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
}

// Counters are the windowable totals — one daily bucket carries the same
// six fields, plus DegradedWritesTotal which is read from the bash
// $COOLANT_DEGRADED_COUNT file at Snapshot time (not folded).
type Counters struct {
	AgentsStarted        int64 `json:"agents_started,omitempty"`
	AgentsCompleted      int64 `json:"agents_completed,omitempty"`
	AgentsOrphaned       int64 `json:"agents_orphaned,omitempty"`
	Sessions             int64 `json:"sessions,omitempty"`
	TranscriptBytesTotal int64 `json:"transcript_bytes,omitempty"`
	GateCapEvents        int64 `json:"gate_cap_events,omitempty"`
	DegradedWritesTotal  int64 `json:"degraded_writes,omitempty"`
}

// Add returns the per-key sum of two Counters. Used by Window/Lifetime
// rollups and by the checkpoint delta-merge.
func (c Counters) Add(o Counters) Counters {
	return Counters{
		AgentsStarted:        c.AgentsStarted + o.AgentsStarted,
		AgentsCompleted:      c.AgentsCompleted + o.AgentsCompleted,
		AgentsOrphaned:       c.AgentsOrphaned + o.AgentsOrphaned,
		Sessions:             c.Sessions + o.Sessions,
		TranscriptBytesTotal: c.TranscriptBytesTotal + o.TranscriptBytesTotal,
		GateCapEvents:        c.GateCapEvents + o.GateCapEvents,
		DegradedWritesTotal:  c.DegradedWritesTotal + o.DegradedWritesTotal,
	}
}

// Records are the eternal high-score moments. Max-merged on every fold,
// preserved across cache discards.
type Records struct {
	PeakConcurrent    RecordEntry `json:"peak_concurrent"`
	LongestAgentS     RecordEntry `json:"longest_agent_s"`
	LongestSessionS   RecordEntry `json:"longest_session_s"`
	MostAgentsSession RecordEntry `json:"most_agents_session"`
	BiggestBurst      BurstRecord `json:"biggest_burst"`
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

// Lifetime sums every daily bucket into one Counters. The aggregator
// never stores lifetime totals separately — sum(daily) is the source
// of truth, eliminating regression risk from cache-vs-live drift.
func (s Snapshot) Lifetime() Counters {
	var total Counters
	for _, c := range s.Daily {
		total = total.Add(c)
	}
	return total
}
