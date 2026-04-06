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
	activeAgents map[string]bool

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
		activeAgents:   make(map[string]bool),
	}
}

// Update processes a new snapshot and recomputes all derived state.
func (s *AppState) Update(snap collector.Snapshot) {
	s.Current = &snap

	// Append to history — O(1) ring buffer push, no allocation
	s.History.Push(snap)

	// Compute spawn/death deltas using pre-allocated PID map
	s.computePIDDeltas(&snap)

	// Type counts — clear and repopulate in place
	clear(s.TypeCounts)
	for _, p := range snap.AllProcs {
		s.TypeCounts[p.TypeCode]++
	}
	if s.SmoothedCounts == nil {
		s.SmoothedCounts = make(map[string]float64)
	}
	smoothMap(s.TypeCounts, s.SmoothedCounts, config.CountSmoothAlpha)

	// Category counts — clear and repopulate in place
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
	smoothMapPerKey(s.CategoryCounts, s.SmoothedCats)

	s.SessionCount = len(snap.Sessions)

	// Network state
	if snap.Online {
		s.Online = true
		s.OfflineSince = time.Time{}
		s.OfflineDuration = 0
	} else {
		if s.Online || s.OfflineSince.IsZero() {
			// Just went offline
			s.OfflineSince = snap.Timestamp
		}
		s.Online = false
		s.OfflineDuration = snap.Timestamp.Sub(s.OfflineSince)
	}

	// Track online/offline per tick — O(1) ring buffer push
	s.OnlineLog.Push(snap.Online)

	// Headroom projection
	s.Headroom = EstimateHeadroom(
		s.TypeCounts,
		snap.System.MemUsedBytes,
		snap.System.MemTotalBytes,
	)

	// Threat level
	s.PrevThreat = s.ThreatLevel
	s.ThreatLevel = Classify(snap, s.SpawnRate)

	// Seed quip on first tick, then rotate on transitions
	if s.stableQuip == "" {
		s.stableQuip = ThreatQuip(s.ThreatLevel)
	}

	// Alert on threat transitions — pick a new random quip each time
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

	// Headroom alerts
	if s.Headroom.HeadroomBytes < config.HeadroomCritBytes*GB && s.Headroom.Warning != "" {
		// Only alert once per threshold crossing (check last alert)
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

	// Idle cycling (advances when no Claude sessions)
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
	case collector.EventGateDebounce:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "gate: " + ev.Command + " debounced",
			Level:   ThreatCool,
		})
	case collector.EventAgentStart:
		s.addAlert(AlertEntry{
			Time:    ev.Timestamp,
			Message: "agent " + ev.AgentType + " started (" + shortID(ev.AgentID) + ")",
			Level:   ThreatCool,
		})
		s.PluginActive = true
		s.activeAgents[ev.AgentID] = true
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

// AgentCount returns the number of live subagents (tracked via JSONL events).
func (s *AppState) AgentCount() int {
	return len(s.activeAgents)
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

// smoothMapPerKey applies EMA smoothing with per-category alpha:
// fixed categories (build, shell) use CountSmoothAlpha for snappy response,
// dynamic runtimes use RuntimeSmoothAlpha for slower decay so they linger visibly.
func smoothMapPerKey(raw map[string]int, smoothed map[string]float64) {
	for key, count := range raw {
		alpha := catAlpha(key)
		if prev, ok := smoothed[key]; ok {
			smoothed[key] = alpha*float64(count) + (1-alpha)*prev
		} else {
			smoothed[key] = float64(count)
		}
	}
	for key, prev := range smoothed {
		if _, ok := raw[key]; !ok {
			alpha := catAlpha(key)
			decayed := (1 - alpha) * prev
			if decayed < 0.5 {
				delete(smoothed, key)
			} else {
				smoothed[key] = decayed
			}
		}
	}
}

func catAlpha(name string) float64 {
	if collector.FixedCategories[name] {
		return config.CountSmoothAlpha
	}
	return config.RuntimeSmoothAlpha
}

// smoothMap applies EMA smoothing: moves existing keys toward current raw counts,
// seeds new keys at their raw value, and decays disappeared keys toward zero.
func smoothMap(raw map[string]int, smoothed map[string]float64, alpha float64) {
	for key, count := range raw {
		if prev, ok := smoothed[key]; ok {
			smoothed[key] = alpha*float64(count) + (1-alpha)*prev
		} else {
			smoothed[key] = float64(count)
		}
	}
	for key, prev := range smoothed {
		if _, ok := raw[key]; !ok {
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
