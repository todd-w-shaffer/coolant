package collector

import (
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// slowSchedule decides, on each 1s base tick of the slow loop, which of the
// adaptive subprocess-heavy sources (network, GPU, battery) are due to run.
// The base sources (swap/vm_stat, tokens, procs) run every tick; these three
// poll on their own longer cadence so an idle laptop forks ioreg/pmset/net far
// less often. Zero last-run times make a source due immediately (startup).
type slowSchedule struct {
	lastNet  time.Time
	lastGPU  time.Time
	lastBatt time.Time
	// gpuInterval is re-derived from each GPU reading: short while load is
	// present, long while flat. Tracked here because the next due-check needs it.
	gpuInterval time.Duration
}

func newSlowSchedule() slowSchedule {
	return slowSchedule{gpuInterval: config.GPUActiveInterval}
}

// due reports which adaptive sources should run at now, given the last-known
// online state (which sets the network cadence).
func (s *slowSchedule) due(now time.Time, online bool) (net, gpu, batt bool) {
	net = now.Sub(s.lastNet) >= netCadence(online)
	gpu = now.Sub(s.lastGPU) >= s.gpuInterval
	batt = now.Sub(s.lastBatt) >= config.BatteryInterval
	return
}

func (s *slowSchedule) markNet(now time.Time)     { s.lastNet = now }
func (s *slowSchedule) markBattery(now time.Time) { s.lastBatt = now }

// markGPU records a GPU read and re-derives the next cadence from its value.
func (s *slowSchedule) markGPU(now time.Time, reading float64) {
	s.lastGPU = now
	s.gpuInterval = gpuCadence(reading)
}
