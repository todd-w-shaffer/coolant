// Package model holds the dashboard's application state, threat
// classification, memory projection, and personality text.
package model

import (
	"math"
	"os"
	"strconv"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// AgentRecord captures the full lifecycle of a single subagent,
// populated incrementally: start-side fields on agent.start,
// stop-side fields on agent.stop, enrichments in Phase 2.
type AgentRecord struct {
	// Start-side (populated on agent.start)
	AgentID   string
	SessionID string
	AgentType string
	Project   string
	Cwd       string
	StartTime time.Time
	BurstID   int // Phase 2b: assigned on start, 0 until then

	// Stop-side (populated on agent.stop, zero if still active)
	StopTime       time.Time
	Duration       time.Duration
	PermissionMode string
	TranscriptPath string

	// Enrichments (Phase 2, zero-valued until populated)
	TranscriptBytes int64 // Phase 2c: from os.Stat on stop
	Orphan          bool  // Phase 2a: stop arrived with no matching start
	Purged          bool  // set when counter.reset evicts an active agent (never got a stop event)
}

// AlertEntry is a single alert with timestamp and severity.
type AlertEntry struct {
	Time    time.Time
	Message string
	Level   ThreatLevel // determines color
}

// AppState holds the rolling history, computed metrics, and UI state.
type AppState struct {
	Current  *collector.Snapshot
	History  *RingBuffer[collector.Snapshot]
	PrevPIDs map[int]bool

	// Computed
	ThreatLevel     ThreatLevel
	PrevThreat      ThreatLevel
	Headroom        HeadroomInfo
	SpawnRate       float64
	DeathRate       float64
	NetRate         float64
	TypeCounts      map[string]int
	SmoothedCounts  map[string]float64 // EMA-smoothed type counts for calm display
	CategoryCounts  map[string]int     // category → count (test, build, run, search, shell)
	SmoothedCats    map[string]float64 // EMA-smoothed category counts
	SessionCount    int
	PluginActive    bool
	UpdateAvailable bool
	Online          bool
	OnlineLog       *RingBuffer[bool] // tracks online/offline per tick, same length as History
	OfflineSince    time.Time         // when we went offline
	OfflineDuration time.Duration

	// Rate tracking
	recentSpawns *RingBuffer[int]
	recentDeaths *RingBuffer[int]

	// Pre-allocated scratch maps (cleared and reused each tick)
	scratchPIDs map[int]bool

	// Agent tracking (from JSONL events, not process discovery)
	activeRecords        map[string]*AgentRecord          // agent_id → in-progress record
	completedRecords     *RingBuffer[AgentRecord]         // capped completed agents
	totalCompleted       int                              // monotonic count — not capped by ring buffer
	epoch                time.Time                        // model init time — only count completions after this
	peakConcurrency      int                              // high-water mark of concurrent agents
	orphanStops          int                              // monotonic count of stops with no matching start
	burstCounter         int                              // monotonic within thermo lifetime, not reset on session reset
	lastStartTime        time.Time                        // timestamp of most recent agent.start (for burst gap detection)
	gateEvents           *RingBuffer[collector.GateEvent] // capped ring of gate.cap + gate.suppress events
	counterDrift         int                              // latest divergence: positive = bash > Go
	sessionEarliestStart time.Time                        // set on first agent.start, reset on counter.reset

	// Alerts
	Alerts *RingBuffer[AlertEntry]

	// Category filter (ephemeral inspection state — not persisted)
	CategoryFilter string

	// Personality
	IdleCycle  int
	idleTicker int
	stableQuip string

	// nil-safe — HandleEvent skips Fold when unset (test paths and
	// degraded init both run without it).
	aggregator *stats.Aggregator

	// Calm tracking — drives the idle animation freeze (see IsCalm). prevCalm
	// holds the previous snapshot's per-frame-animated signals (the five
	// sparkline sources) at display granularity; when they hold steady across
	// CalmStableSnapshots consecutive snapshots — and no agents are active and
	// the battery isn't alerting — the dashboard is "calm" and the caller stops
	// re-arming the animation tick.
	metricStableCount int
	prevCalm          [5]int64

	// displayCPU is the deadbanded CPU% shown by the rates readout and fed to
	// the CPU gauge spring + calm tracking. It holds steady through sub-band
	// jitter (see updateDisplayCPU / config.DisplayDeadbandPct) so the number
	// doesn't flicker and the dashboard can settle into calm. cpuSeeded forces
	// the first sample to commit so cold start shows the real value, not 0.
	displayCPU float64
	cpuSeeded  bool
}

// NewAppState creates an initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		History:          NewRingBuffer[collector.Snapshot](config.MaxHistory),
		OnlineLog:        NewRingBuffer[bool](config.MaxHistory),
		Alerts:           NewRingBuffer[AlertEntry](config.MaxAlerts),
		recentSpawns:     NewRingBuffer[int](config.RateWindowSize),
		recentDeaths:     NewRingBuffer[int](config.RateWindowSize),
		TypeCounts:       make(map[string]int),
		SmoothedCounts:   make(map[string]float64),
		CategoryCounts:   make(map[string]int),
		SmoothedCats:     make(map[string]float64),
		scratchPIDs:      make(map[int]bool),
		activeRecords:    make(map[string]*AgentRecord),
		completedRecords: NewRingBuffer[AgentRecord](config.MaxAgentRecords),
		gateEvents:       NewRingBuffer[collector.GateEvent](config.MaxGateEvents),
		epoch:            time.Now(),
	}
}

// Update processes a new snapshot and recomputes all derived state.
func (s *AppState) Update(snap collector.Snapshot) {
	s.Current = &snap
	s.History.Push(snap)
	s.computePIDDeltas(&snap)
	s.updateTypeCounts(&snap)
	s.updateCategoryCounts()
	s.SessionCount = len(snap.Sessions)
	s.updateNetworkState(&snap)
	s.Headroom = EstimateHeadroom(s.TypeCounts, snap.System.MemUsedBytes, snap.System.MemTotalBytes)
	s.updateThreatAndAlerts(&snap)
	s.updateIdleTicker()
	s.updateDisplayCPU()
	s.updateCalmTracking()
}

// updateDisplayCPU applies the CPU% display deadband: the shown value only
// moves when the raw sample is ≥ DisplayDeadbandPct from the currently displayed
// value. Comparing against the displayed (not the previous raw) value means a
// slow monotonic drift still accumulates past the band and commits, so the
// readout can't stall indefinitely — and a value bouncing within the band stays
// put, which is the flicker suppression we want. Runs before updateCalmTracking
// so the deadbanded value feeds the calm signal set.
func (s *AppState) updateDisplayCPU() {
	raw := s.Current.System.CPUPercent
	if !s.cpuSeeded || math.Abs(raw-s.displayCPU) >= config.DisplayDeadbandPct {
		s.displayCPU = raw
		s.cpuSeeded = true
	}
}

// DisplayCPUPercent is the deadbanded CPU% for the rates readout and CPU gauge.
func (s *AppState) DisplayCPUPercent() float64 { return s.displayCPU }

// calmSignals returns the per-frame-animated signals at display granularity:
// the five sparkline sources (widgets/gauges.go feeds CPU%, MEM%, decompression
// delta, and the two token-per-sec rates into the spring history that scrolls
// on every AnimTick). These are exactly the values whose sparklines lie if
// frozen mid-stream, so the freeze must not engage while any of them is moving.
// GPU and swap are deliberately excluded: they render as static text with no
// per-frame scroll, so the snapshot path repaints them fine even while frozen.
func (s *AppState) calmSignals() [5]int64 {
	sys := s.Current.System
	return [5]int64{
		int64(s.displayCPU), // deadbanded — sub-band CPU jitter doesn't reset calm
		int64(sys.MemPercent()),
		sys.Decompressions,
		int64(s.Current.Tokens.IOTokensPerSec),
		int64(s.Current.Tokens.PrettyTokensPerSec),
	}
}

// updateCalmTracking advances the consecutive-stable-snapshot counter that
// IsCalm reads. Signals are compared at display granularity, so sub-unit jitter
// that never changes the rendered output doesn't reset calm.
func (s *AppState) updateCalmTracking() {
	cur := s.calmSignals()
	if cur == s.prevCalm {
		s.metricStableCount++
	} else {
		s.metricStableCount = 0
	}
	s.prevCalm = cur
}

// IsCalm reports whether the dashboard is fully quiescent: no active agents,
// the five sparkline sources flat for CalmStableSnapshots consecutive
// snapshots, and the battery not in a low-charge alert pulse. When calm, the
// caller stops re-arming the animation tick, so breathing/sparkline motion
// freezes (still rendered) and terminal repaints drop toward zero. Any change
// wakes it.
func (s *AppState) IsCalm() bool {
	if s.Current == nil {
		return false
	}
	// Any active agent keeps a breathing/scanning dot on screen, so it can't be
	// calm. Stale agents are a subset of activeRecords, so AgentCount already
	// covers them — no separate StaleAgentCount walk (time.Now + map scan) on
	// this hot path, which runs every animation tick.
	if s.AgentCount() > 0 {
		return false
	}
	if s.batteryAlerting() {
		return false
	}
	return s.metricStableCount >= config.CalmStableSnapshots
}

// batteryAlerting is true while the battery is discharging below the critical
// threshold — the meltdown/warn pulse (widgets/battery.go) is an alert, not
// decoration, so calm must exclude it to keep that pulse animating.
func (s *AppState) batteryAlerting() bool {
	sys := s.Current.System
	return sys.BatteryPresent &&
		sys.BatteryState == collector.BatteryDischarging &&
		sys.BatteryPercent < config.BatteryCritPct
}

// updateTypeCounts clears and repopulates type counts from the snapshot,
// then applies EMA smoothing for calm display.
func (s *AppState) updateTypeCounts(snap *collector.Snapshot) {
	clear(s.TypeCounts)
	for _, p := range snap.AllProcs {
		s.TypeCounts[p.TypeCode]++
	}
	smoothEMA(s.TypeCounts, s.SmoothedCounts, countAlpha)
}

// updateCategoryCounts rolls up type counts into categories and applies EMA.
func (s *AppState) updateCategoryCounts() {
	clear(s.CategoryCounts)
	for typeCode, count := range s.TypeCounts {
		cat, ok := collector.TypeToCategory[typeCode]
		if !ok {
			cat = "shell"
		}
		s.CategoryCounts[cat] += count
	}
	smoothEMA(s.CategoryCounts, s.SmoothedCats, catAlpha)
}

// updateNetworkState tracks online/offline transitions and duration.
func (s *AppState) updateNetworkState(snap *collector.Snapshot) {
	if snap.Online {
		s.Online = true
		s.OfflineSince = time.Time{}
		s.OfflineDuration = 0
	} else {
		if s.Online || s.OfflineSince.IsZero() {
			s.OfflineSince = snap.Timestamp
		}
		s.Online = false
		s.OfflineDuration = snap.Timestamp.Sub(s.OfflineSince)
	}
	s.OnlineLog.Push(snap.Online)
}

// updateThreatAndAlerts classifies threat level, rotates quips, and fires
// alerts on threat transitions and headroom threshold crossings.
func (s *AppState) updateThreatAndAlerts(snap *collector.Snapshot) {
	s.PrevThreat = s.ThreatLevel
	s.ThreatLevel = Classify(snap, s.SpawnRate)

	if s.stableQuip == "" {
		s.stableQuip = ThreatQuip(s.ThreatLevel)
	}

	if s.ThreatLevel != s.PrevThreat {
		s.stableQuip = ThreatQuip(s.ThreatLevel)
		if s.PrevThreat != 0 {
			if s.ThreatLevel > s.PrevThreat {
				s.addAlert(AlertEntry{
					Time:    snap.Timestamp,
					Message: s.stableQuip,
					Level:   s.ThreatLevel,
				})
			} else {
				s.addAlert(AlertEntry{
					Time:    snap.Timestamp,
					Message: "cooling down -- " + s.stableQuip,
					Level:   s.ThreatLevel,
				})
			}
		}
	}

	if s.Headroom.HeadroomBytes < config.C.Headroom.CritBytes && s.Headroom.Warning != "" {
		lastMsg := ""
		if s.Alerts.Len() > 0 {
			lastMsg = s.Alerts.Peek().Message
		}
		if lastMsg != s.Headroom.Warning {
			s.addAlert(AlertEntry{
				Time:    snap.Timestamp,
				Message: s.Headroom.Warning,
				Level:   ThreatHot,
			})
		}
	}
}

// updateIdleTicker advances the idle cycle counter when no sessions are active.
func (s *AppState) updateIdleTicker() {
	if s.SessionCount == 0 {
		s.idleTicker++
		if s.idleTicker%config.IdleTickerModulo == 0 {
			s.IdleCycle++
		}
	} else {
		s.idleTicker = 0
	}
}

// computePIDDeltas calculates spawn/death counts using pre-allocated maps.
func (s *AppState) computePIDDeltas(snap *collector.Snapshot) {
	clear(s.scratchPIDs)
	for _, p := range snap.AllProcs {
		s.scratchPIDs[p.PID] = true
	}

	if s.PrevPIDs != nil {
		spawns := 0
		deaths := 0
		for pid := range s.scratchPIDs {
			if !s.PrevPIDs[pid] {
				spawns++
			}
		}
		for pid := range s.PrevPIDs {
			if !s.scratchPIDs[pid] {
				deaths++
			}
		}
		s.recentSpawns.Push(spawns)
		s.recentDeaths.Push(deaths)
		s.SpawnRate = smoothedRateRing(s.recentSpawns)
		s.DeathRate = smoothedRateRing(s.recentDeaths)
		s.NetRate = s.SpawnRate - s.DeathRate

		// Alert on spawn bursts
		if spawns >= config.C.Spawn.BurstThreshold {
			s.addAlert(AlertEntry{
				Time:    snap.Timestamp,
				Message: "spawn burst -- " + strconv.Itoa(spawns) + " new procs",
				Level:   ThreatHot,
			})
		}
	}

	// Swap scratch into PrevPIDs, reuse the old PrevPIDs as next scratch
	s.scratchPIDs, s.PrevPIDs = s.PrevPIDs, s.scratchPIDs
	// If PrevPIDs was nil (first tick), allocate a new scratch
	if s.scratchPIDs == nil {
		s.scratchPIDs = make(map[int]bool)
	}
}

// eventAlertSpec defines the message format and threat level for a JSONL event.
type eventAlertSpec struct {
	msg   func(collector.GateEvent) string
	level ThreatLevel
}

var eventAlerts = map[string]eventAlertSpec{
	collector.EventGateSuppress: {func(ev collector.GateEvent) string {
		return "blocked: " + ev.Command + " (parallel mode — /coolant to release)"
	}, ThreatWarm},
	collector.EventGateCap: {func(ev collector.GateEvent) string { return "throttled: " + ev.Command + " → " + ev.Rewritten }, ThreatWarm},
	collector.EventAgentStart: {func(ev collector.GateEvent) string {
		return "agent " + ev.AgentType + " started (" + shortID(ev.AgentID) + ")"
	}, ThreatCool},
	collector.EventAgentStop: {func(ev collector.GateEvent) string {
		return "agent " + ev.AgentType + " stopped (" + shortID(ev.AgentID) + ")"
	}, ThreatCool},
	collector.EventParallelEngaged:    {func(ev collector.GateEvent) string { return "parallel mode engaged" }, ThreatHot},
	collector.EventParallelDisengaged: {func(ev collector.GateEvent) string { return "parallel mode disengaged" }, ThreatCool},
	collector.EventCounterReset:       {func(ev collector.GateEvent) string { return "session reset" }, ThreatCool},
	collector.EventPreflightWarn:      {func(ev collector.GateEvent) string { return "preflight: " + ev.Reason }, ThreatWarm},
}

func (s *AppState) AttachAggregator(a *stats.Aggregator) {
	s.aggregator = a
}

func (s *AppState) Aggregator() *stats.Aggregator {
	return s.aggregator
}

// HandleEvent processes an external event from the JSONL event log
// and generates appropriate alerts.
func (s *AppState) HandleEvent(ev collector.GateEvent) {
	// Any event that reaches HandleEvent arrived via the plugin's
	// JSONL bus, so its existence proves the plugin is installed.
	// Previously only agent.start flipped this, which made the
	// "install plugin" banner show in subagent-free sessions.
	s.PluginActive = true

	if s.aggregator != nil {
		s.aggregator.Fold(ev, 0)
	}
	if spec, ok := eventAlerts[ev.Event]; ok {
		s.addAlert(AlertEntry{Time: ev.Timestamp, Message: spec.msg(ev), Level: spec.level})
	}

	switch ev.Event {
	case collector.EventAgentStart:
		if s.sessionEarliestStart.IsZero() {
			s.sessionEarliestStart = ev.Timestamp
		}
		if !s.lastStartTime.IsZero() && ev.Timestamp.Sub(s.lastStartTime) > config.BurstGapThreshold {
			s.burstCounter++
		}
		s.lastStartTime = ev.Timestamp
		s.activeRecords[ev.AgentID] = &AgentRecord{
			AgentID:   ev.AgentID,
			SessionID: ev.SessionID,
			AgentType: ev.AgentType,
			Project:   ev.Project,
			Cwd:       ev.Cwd,
			StartTime: ev.Timestamp,
			BurstID:   s.burstCounter,
		}
		if current := len(s.activeRecords); current > s.peakConcurrency {
			s.peakConcurrency = current
		}
		s.counterDrift = ev.AgentCount - len(s.activeRecords)
	case collector.EventAgentStop:
		if rec, tracked := s.activeRecords[ev.AgentID]; tracked {
			rec.StopTime = ev.Timestamp
			rec.Duration = ev.Timestamp.Sub(rec.StartTime)
			rec.PermissionMode = ev.PermissionMode
			rec.TranscriptPath = ev.TranscriptPath
			rec.TranscriptBytes = statTranscript(rec.TranscriptPath)
			if !ev.Timestamp.Before(s.epoch) {
				s.completedRecords.Push(*rec)
				s.totalCompleted++
			}
			delete(s.activeRecords, ev.AgentID)
		} else {
			orphan := AgentRecord{
				AgentID:        ev.AgentID,
				SessionID:      ev.SessionID,
				AgentType:      ev.AgentType,
				Project:        ev.Project,
				Cwd:            ev.Cwd,
				StopTime:       ev.Timestamp,
				PermissionMode: ev.PermissionMode,
				TranscriptPath: ev.TranscriptPath,
				Orphan:         true,
			}
			orphan.TranscriptBytes = statTranscript(orphan.TranscriptPath)
			if !ev.Timestamp.Before(s.epoch) {
				s.completedRecords.Push(orphan)
				s.totalCompleted++
				s.orphanStops++
			}
		}
		s.counterDrift = ev.AgentCount - len(s.activeRecords)
	case collector.EventGateCap, collector.EventGateSuppress:
		s.gateEvents.Push(ev)
	case collector.EventCounterReset:
		// New session — move all active records to completed as purged.
		// Does NOT increment totalCompleted: purged records reflect an
		// interrupted lifecycle (no clean stop event), not earned completions.
		for id, rec := range s.activeRecords {
			rec.Purged = true
			rec.StopTime = ev.Timestamp
			s.completedRecords.Push(*rec)
			delete(s.activeRecords, id)
		}
		s.peakConcurrency = 0
		s.sessionEarliestStart = time.Time{}
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// StableQuip returns the current personality quip (doesn't change every tick).
func (s *AppState) StableQuip() string {
	if s.SessionCount == 0 {
		return IdleMessage(s.IdleCycle)
	}
	return s.stableQuip
}

// AgentCount returns the total number of tracked subagents (fresh + stale).
func (s *AppState) AgentCount() int {
	return len(s.activeRecords)
}

// FreshAgentCount returns agents started within the staleness threshold.
func (s *AppState) FreshAgentCount() int {
	fresh, _ := s.agentCountSplit()
	return fresh
}

// StaleAgentCount returns agents active longer than the staleness threshold.
func (s *AppState) StaleAgentCount() int {
	_, stale := s.agentCountSplit()
	return stale
}

// CompletedAgentCount returns the monotonic count of agents that received
// a confirmed stop event. Not capped by the ring buffer — this is the
// counter KITT highscore reads.
func (s *AppState) CompletedAgentCount() int {
	return s.totalCompleted
}

// agentCountSplit does a single pass over activeRecords with one time.Now() call.
func (s *AppState) agentCountSplit() (fresh, stale int) {
	cutoff := time.Now().Add(-config.AgentStaleThreshold)
	for _, rec := range s.activeRecords {
		if rec.StartTime.After(cutoff) {
			fresh++
		} else {
			stale++
		}
	}
	return
}

// PeakConcurrency returns the high-water mark of concurrent active agents.
// Reset on counter.reset (per-session metric).
func (s *AppState) PeakConcurrency() int {
	return s.peakConcurrency
}

// OrphanStopCount returns the monotonic count of stop events that had
// no matching start (likely pre-epoch agents or hook race conditions).
func (s *AppState) OrphanStopCount() int {
	return s.orphanStops
}

// CounterDrift returns the latest divergence between the bash counter
// file and Go's agent tracking. Positive = bash thinks more agents
// exist than Go does.
func (s *AppState) CounterDrift() int {
	return s.counterDrift
}

// GateEvents returns a slice of gate.cap + gate.suppress events from the ring buffer.
func (s *AppState) GateEvents() []collector.GateEvent {
	return s.gateEvents.Slice()
}

// GateCapCount returns the number of gate.cap events in the ring buffer.
func (s *AppState) GateCapCount() int { return s.gateEventCount(collector.EventGateCap) }

// GateSuppressCount returns the number of gate.suppress events in the ring buffer.
func (s *AppState) GateSuppressCount() int { return s.gateEventCount(collector.EventGateSuppress) }

func (s *AppState) gateEventCount(kind string) int {
	count := 0
	for i := 0; i < s.gateEvents.Len(); i++ {
		if s.gateEvents.At(i).Event == kind {
			count++
		}
	}
	return count
}

// ActiveRecords returns a copy of all in-progress agent records.
func (s *AppState) ActiveRecords() []AgentRecord {
	if len(s.activeRecords) == 0 {
		return nil
	}
	out := make([]AgentRecord, 0, len(s.activeRecords))
	for _, rec := range s.activeRecords {
		out = append(out, *rec)
	}
	return out
}

// CompletedRecords returns a slice of completed agent records from the ring buffer.
func (s *AppState) CompletedRecords() []AgentRecord {
	return s.completedRecords.Slice()
}

// LookupAgent returns a single record by ID, searching active then completed.
// Returns nil if not found. The returned pointer is to a copy — callers
// cannot mutate the ring buffer or active map through it.
func (s *AppState) LookupAgent(id string) *AgentRecord {
	if rec, ok := s.activeRecords[id]; ok {
		cp := *rec
		return &cp
	}
	// Linear scan completed records newest-first. 500-element scan is <1μs.
	for i := s.completedRecords.Len() - 1; i >= 0; i-- {
		rec := s.completedRecords.At(i)
		if rec.AgentID == id {
			return &rec
		}
	}
	return nil
}

// ClearCompletedRecords resets the agent scoreboard: empties the completed
// ring buffer and zeroes the totalCompleted + orphanStops counters that feed
// the KITT highscore display. Active records and session metadata
// (peakConcurrency, sessionEarliestStart, burstCounter) are intentionally
// untouched — those track live work, not scoreboard history.
func (s *AppState) ClearCompletedRecords() {
	s.completedRecords.Clear()
	s.totalCompleted = 0
	s.orphanStops = 0
}

// SetCategoryFilter sets the active category filter. Empty string clears.
func (s *AppState) SetCategoryFilter(name string) {
	s.CategoryFilter = name
}

// ToggleCategoryFilter toggles filtering: if name matches the current
// filter, clears it; otherwise sets it to name.
func (s *AppState) ToggleCategoryFilter(name string) {
	if s.CategoryFilter == name {
		s.CategoryFilter = ""
	} else {
		s.CategoryFilter = name
	}
}

// CycleCategoryFilter cycles through visible categories. +1 forward,
// -1 backward. The cycle is none → A → B → … → Z → none. direction == 0
// is a no-op.
func (s *AppState) CycleCategoryFilter(direction int) {
	if direction == 0 {
		return
	}
	visible := VisibleCategoryNames(s.SmoothedCats)
	if len(visible) == 0 {
		s.CategoryFilter = ""
		return
	}

	// Find current index (-1 when no filter active)
	cur := -1
	for i, name := range visible {
		if name == s.CategoryFilter {
			cur = i
			break
		}
	}

	next := cur + direction
	if next < -1 {
		next = len(visible) - 1 // backward past none → last
	}
	if next >= len(visible) {
		next = -1 // forward past last → none
	}

	if next == -1 {
		s.CategoryFilter = ""
	} else {
		s.CategoryFilter = visible[next]
	}
}

// PushAlert adds an alert to the ring buffer (public wrapper).
func (s *AppState) PushAlert(a AlertEntry) {
	s.addAlert(a)
}

// SessionUptime returns the duration since the first agent.start in this
// session. Returns 0 if no agents have started (or after a counter.reset
// with no new agents). Reads sessionEarliestStart directly — does not
// scan the ring buffer (which evicts old entries at 500).
func (s *AppState) SessionUptime() time.Duration {
	if s.sessionEarliestStart.IsZero() {
		return 0
	}
	return time.Since(s.sessionEarliestStart)
}

// TotalTranscriptBytes returns the sum of TranscriptBytes across completed
// records. Active records have 0 (populated on agent.stop), so only
// completed records contribute.
func (s *AppState) TotalTranscriptBytes() int64 {
	var total int64
	for i := 0; i < s.completedRecords.Len(); i++ {
		total += s.completedRecords.At(i).TranscriptBytes
	}
	return total
}

// AgentTypeCounts returns a type breakdown across completed + active records.
// The returned map is unordered — callers handle display sorting.
func (s *AppState) AgentTypeCounts() map[string]int {
	counts := make(map[string]int)
	for i := 0; i < s.completedRecords.Len(); i++ {
		counts[s.completedRecords.At(i).AgentType]++
	}
	for _, rec := range s.activeRecords {
		counts[rec.AgentType]++
	}
	return counts
}

// IsIdle returns true when no Claude sessions are detected.
func (s *AppState) IsIdle() bool {
	return s.SessionCount == 0
}

// LastSpawns returns the most recent raw spawn count.
func (s *AppState) LastSpawns() int {
	if s.recentSpawns.Len() == 0 {
		return 0
	}
	return s.recentSpawns.Peek()
}

// LastDeaths returns the most recent raw death count.
func (s *AppState) LastDeaths() int {
	if s.recentDeaths.Len() == 0 {
		return 0
	}
	return s.recentDeaths.Peek()
}

// CompositeHeat returns the weighted CPU/MEM/SWAP+spawn pressure scalar
// in [0.0, 1.0]. The HeatBloom widget consumes this as its heat target;
// it is the same underlying score Classify buckets into threat levels.
func (s *AppState) CompositeHeat() float64 {
	if s == nil || s.Current == nil {
		return 0
	}
	return CompositeHeatFor(s.Current, s.SpawnRate)
}

func (s *AppState) addAlert(a AlertEntry) {
	s.Alerts.Push(a)
}

// statTranscript returns the file size for a local transcript path,
// or 0 if the path is empty or the file doesn't exist.
func statTranscript(path string) int64 {
	if path == "" {
		return 0
	}
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// VisibleCategoryNames returns category names that should appear in the
// headline: fixed categories always, dynamic runtimes when smoothed >= 0.5.
// Order follows collector.Categories. Lives in model (not widgets) to avoid
// an import cycle — widgets imports model, so model cannot import widgets.
func VisibleCategoryNames(smoothed map[string]float64) []string {
	var names []string
	for _, cat := range collector.Categories {
		if collector.FixedCategories[cat.Name] || smoothed[cat.Name] >= 0.5 {
			names = append(names, cat.Name)
		}
	}
	return names
}

func countAlpha(string) float64 { return config.CountSmoothAlpha }

func catAlpha(name string) float64 {
	if collector.FixedCategories[name] {
		return config.CountSmoothAlpha
	}
	return config.RuntimeSmoothAlpha
}

// smoothEMA applies EMA smoothing: moves existing keys toward current raw counts,
// seeds new keys at their raw value, and decays disappeared keys toward zero.
// alphaFn returns the per-key alpha (use a constant func for uniform smoothing).
func smoothEMA(raw map[string]int, smoothed map[string]float64, alphaFn func(string) float64) {
	for key, count := range raw {
		alpha := alphaFn(key)
		if prev, ok := smoothed[key]; ok {
			smoothed[key] = alpha*float64(count) + (1-alpha)*prev
		} else {
			smoothed[key] = float64(count)
		}
	}
	for key, prev := range smoothed {
		if _, ok := raw[key]; !ok {
			alpha := alphaFn(key)
			decayed := (1 - alpha) * prev
			if decayed < 0.5 {
				delete(smoothed, key)
			} else {
				smoothed[key] = decayed
			}
		}
	}
}

// smoothedRateRing computes EMA over ring buffer contents.
func smoothedRateRing(r *RingBuffer[int]) float64 {
	n := r.Len()
	if n == 0 {
		return 0
	}
	alpha := config.RateSmoothAlpha
	ema := float64(r.At(0))
	for i := 1; i < n; i++ {
		ema = alpha*float64(r.At(i)) + (1-alpha)*ema
	}
	return ema
}
