package stats

import (
	"bytes"
	"os"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

// Config carries the paths and injection points the Aggregator needs.
// Production callers pass production paths; tests pass tempdir paths
// and (optionally) a stub TranscriptStat. The constructor is a struct
// rather than positional args (deviation from spec §4.2 signature) so
// tests can substitute the degraded counter path and transcript-size
// hook without touching package globals.
type Config struct {
	CachePath    string
	JSONLPath    string
	DegradedPath string
	// TranscriptStat returns the byte size of a transcript file, or 0
	// if it can't be read. Defaults to os.Stat-based size when nil.
	TranscriptStat func(path string) int64
}

// Aggregator owns the in-memory aggregate state. Mutated only via Fold
// (write lock) and Checkpoint (write lock during disk dance). Snapshot
// is read-locked.
type Aggregator struct {
	mu  sync.RWMutex
	cfg Config

	firstSeen   time.Time
	lastUpdated time.Time

	byType    map[string]int64
	byProject map[string]int64
	daily     map[string]Counters // keyed by dayKey(ts)
	records   Records

	seenSessions map[string]struct{}

	// agentStarts doubles as the active-agent set — presence == active.
	// Pruned at Checkpoint to bound growth from agents that crash without
	// emitting agent.stop (mirrors bash's 24h cutoff in _compute_agent_duration).
	agentStarts       map[string]time.Time
	agentMeta         map[string]agentMeta
	sessionFirstStart map[string]time.Time
	sessionAgentCount map[string]int64
	burstWindow       []time.Time

	// baseline = on-disk state at last load/checkpoint; delta = (current -
	// baseline) merged into fresh disk read at Checkpoint time so two
	// thermos can't clobber each other's increments.
	baseline Snapshot
}

// agentMeta is the minimum agent.start metadata needed to attribute
// a longest_agent_s record on the matching agent.stop, so we don't
// have to re-read the original start event.
type agentMeta struct {
	AgentType string
	Project   string
	SessionID string
}

// burstWindowS is the sliding window for biggest-burst detection.
// Hardcoded to 2 in v1 (locked decision §0); record schema preserves
// it as a field so a future bump can be traced.
const burstWindowS = 2

// New constructs an Aggregator and loads the on-disk cache if present.
// Missing, mangled, or schema-mismatched caches yield an empty
// in-memory aggregator (no panic, no error returned) — callers can
// always assume New returns a usable instance. Schema-mismatch path
// permissively partial-parses primary fields (records, daily,
// first_seen, by_type, by_project) so historical high-scores survive
// a future schema bump.
func New(cfg Config) *Aggregator {
	if cfg.TranscriptStat == nil {
		cfg.TranscriptStat = statSize
	}
	a := &Aggregator{
		cfg:               cfg,
		byType:            map[string]int64{},
		byProject:         map[string]int64{},
		daily:             map[string]Counters{},
		seenSessions:      map[string]struct{}{},
		agentStarts:       map[string]time.Time{},
		agentMeta:         map[string]agentMeta{},
		sessionFirstStart: map[string]time.Time{},
		sessionAgentCount: map[string]int64{},
		records:           Records{BiggestBurst: BurstRecord{WindowS: burstWindowS}},
	}
	if loaded, ok := loadCache(cfg.CachePath); ok {
		a.byType = copyInt64Map(loaded.ByType)
		a.byProject = copyInt64Map(loaded.ByProject)
		a.daily = copyDaily(loaded.Daily)
		a.records = loaded.Records
		a.firstSeen = loaded.FirstSeen
		// Default BiggestBurst.WindowS if cache predates the field.
		if a.records.BiggestBurst.WindowS == 0 {
			a.records.BiggestBurst.WindowS = burstWindowS
		}
		a.baseline = loaded
	}
	return a
}

// dayKey is the canonical UTC day-bucket key for the daily map.
// Centralized here so a future timezone-policy change touches one site.
func dayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// staleAgentCutoff bounds agentStarts/agentMeta growth from agents that
// crash or get killed without emitting agent.stop (kill -9, hook race,
// etc.). Mirrors the bash side's 24h cutoff in _compute_agent_duration.
const staleAgentCutoff = 24 * time.Hour

// pruneStale drops agentStarts/agentMeta entries older than the cutoff.
// Caller must hold a.mu.Lock(). Cheap relative to Checkpoint's disk
// dance; runs once per checkpoint cycle.
func (a *Aggregator) pruneStale(now time.Time) {
	cutoff := now.Add(-staleAgentCutoff)
	for id, started := range a.agentStarts {
		if started.Before(cutoff) {
			delete(a.agentStarts, id)
			delete(a.agentMeta, id)
		}
	}
}

// statSize is the default TranscriptStat. Missing files contribute 0 —
// Claude Code prunes old transcripts, so a stop event whose path no
// longer resolves is the steady-state case, not an error.
func statSize(path string) int64 {
	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// Fold mutates aggregates from one parsed JSONL event. Pipeline:
//  1. Schema gate: drops events with Schema < 1 or Schema > MaxKnownSchema.
//  2. Type switch on Event. Unknown event types are silently ignored —
//     the producer's bats coverage carries the contract, not a runtime
//     allowlist.
//  3. Per-bucket additive updates under write lock.
//
// byteOffset is the per-event start-of-line position in the JSONL,
// captured by events.go before the scanner advances. Used by the
// watermark id synthesis (cycle 5); fold logic itself ignores it.
func (a *Aggregator) Fold(evt collector.GateEvent, byteOffset int64) {
	if evt.Schema < 1 || evt.Schema > MaxKnownSchema {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.firstSeen.IsZero() {
		a.firstSeen = evt.Timestamp
	}
	a.lastUpdated = evt.Timestamp

	day := dayKey(evt.Timestamp)
	bucket := a.daily[day]

	switch evt.Event {
	case collector.EventAgentStart:
		bucket.AgentsStarted++
		if evt.AgentType != "" {
			a.byType[evt.AgentType]++
		}
		if evt.Project != "" {
			a.byProject[evt.Project]++
		}
		if evt.AgentID != "" {
			a.agentStarts[evt.AgentID] = evt.Timestamp
			a.agentMeta[evt.AgentID] = agentMeta{
				AgentType: evt.AgentType,
				Project:   evt.Project,
				SessionID: evt.SessionID,
			}
		}
		if evt.SessionID != "" {
			if _, seen := a.seenSessions[evt.SessionID]; !seen {
				a.seenSessions[evt.SessionID] = struct{}{}
				bucket.Sessions++
				a.sessionFirstStart[evt.SessionID] = evt.Timestamp
			}
			a.sessionAgentCount[evt.SessionID]++
			if c := a.sessionAgentCount[evt.SessionID]; c > a.records.MostAgentsSession.Value {
				a.records.MostAgentsSession = RecordEntry{
					Value:     c,
					SessionID: evt.SessionID,
					At:        evt.Timestamp,
				}
			}
		}
		if int64(len(a.agentStarts)) > a.records.PeakConcurrent.Value {
			a.records.PeakConcurrent = RecordEntry{
				Value:     int64(len(a.agentStarts)),
				SessionID: evt.SessionID,
				At:        evt.Timestamp,
			}
		}
		a.observeBurst(evt.Timestamp, evt.SessionID)

	case collector.EventAgentStop:
		bucket.AgentsCompleted++
		var matched bool
		if evt.AgentID != "" {
			if _, ok := a.agentStarts[evt.AgentID]; ok {
				matched = true
			} else {
				bucket.AgentsCompleted--
				bucket.AgentsOrphaned++
			}
		}
		if evt.TranscriptPath != "" {
			bucket.TranscriptBytesTotal += a.cfg.TranscriptStat(evt.TranscriptPath)
		}
		if matched {
			start, ok := a.agentStarts[evt.AgentID]
			if ok {
				duration := int64(evt.Timestamp.Sub(start).Seconds())
				meta := a.agentMeta[evt.AgentID]
				if duration > a.records.LongestAgentS.Value {
					a.records.LongestAgentS = RecordEntry{
						Value:     duration,
						AgentID:   evt.AgentID,
						AgentType: meta.AgentType,
						Project:   meta.Project,
						SessionID: meta.SessionID,
						At:        evt.Timestamp,
					}
				}
				delete(a.agentStarts, evt.AgentID)
				delete(a.agentMeta, evt.AgentID)
			}
		}
		if evt.SessionID != "" {
			if first, ok := a.sessionFirstStart[evt.SessionID]; ok {
				duration := int64(evt.Timestamp.Sub(first).Seconds())
				if duration > a.records.LongestSessionS.Value {
					a.records.LongestSessionS = RecordEntry{
						Value:     duration,
						SessionID: evt.SessionID,
						At:        evt.Timestamp,
					}
				}
			}
		}

	case collector.EventGateCap:
		bucket.GateCapEvents++

	case collector.EventCounterReset:
		// No-op for stats — sessions are keyed on session_id, not on
		// counter epochs (locked decision §0).

	default:
		// Unknown event type: silent drop, no panic. New event types
		// added in the future are validated by producer-side tests
		// (we control both sides).
		return
	}

	a.daily[day] = bucket
}

// Snapshot returns a deep-copy view of current state. Read-locked
// during the copy. DegradedWritesTotal is read from the bash-side
// counter file at snapshot time (not folded into daily buckets) —
// it's a global ambient signal, not a per-day event count.
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()

	s := Snapshot{
		SchemaVersion: CurrentSchemaVersion,
		FirstSeen:     a.firstSeen,
		LastUpdated:   a.lastUpdated,
		Records:       a.records,
		ByType:        copyInt64Map(a.byType),
		ByProject:     copyInt64Map(a.byProject),
		Daily:         copyDaily(a.daily),
	}

	// Surface the bash degraded-write counter as a lifetime-only field.
	// Stamped into today's bucket for visibility — Lifetime() sums all
	// daily, so this surfaces in lifetime totals identically.
	if degraded := readDegraded(a.cfg.DegradedPath); degraded > 0 {
		today := dayKey(time.Now())
		bucket := s.Daily[today]
		bucket.DegradedWritesTotal = degraded
		s.Daily[today] = bucket
	}

	return s
}

// observeBurst trims age-based (not count-based — a 50-agent burst stays
// whole) and updates the biggest-burst record. Caller must hold a.mu.
func (a *Aggregator) observeBurst(ts time.Time, sessionID string) {
	cutoff := ts.Add(-burstWindowS * time.Second)
	keep := a.burstWindow[:0]
	for _, t := range a.burstWindow {
		if !t.Before(cutoff) {
			keep = append(keep, t)
		}
	}
	keep = append(keep, ts)
	a.burstWindow = keep

	count := int64(len(a.burstWindow))
	if count > a.records.BiggestBurst.Count {
		a.records.BiggestBurst = BurstRecord{
			Count:     count,
			WindowS:   burstWindowS,
			SessionID: sessionID,
			// At anchors on the FIRST start so a burst spanning midnight
			// gets day-attributed to its start day.
			At: a.burstWindow[0],
		}
	}
}

// Window returns counters summed across the last N days, anchored on
// today's UTC date. Missing days count as zero. N=7/30/60/90 are the
// canonical scoreboard windows; "alltime" callers use Snapshot.Lifetime.
func (a *Aggregator) Window(days int) Counters {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var total Counters
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		key := dayKey(now.Add(-time.Duration(i) * 24 * time.Hour))
		total = total.Add(a.daily[key])
	}
	return total
}

// VisibleWindows returns the labels appropriate for the current install
// age (days since first_seen). The tier rules are locked in §0:
//
//	day_age <  30 → ["7d", "alltime"]
//	day_age <  60 → ["7d", "30d"]
//	day_age <  90 → ["7d", "30d", "60d"]
//	day_age >= 90 → ["7d", "30d", "90d", "alltime"]
//
// "alltime" is included on the youngest tier so day-1 users see
// something resembling their full history; on the >=90 tier it returns
// alongside 90d as the always-anchored eternal window.
func (a *Aggregator) VisibleWindows() []string {
	a.mu.RLock()
	first := a.firstSeen
	a.mu.RUnlock()
	if first.IsZero() {
		return []string{"7d", "alltime"}
	}
	age := int(time.Since(first).Hours() / 24)
	switch {
	case age < 30:
		return []string{"7d", "alltime"}
	case age < 60:
		return []string{"7d", "30d"}
	case age < 90:
		return []string{"7d", "30d", "60d"}
	default:
		return []string{"7d", "30d", "90d", "alltime"}
	}
}

// readDegraded counts newlines in $COOLANT_DEGRADED_COUNT. Bash's
// fallback path appends one '\n' per torn-write incident (§3.2 of
// the bash spec). Missing file → 0 (the bash side never created it).
func readDegraded(path string) int64 {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return int64(bytes.Count(data, []byte{'\n'}))
}

func copyInt64Map(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyDaily(m map[string]Counters) map[string]Counters {
	out := make(map[string]Counters, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
