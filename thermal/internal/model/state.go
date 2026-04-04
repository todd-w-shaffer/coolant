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
	History  []collector.Snapshot
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
	OnlineLog       []bool    // tracks online/offline per tick, same length as History
	OfflineSince    time.Time // when we went offline
	OfflineDuration time.Duration

	// Rate tracking
	recentSpawns []int
	recentDeaths []int

	// Alerts
	Alerts    []AlertEntry
	maxAlerts int

	// Personality
	IdleCycle  int
	idleTicker int
	stableQuip string
}

// NewAppState creates an initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		maxAlerts: config.MaxAlerts,
	}
}

// Update processes a new snapshot and recomputes all derived state.
func (s *AppState) Update(snap collector.Snapshot) {
	s.Current = &snap

	// Append to history — copy to new slice to release the old backing array
	s.History = append(s.History, snap)
	if len(s.History) > config.MaxHistory {
		trimmed := make([]collector.Snapshot, config.MaxHistory)
		copy(trimmed, s.History[len(s.History)-config.MaxHistory:])
		s.History = trimmed
	}

	// Compute spawn/death deltas
	currPIDs := snap.PIDs()
	if s.PrevPIDs != nil {
		spawns := 0
		deaths := 0
		for pid := range currPIDs {
			if !s.PrevPIDs[pid] {
				spawns++
			}
		}
		for pid := range s.PrevPIDs {
			if !currPIDs[pid] {
				deaths++
			}
		}
		s.recentSpawns = append(s.recentSpawns, spawns)
		s.recentDeaths = append(s.recentDeaths, deaths)
		if len(s.recentSpawns) > config.RateWindowSize {
			s.recentSpawns = s.recentSpawns[len(s.recentSpawns)-config.RateWindowSize:]
			s.recentDeaths = s.recentDeaths[len(s.recentDeaths)-config.RateWindowSize:]
		}
		s.SpawnRate = smoothedRate(s.recentSpawns)
		s.DeathRate = smoothedRate(s.recentDeaths)
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
	s.PrevPIDs = currPIDs

	// Type counts — raw and smoothed
	s.TypeCounts = snap.TypeCounts()
	if s.SmoothedCounts == nil {
		s.SmoothedCounts = make(map[string]float64)
	}
	smoothMap(s.TypeCounts, s.SmoothedCounts, config.CountSmoothAlpha)

	// Category counts — raw and smoothed
	s.CategoryCounts = collector.CategoryCounts(s.TypeCounts)
	if s.SmoothedCats == nil {
		s.SmoothedCats = make(map[string]float64)
	}
	smoothMap(s.CategoryCounts, s.SmoothedCats, config.CountSmoothAlpha)

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

	// Track online/offline per tick — copy to release old backing array
	s.OnlineLog = append(s.OnlineLog, snap.Online)
	if len(s.OnlineLog) > config.MaxHistory {
		trimmed := make([]bool, config.MaxHistory)
		copy(trimmed, s.OnlineLog[len(s.OnlineLog)-config.MaxHistory:])
		s.OnlineLog = trimmed
	}

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
		if len(s.Alerts) == 0 || s.Alerts[len(s.Alerts)-1].Message != s.Headroom.Warning {
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

// StableQuip returns the current personality quip (doesn't change every tick).
func (s *AppState) StableQuip() string {
	if s.SessionCount == 0 {
		return IdleMessage(s.IdleCycle)
	}
	return s.stableQuip
}

// IsIdle returns true when no Claude sessions are detected.
func (s *AppState) IsIdle() bool {
	return s.SessionCount == 0
}

// LastSpawns returns the most recent raw spawn count.
func (s *AppState) LastSpawns() int {
	if len(s.recentSpawns) == 0 {
		return 0
	}
	return s.recentSpawns[len(s.recentSpawns)-1]
}

// LastDeaths returns the most recent raw death count.
func (s *AppState) LastDeaths() int {
	if len(s.recentDeaths) == 0 {
		return 0
	}
	return s.recentDeaths[len(s.recentDeaths)-1]
}

func (s *AppState) addAlert(a AlertEntry) {
	s.Alerts = append(s.Alerts, a)
	if len(s.Alerts) > s.maxAlerts {
		trimmed := make([]AlertEntry, s.maxAlerts)
		copy(trimmed, s.Alerts[len(s.Alerts)-s.maxAlerts:])
		s.Alerts = trimmed
	}
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

// smoothedRate computes exponentially weighted moving average of recent values.
func smoothedRate(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	alpha := config.RateSmoothAlpha
	ema := float64(vals[0])
	for _, v := range vals[1:] {
		ema = alpha*float64(v) + (1-alpha)*ema
	}
	return ema
}
