package collector

import (
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

func TestGPUCadenceBacksOffWhenFlat(t *testing.T) {
	if got := gpuCadence(0); got != config.GPUIdleInterval {
		t.Errorf("gpuCadence(0) = %v, want idle %v", got, config.GPUIdleInterval)
	}
	if got := gpuCadence(config.GPUFlatThreshold - 0.1); got != config.GPUIdleInterval {
		t.Errorf("gpuCadence(below threshold) = %v, want idle %v", got, config.GPUIdleInterval)
	}
	if got := gpuCadence(config.GPUFlatThreshold); got != config.GPUActiveInterval {
		t.Errorf("gpuCadence(at threshold) = %v, want active %v", got, config.GPUActiveInterval)
	}
	if got := gpuCadence(90); got != config.GPUActiveInterval {
		t.Errorf("gpuCadence(90) = %v, want active %v", got, config.GPUActiveInterval)
	}
}

func TestNetCadenceFastWhenOffline(t *testing.T) {
	if got := netCadence(true); got != config.NetOnlineInterval {
		t.Errorf("netCadence(online) = %v, want %v", got, config.NetOnlineInterval)
	}
	if got := netCadence(false); got != config.NetFailInterval {
		t.Errorf("netCadence(offline) = %v, want %v", got, config.NetFailInterval)
	}
}

func TestSlowScheduleAllDueAtStartup(t *testing.T) {
	s := newSlowSchedule()
	net, gpu, batt := s.due(time.Now(), true)
	if !net || !gpu || !batt {
		t.Errorf("startup (zero last-run) should make all sources due: net=%v gpu=%v batt=%v", net, gpu, batt)
	}
}

func TestSlowScheduleNoneDueRightAfterRun(t *testing.T) {
	s := newSlowSchedule()
	t0 := time.Now()
	s.markNet(t0)
	s.markGPU(t0, 0) // flat → idle cadence
	s.markBattery(t0)

	// One base tick later (1s): nothing is due (all cadences are >= 12s/15s/20s).
	net, gpu, batt := s.due(t0.Add(config.SlowInterval), true)
	if net || gpu || batt {
		t.Errorf("1s after a run nothing should be due: net=%v gpu=%v batt=%v", net, gpu, batt)
	}
}

func TestSlowScheduleNetDueOnItsInterval(t *testing.T) {
	s := newSlowSchedule()
	t0 := time.Now()
	s.markNet(t0)

	if net, _, _ := s.due(t0.Add(config.NetOnlineInterval-time.Millisecond), true); net {
		t.Error("net should not be due just before NetOnlineInterval")
	}
	if net, _, _ := s.due(t0.Add(config.NetOnlineInterval), true); !net {
		t.Error("net should be due at NetOnlineInterval when online")
	}
	// After a failure the next check comes sooner.
	if net, _, _ := s.due(t0.Add(config.NetFailInterval), false); !net {
		t.Error("net should be due at NetFailInterval when offline")
	}
}

func TestSlowScheduleGPUAdaptsToLoad(t *testing.T) {
	s := newSlowSchedule()
	t0 := time.Now()

	// Active reading → polls every base tick.
	s.markGPU(t0, 80)
	if _, gpu, _ := s.due(t0.Add(config.GPUActiveInterval), true); !gpu {
		t.Error("active GPU should be due again at GPUActiveInterval")
	}

	// Flat reading → backs off; not due at the active interval.
	s.markGPU(t0, 0)
	if _, gpu, _ := s.due(t0.Add(config.GPUActiveInterval), true); gpu {
		t.Error("flat GPU should NOT be due at GPUActiveInterval (backed off)")
	}
	if _, gpu, _ := s.due(t0.Add(config.GPUIdleInterval), true); !gpu {
		t.Error("flat GPU should be due at GPUIdleInterval")
	}
}
