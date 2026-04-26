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

	// Lifecycle map — explicit session.start / session.end pairs let
	// LongestSessionS use real wall-clock duration instead of inferring
	// from the first agent.start.
	sessionStart           map[string]time.Time
	sessionEnded           map[string]bool
	lastActivityForSession map[string]time.Time

	// Per-day distinct sets. Outer key is dayKey(ts); inner is the
	// value set (project name or session_id). Counts are derived via
	// `len(set)` and stamped onto the day's Counters bucket on every
	// fold. Day rollover freezes the previous day's count and clears
	// the previous day's inner map.
	dailyDistinctProjects map[string]map[string]struct{}
	dailyDistinctSessions map[string]map[string]struct{}
	currentDayKey         string

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
		cfg:                    cfg,
		byType:                 map[string]int64{},
		byProject:              map[string]int64{},
		daily:                  map[string]Counters{},
		seenSessions:           map[string]struct{}{},
		agentStarts:            map[string]time.Time{},
		agentMeta:              map[string]agentMeta{},
		sessionFirstStart:      map[string]time.Time{},
		sessionAgentCount:      map[string]int64{},
		sessionStart:           map[string]time.Time{},
		sessionEnded:           map[string]bool{},
		lastActivityForSession: map[string]time.Time{},
		dailyDistinctProjects:  map[string]map[string]struct{}{},
		dailyDistinctSessions:  map[string]map[string]struct{}{},
	}
	if loaded, ok := loadCache(cfg.CachePath); ok {
		if loaded.RecordsParseFailed {
			// Cache-level corruption: the records field was present but
			// unparseable. Bump the degraded counter so the failure
			// surfaces alongside the bash-side ones in `wc -l` /
			// DegradedWritesTotal — opposite of the original silent
			// `_ = json.Unmarshal(...)` path.
			bumpDegraded(cfg.DegradedPath)
		}
		snap := loaded.Snap
		a.byType = copyInt64Map(snap.ByType)
		a.byProject = copyInt64Map(snap.ByProject)
		a.daily = copyDaily(snap.Daily)
		a.records = snap.Records
		a.firstSeen = snap.FirstSeen
		// Restore today's distinct sets only when the persisted date
		// matches today's UTC day. Stale today blocks (cache from
		// yesterday loaded today) are discarded — yesterday's count
		// already lives in its bucket.
		todayKey := dayKey(time.Now())
		if snap.TodayDistinct.Date == todayKey {
			a.currentDayKey = todayKey
			if len(snap.TodayDistinct.Projects) > 0 {
				set := make(map[string]struct{}, len(snap.TodayDistinct.Projects))
				for _, p := range snap.TodayDistinct.Projects {
					set[p] = struct{}{}
				}
				a.dailyDistinctProjects[todayKey] = set
			}
			if len(snap.TodayDistinct.Sessions) > 0 {
				set := make(map[string]struct{}, len(snap.TodayDistinct.Sessions))
				for _, s := range snap.TodayDistinct.Sessions {
					set[s] = struct{}{}
				}
				a.dailyDistinctSessions[todayKey] = set
			}
		}
		a.baseline = snap
	}
	return a
}

// DayKey is the exported alias for dayKey — consumers outside the
// package (CLI rendering, OTEL exporter) need the same UTC bucket
// convention without duplicating the format string.
func DayKey(t time.Time) string { return dayKey(t) }

// dayKey is the canonical UTC day-bucket key for the daily map.
// Centralized here so a future timezone-policy change touches one site.
func dayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// staleAgentCutoff bounds agentStarts/agentMeta growth from agents that
// crash or get killed without emitting agent.stop (kill -9, hook race,
// etc.). Mirrors the bash side's 24h cutoff in _compute_agent_duration.
const staleAgentCutoff = 24 * time.Hour

// sessionStartCutoff auto-closes sessions that received session.start
// but never matched a session.end (kill -9, OS reboot, hard crash).
// 8h sits well above any plausible interactive session and well below
// staleAgentCutoff, so it triggers before agent state is dropped.
// Computed at Snapshot read time, not at Fold, so a late session.end
// for the same sid still closes cleanly.
const sessionStartCutoff = 8 * time.Hour

// handleDayRollover freezes the previous day's distinct-set sizes
// into its bucket counters and clears the previous day's in-memory
// sets so they don't accumulate. Caller must hold a.mu.Lock().
// First-fold (no previous currentDayKey) just adopts the new day.
// Same-day folds are a no-op.
//
// Out-of-order JSONL replay: a stale event from a PAST day must NOT
// roll currentDayKey backwards or freeze today's freshest sets.
// Treat the past-day event as bucket-only — its count contribution
// already lands via the EventAgentStart bucket update, but the
// distinct-set semantics belong to currentDayKey alone. dayKey
// format is fixed-width zero-padded so lex compare equals
// chronological.
func (a *Aggregator) handleDayRollover(day string) {
	if a.currentDayKey == day {
		return
	}
	if a.currentDayKey != "" && day < a.currentDayKey {
		return
	}
	if a.currentDayKey != "" {
		prev := a.daily[a.currentDayKey]
		if set, ok := a.dailyDistinctProjects[a.currentDayKey]; ok {
			prev.DistinctProjectsDay = int64(len(set))
		}
		if set, ok := a.dailyDistinctSessions[a.currentDayKey]; ok {
			prev.DistinctSessionsDay = int64(len(set))
		}
		a.daily[a.currentDayKey] = prev
		delete(a.dailyDistinctProjects, a.currentDayKey)
		delete(a.dailyDistinctSessions, a.currentDayKey)
	}
	a.currentDayKey = day
}

// resolveAgentMeta returns the stored agentMeta for evt.AgentID,
// filling in any empty fields from evt itself. The stored meta is
// the authoritative source (populated at agent.start with full
// project/type/sid context); the event-side fallback covers stops
// that arrive without a matching start record (e.g., crashed agent
// state pruned, then a late stop replays).
func (a *Aggregator) resolveAgentMeta(evt collector.GateEvent) agentMeta {
	meta := a.agentMeta[evt.AgentID]
	if meta.AgentType == "" {
		meta.AgentType = evt.AgentType
	}
	if meta.Project == "" {
		meta.Project = evt.Project
	}
	if meta.SessionID == "" {
		meta.SessionID = evt.SessionID
	}
	return meta
}

// clampNonNeg returns v unchanged when non-negative; zero otherwise.
// Defensive against corrupt transcript parses that could surface
// negative tokens / tool-call counts (the bash awk parse extracts
// digits-only, so this should never fire — but cheap belt-and-
// suspenders against future emitter regressions).
func clampNonNeg(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

// clampDuration computes (end - start) in seconds, clamping a negative
// result to zero. NTP backstep across JSONL serialization can push a
// stop timestamp before its start in wall-clock terms; clamping keeps
// the record contract (non-negative, never panics) without dropping
// the event entirely.
func clampDuration(end, start time.Time) int64 {
	d := int64(end.Sub(start).Seconds())
	if d < 0 {
		return 0
	}
	return d
}

// pruneStale drops agentStarts/agentMeta entries older than the cutoff,
// and also bounds the lifecycle maps (sessionStart, sessionEnded,
// lastActivityForSession) so a long-lived thermo process doesn't
// accumulate unbounded per-session state across days/weeks. Caller
// must hold a.mu.Lock(). Cheap relative to Checkpoint's disk dance;
// runs once per checkpoint cycle.
func (a *Aggregator) pruneStale(now time.Time) {
	cutoff := now.Add(-staleAgentCutoff)
	for id, started := range a.agentStarts {
		if started.Before(cutoff) {
			delete(a.agentStarts, id)
			delete(a.agentMeta, id)
		}
	}
	for sid, last := range a.lastActivityForSession {
		if last.Before(cutoff) {
			delete(a.lastActivityForSession, sid)
			delete(a.sessionStart, sid)
			delete(a.sessionEnded, sid)
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
	if evt.SessionID != "" {
		// Last-activity tracking feeds the staleness sweep in
		// Snapshot — a session that started but never ended uses its
		// last observed activity as the implicit close timestamp.
		a.lastActivityForSession[evt.SessionID] = evt.Timestamp
	}

	day := dayKey(evt.Timestamp)
	bucket := a.daily[day]

	switch evt.Event {
	case collector.EventAgentStart:
		// Day-rollover: when this start lands on a new day, freeze the
		// previous day's distinct counts (final size already on the
		// bucket; this is belt-and-suspenders) and clear the previous
		// day's in-memory sets so they don't grow unbounded.
		a.handleDayRollover(day)

		bucket.AgentsStarted++
		if evt.AgentType != "" {
			a.byType[evt.AgentType]++
		}
		if evt.Project != "" {
			a.byProject[evt.Project]++
			projects := a.dailyDistinctProjects[day]
			if projects == nil {
				projects = map[string]struct{}{}
				a.dailyDistinctProjects[day] = projects
			}
			projects[evt.Project] = struct{}{}
			bucket.DistinctProjectsDay = int64(len(projects))
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
			c := a.sessionAgentCount[evt.SessionID]
			a.records.MostAgentsSession = a.records.MostAgentsSession.Insert(RecordEntry{
				Value:     c,
				SessionID: evt.SessionID,
				At:        evt.Timestamp,
			})
			sessions := a.dailyDistinctSessions[day]
			if sessions == nil {
				sessions = map[string]struct{}{}
				a.dailyDistinctSessions[day] = sessions
			}
			sessions[evt.SessionID] = struct{}{}
			bucket.DistinctSessionsDay = int64(len(sessions))
		}
		if n := int64(len(a.agentStarts)); n > bucket.PeakConcurrentDay {
			bucket.PeakConcurrentDay = n
		}
		a.records.PeakConcurrent = a.records.PeakConcurrent.Insert(RecordEntry{
			Value:     int64(len(a.agentStarts)),
			SessionID: evt.SessionID,
			At:        evt.Timestamp,
		})
		a.observeBurst(evt.Timestamp, evt.SessionID)

	case collector.EventAgentStop:
		// CC orphan-stop bug defense (#44971/#49671): empty agent_type
		// is the upstream signature. Bash drops these at the hook
		// before they enter the JSONL bus; this guard catches
		// in-flight lines from degraded-write fallback or pre-fix
		// historical events on replay.
		if evt.AgentType == "" {
			return
		}
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
		// Resolved once — used for LongestAgentS + the two telemetry
		// leaderboards. Stored meta wins; event fields fill in for
		// stops without a matching start record.
		meta := a.resolveAgentMeta(evt)
		// Per-agent telemetry roll-up. Negative values are defensive
		// against transcript-parse corruption.
		tokensIn := clampNonNeg(evt.TokensIn)
		tokensOut := clampNonNeg(evt.TokensOut)
		toolCalls := clampNonNeg(evt.ToolCallCount)
		bucket.TokensInTotal += tokensIn
		bucket.TokensOutTotal += tokensOut
		bucket.ToolCallsTotal += toolCalls
		// Zero values are excluded from the leaderboard so an idle
		// agent.stop doesn't poison rankings (locked decision §0).
		if total := tokensIn + tokensOut; total > 0 {
			a.records.MostTokensAgent = a.records.MostTokensAgent.Insert(RecordEntry{
				Value:     total,
				AgentID:   evt.AgentID,
				AgentType: meta.AgentType,
				Project:   meta.Project,
				SessionID: meta.SessionID,
				At:        evt.Timestamp,
			})
		}
		if toolCalls > 0 {
			a.records.MostToolCallsAgent = a.records.MostToolCallsAgent.Insert(RecordEntry{
				Value:     toolCalls,
				AgentID:   evt.AgentID,
				AgentType: meta.AgentType,
				Project:   meta.Project,
				SessionID: meta.SessionID,
				At:        evt.Timestamp,
			})
		}
		if matched {
			if start, ok := a.agentStarts[evt.AgentID]; ok {
				duration := clampDuration(evt.Timestamp, start)
				a.records.LongestAgentS = a.records.LongestAgentS.Insert(RecordEntry{
					Value:     duration,
					AgentID:   evt.AgentID,
					AgentType: meta.AgentType,
					Project:   meta.Project,
					SessionID: meta.SessionID,
					At:        evt.Timestamp,
				})
				delete(a.agentStarts, evt.AgentID)
				delete(a.agentMeta, evt.AgentID)
			}
		}
		// Inferred-from-first-agent path: only when no explicit
		// session.end has closed this sid yet. session.end takes
		// precedence and sets the record from real lifecycle math.
		if evt.SessionID != "" && !a.sessionEnded[evt.SessionID] {
			if first, ok := a.sessionFirstStart[evt.SessionID]; ok {
				duration := clampDuration(evt.Timestamp, first)
				a.records.LongestSessionS = a.records.LongestSessionS.Insert(RecordEntry{
					Value:     duration,
					SessionID: evt.SessionID,
					At:        evt.Timestamp,
				})
			}
		}

	case collector.EventGateCap:
		bucket.GateCapEvents++

	case collector.EventCounterReset:
		// No-op for stats — sessions are keyed on session_id, not on
		// counter epochs (locked decision §0).

	case collector.EventSessionStart:
		// Idempotent: keep the earliest start timestamp per sid.
		// session.end's lifecycle math reads from this map.
		if evt.SessionID == "" {
			return
		}
		if _, seen := a.sessionStart[evt.SessionID]; !seen {
			a.sessionStart[evt.SessionID] = evt.Timestamp
		}

	case collector.EventSessionEnd:
		if evt.SessionID == "" {
			return
		}
		// Lifecycle math: prefer (session.end - session.start) when
		// the matching session.start was seen; fall back to
		// (session.end - first agent.start) for sessions that pre-date
		// the lifecycle event rollout. Negative-duration clamp covers
		// NTP backstep on either path.
		var duration int64
		if start, ok := a.sessionStart[evt.SessionID]; ok {
			duration = clampDuration(evt.Timestamp, start)
		} else if first, ok := a.sessionFirstStart[evt.SessionID]; ok {
			duration = clampDuration(evt.Timestamp, first)
		}
		a.records.LongestSessionS = a.records.LongestSessionS.Insert(RecordEntry{
			Value:     duration,
			SessionID: evt.SessionID,
			At:        evt.Timestamp,
		})
		// Synthesize orphan accounting for any agents still active in
		// this session — the JSONL bus stays single-writer; we don't
		// fabricate agent.stop lines, we just count them as orphaned
		// at fold time.
		for id, meta := range a.agentMeta {
			if meta.SessionID != evt.SessionID {
				continue
			}
			bucket.AgentsOrphaned++
			delete(a.agentStarts, id)
			delete(a.agentMeta, id)
		}
		a.sessionEnded[evt.SessionID] = true

	case collector.EventCounterUnderflow:
		// Diagnostic-only at v1: bash already logged + emitted; the
		// aggregator records nothing user-visible. Fold returning
		// without mutation is intentional — see locked decision §0.
		return

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

	records := a.records
	// Staleness sweep: sessions with session.start but no session.end
	// after sessionStartCutoff (8h) are auto-closed using their last
	// observed activity as the implicit end. Computed on read so a
	// late session.end for the same sid still wins via Fold's
	// lifecycle math. Operates on a copy of records — a.records itself
	// is not mutated under the read lock.
	now := time.Now()
	for sid, start := range a.sessionStart {
		if a.sessionEnded[sid] {
			continue
		}
		if now.Sub(start) < sessionStartCutoff {
			continue
		}
		end, ok := a.lastActivityForSession[sid]
		if !ok {
			end = start
		}
		duration := clampDuration(end, start)
		records.LongestSessionS = records.LongestSessionS.Insert(RecordEntry{
			Value:     duration,
			SessionID: sid,
			At:        end,
		})
	}

	s := Snapshot{
		SchemaVersion: CurrentSchemaVersion,
		FirstSeen:     a.firstSeen,
		LastUpdated:   a.lastUpdated,
		Records:       records,
		ByType:        copyInt64Map(a.byType),
		ByProject:     copyInt64Map(a.byProject),
		Daily:         copyDaily(a.daily),
	}
	// Persist today's distinct sets so a thermo restart mid-day
	// doesn't send DistinctProjectsDay backwards. Older days are not
	// persisted — their integer count was frozen into the bucket at
	// the rollover moment.
	if a.currentDayKey != "" {
		s.TodayDistinct = TodayDistinctSets{
			Date:     a.currentDayKey,
			Projects: setKeys(a.dailyDistinctProjects[a.currentDayKey]),
			Sessions: setKeys(a.dailyDistinctSessions[a.currentDayKey]),
		}
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
	a.records.BiggestBurst = a.records.BiggestBurst.Insert(BurstRecord{
		Count:     count,
		WindowS:   burstWindowS,
		SessionID: sessionID,
		// At anchors on the FIRST start so a burst spanning midnight
		// gets day-attributed to its start day.
		At: a.burstWindow[0],
	})
}

// Window returns counters summed across the last N days, anchored on
// today's UTC date. Missing days count as zero. N=7/30/60/90 are the
// canonical scoreboard windows; "alltime" callers use Snapshot.Lifetime.
//
// Note: PeakConcurrentDay, DistinctProjectsDay, and DistinctSessionsDay
// are NOT semantically additive — they sum here for uniformity but
// the result is meaningless. Use BestDay* helpers for max-across-days.
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

func setKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
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
