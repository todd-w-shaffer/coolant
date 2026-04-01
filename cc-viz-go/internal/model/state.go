package model

import (
	"time"

	"github.com/toddwshaffer/coolant/cc-viz-go/internal/collector"
)

const maxHistory = 300 // ~5 min at 1Hz

// AlertEntry is a single alert with timestamp and severity.
type AlertEntry struct {
	Time    time.Time
	Message string
	Level   ThreatLevel // determines color
}

// AppState holds the rolling history, computed metrics, and UI state.
type AppState struct {
	Current      *collector.Snapshot
	History      []collector.Snapshot
	PrevPIDs     map[int]bool

	// Computed
	ThreatLevel  ThreatLevel
	PrevThreat   ThreatLevel
	Headroom     HeadroomInfo
	SpawnRate    float64
	DeathRate    float64
	NetRate      float64
	TypeCounts       map[string]int
	SmoothedCounts   map[string]float64 // EMA-smoothed type counts for calm display
	SessionCount     int
	PluginActive     bool

	// Rate tracking
	recentSpawns []int
	recentDeaths []int

	// Alerts
	Alerts       []AlertEntry
	maxAlerts    int

	// Personality
	IdleCycle    int
	idleTicker   int
	stableQuip   string
}

// NewAppState creates an initialized AppState.
func NewAppState() *AppState {
	return &AppState{
		maxAlerts: 100,
	}
}

// Update processes a new snapshot and recomputes all derived state.
func (s *AppState) Update(snap collector.Snapshot) {
	s.Current = &snap

	// Append to history ring buffer
	s.History = append(s.History, snap)
	if len(s.History) > maxHistory {
		s.History = s.History[len(s.History)-maxHistory:]
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
		if len(s.recentSpawns) > 10 {
			s.recentSpawns = s.recentSpawns[len(s.recentSpawns)-10:]
			s.recentDeaths = s.recentDeaths[len(s.recentDeaths)-10:]
		}
		s.SpawnRate = smoothedRate(s.recentSpawns)
		s.DeathRate = smoothedRate(s.recentDeaths)
		s.NetRate = s.SpawnRate - s.DeathRate

		// Alert on spawn bursts
		if spawns >= 8 {
			s.addAlert(AlertEntry{
				Time:    snap.Timestamp,
				Message: "spawn burst -- " + string(rune('0'+spawns)) + " new procs",
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
	alpha := 0.15 // low alpha = more smoothing, less jitter
	// Smooth existing types toward current value
	for code, count := range s.TypeCounts {
		if prev, ok := s.SmoothedCounts[code]; ok {
			s.SmoothedCounts[code] = alpha*float64(count) + (1-alpha)*prev
		} else {
			s.SmoothedCounts[code] = float64(count)
		}
	}
	// Decay types that disappeared
	for code, prev := range s.SmoothedCounts {
		if _, ok := s.TypeCounts[code]; !ok {
			decayed := (1 - alpha) * prev
			if decayed < 0.5 {
				delete(s.SmoothedCounts, code)
			} else {
				s.SmoothedCounts[code] = decayed
			}
		}
	}
	s.SessionCount = len(snap.Sessions)

	// Headroom projection
	s.Headroom = EstimateHeadroom(
		s.TypeCounts,
		snap.System.MemUsedBytes,
		snap.System.MemTotalBytes,
	)

	// Threat level
	s.PrevThreat = s.ThreatLevel
	s.ThreatLevel = Classify(snap, s.SpawnRate)

	// Alert on threat transitions
	if s.ThreatLevel != s.PrevThreat && s.PrevThreat != 0 {
		if s.ThreatLevel > s.PrevThreat {
			s.addAlert(AlertEntry{
				Time:    snap.Timestamp,
				Message: ThreatQuipStable(s.ThreatLevel),
				Level:   s.ThreatLevel,
			})
		} else {
			s.addAlert(AlertEntry{
				Time:    snap.Timestamp,
				Message: "cooling down -- " + ThreatQuipStable(s.ThreatLevel),
				Level:   s.ThreatLevel,
			})
		}
	}
	// Always keep quip in sync with current threat level
	s.stableQuip = ThreatQuipStable(s.ThreatLevel)

	// Headroom alerts
	if s.Headroom.HeadroomBytes < 2*GB && s.Headroom.Warning != "" {
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
		if s.idleTicker%8 == 0 { // change message every ~8 ticks
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

// CPUHistory returns the CPU% history as float64 slice for sparklines.
func (s *AppState) CPUHistory() []float64 {
	out := make([]float64, len(s.History))
	for i, h := range s.History {
		out[i] = h.System.CPUPercent
	}
	return out
}

// MemHistory returns the memory% history.
func (s *AppState) MemHistory() []float64 {
	out := make([]float64, len(s.History))
	for i, h := range s.History {
		out[i] = h.System.MemPercent()
	}
	return out
}

// SwapHistory returns the swap% history.
func (s *AppState) SwapHistory() []float64 {
	out := make([]float64, len(s.History))
	for i, h := range s.History {
		out[i] = h.System.SwapPercent()
	}
	return out
}

// ProcCountHistory returns the total Claude process count history.
func (s *AppState) ProcCountHistory() []float64 {
	out := make([]float64, len(s.History))
	for i, h := range s.History {
		out[i] = float64(h.TotalProcs())
	}
	return out
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
		s.Alerts = s.Alerts[len(s.Alerts)-s.maxAlerts:]
	}
}

// smoothedRate computes exponentially weighted moving average of recent values.
func smoothedRate(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	alpha := 0.3
	ema := float64(vals[0])
	for _, v := range vals[1:] {
		ema = alpha*float64(v) + (1-alpha)*ema
	}
	return ema
}
