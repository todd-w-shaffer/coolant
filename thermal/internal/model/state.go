// Package model holds the dashboard's application state, threat
// classification, memory projection, and personality text.
package model

import (
	"strconv"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

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
	activeAgents map[string]time.Time // agent_id → start timestamp

	// Alerts
	Alerts *RingBuffer[AlertEntry]

	// Personality
	IdleCycle  int
	idleTicker int
	stableQuip string
}

// NewAppState creates an initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		History:        NewRingBuffer[collector.Snapshot](config.MaxHistory),
		OnlineLog:      NewRingBuffer[bool](config.MaxHistory),
		Alerts:         NewRingBuffer[AlertEntry](config.MaxAlerts),
		recentSpawns:   NewRingBuffer[int](config.RateWindowSize),
		recentDeaths:   NewRingBuffer[int](config.RateWindowSize),
		TypeCounts:     make(map[string]int),
		CategoryCounts: make(map[string]int),
		scratchPIDs:    make(map[int]bool),
		activeAgents:   make(map[string]time.Time),
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
}

// updateTypeCounts clears and repopulates type counts from the snapshot,
// then applies EMA smoothing for calm display.
func (s *AppState) updateTypeCounts(snap *collector.Snapshot) {
	clear(s.TypeCounts)
	for _, p := range snap.AllProcs {
		s.TypeCounts[p.TypeCode]++
	}
	if s.SmoothedCounts == nil {
		s.SmoothedCounts = make(map[string]float64)
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
	if s.SmoothedCats == nil {
		s.SmoothedCats = make(map[string]float64)
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
	s.ThreatLevel = Classify(*snap, s.SpawnRate)

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

	if s.Headroom.HeadroomBytes < config.HeadroomCritBytes && s.Headroom.Warning != "" {
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
	// Build current PID set in scratch map (clear + reuse)
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
		if spawns >= config.SpawnBurstThreshold {
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

// HandleEvent processes an external event from the JSONL event log
// and generates appropriate alerts.
func (s *AppState) HandleEvent(ev collector.GateEvent) {
	switch ev.Event {
	case collector.EventGateSuppress:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "gate: " + ev.Command + " suppressed (" + ev.Reason + ")",
			Level:   ThreatWarm,
		})
	case collector.EventGateCap:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "gate: " + ev.Command + " capped → " + ev.Rewritten,
			Level:   ThreatWarm,
		})
	case collector.EventAgentStart:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "agent " + ev.AgentType + " started (" + shortID(ev.AgentID) + ")",
			Level:   ThreatCool,
		})
		s.PluginActive = true
		s.activeAgents[ev.AgentID] = ev.Timestamp
	case collector.EventAgentStop:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "agent " + ev.AgentType + " stopped (" + shortID(ev.AgentID) + ")",
			Level:   ThreatCool,
		})
		delete(s.activeAgents, ev.AgentID)
	case collector.EventParallelEngaged:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "parallel mode engaged",
			Level:   ThreatHot,
		})
	case collector.EventParallelDisengaged:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "parallel mode disengaged",
			Level:   ThreatCool,
		})
	case collector.EventCounterReset:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "session reset",
			Level:   ThreatCool,
		})
	case collector.EventPreflightWarn:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "preflight: " + ev.Reason,
			Level:   ThreatWarm,
		})
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
	return len(s.activeAgents)
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

// agentCountSplit does a single pass over activeAgents with one time.Now() call.
func (s *AppState) agentCountSplit() (fresh, stale int) {
	cutoff := time.Now().Add(-config.AgentStaleThreshold)
	for _, started := range s.activeAgents {
		if started.After(cutoff) {
			fresh++
		} else {
			stale++
		}
	}
	return
}

// PurgeStaleAgents removes all agents that have exceeded the staleness threshold.
func (s *AppState) PurgeStaleAgents() {
	cutoff := time.Now().Add(-config.AgentStaleThreshold)
	for id, started := range s.activeAgents {
		if !started.After(cutoff) {
			delete(s.activeAgents, id)
		}
	}
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

func (s *AppState) addAlert(a AlertEntry) {
	s.Alerts.Push(a)
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
