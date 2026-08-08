package collector

import (
	"context"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// staticSysctl caches values that never change during the process lifetime.
var staticSysctl struct {
	once     sync.Once
	memTotal int64
	ncpu     int
	pageSize int64
}

func initStaticSysctl() {
	ctx, cancel := context.WithTimeout(context.Background(), config.SysInitTimeout)
	defer cancel()

	// Sequential reads — these are one-shot static values, no need for goroutines.
	if out, err := execCmd(ctx, "sysctl", "-n", "hw.memsize"); err == nil {
		v, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		if err != nil {
			log.Printf("coolant: parse hw.memsize %q: %v", out, err)
		}
		staticSysctl.memTotal = v
	}
	if out, err := execCmd(ctx, "sysctl", "-n", "hw.ncpu"); err == nil {
		v, err := strconv.Atoi(strings.TrimSpace(out))
		if err != nil {
			log.Printf("coolant: parse hw.ncpu %q: %v", out, err)
		}
		staticSysctl.ncpu = v
	}
	if out, err := execCmd(ctx, "sysctl", "-n", "hw.pagesize"); err == nil {
		v, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		if err != nil {
			log.Printf("coolant: parse hw.pagesize %q: %v", out, err)
		}
		staticSysctl.pageSize = v
	}
	if staticSysctl.pageSize == 0 {
		staticSysctl.pageSize = config.DefaultPageSize
	}
}

// decompSampler tracks cumulative decompressions for delta calculation.
var decompSampler struct {
	mu   sync.Mutex
	prev int64
}

// sampleDecompressions returns the delta since last call.
// First call returns 0 (no baseline yet).
func sampleDecompressions(cumulative int64) int64 {
	decompSampler.mu.Lock()
	prev := decompSampler.prev
	decompSampler.prev = cumulative
	decompSampler.mu.Unlock()

	if prev == 0 {
		return 0
	}
	delta := cumulative - prev
	if delta < 0 {
		return 0
	}
	return delta
}

// CollectCPU gathers CPU-only stats via cgo mach host_statistics — no subprocess.
// Called from the fast loop (150ms). Static values (memsize, ncpu) are cached once.
func CollectCPU() SystemStats {
	staticSysctl.once.Do(initStaticSysctl)

	var stats SystemStats
	stats.Timestamp = time.Now()
	stats.MemTotalBytes = staticSysctl.memTotal
	stats.NCPUs = staticSysctl.ncpu
	stats.CPUPercent = SampleCPUPercent()
	return stats
}

// CollectSwapVM gathers swap + memory/decompressions in-process via
// host_statistics64(HOST_VM_INFO64) and sysctlbyname("vm.swapusage") — see
// vmswap_darwin.go. These are the fixed-1s probes (vm's decompression counter
// is a per-tick delta, so its cadence must stay fixed); reading them in-process
// removes the two per-second subprocess forks the slow loop used to spawn. Each
// reader is nil-safe: a failed read leaves its fields at zero rather than
// faulting. ctx is retained for the SlowCollect signature; the in-process reads
// don't block on it.
func CollectSwapVM(ctx context.Context) SystemStats {
	staticSysctl.once.Do(initStaticSysctl)

	var stats SystemStats
	if total, used, ok := readSwapUsage(); ok {
		stats.SwapTotalBytes = int64(total)
		stats.SwapUsedBytes = int64(used)
	}
	if vc, ok := readVMStats64(); ok {
		stats.MemUsedBytes = int64(vc.Active+vc.Wired+vc.Compressed) * staticSysctl.pageSize
		stats.Decompressions = sampleDecompressions(int64(vc.Decompressions))
	}
	return stats
}

// CollectGPU reads GPU Device Utilization % via ioreg. Called on the adaptive
// GPU cadence (backs off when idle). ioreg is the heaviest probe, so polling it
// only when GPU load is present is the largest slow-loop power win. parseGPU
// regex-matches the value from the full output — no `| grep` shell wrapper.
//
// Returns ok=false on a read/parse failure so the scheduler can tell a flake
// apart from a genuine idle reading: a failed read must NOT drive the cadence
// to back off (which would mask real load for a full idle interval) and must
// NOT clobber the last-good cached value with a spurious 0%.
func CollectGPU(ctx context.Context) (pct float64, ok bool) {
	out, err := execCmd(ctx, "ioreg", "-r", "-d", "1", "-c", "AGXAccelerator")
	if err != nil {
		return 0, false
	}
	var stats SystemStats
	if !parseGPU(out, &stats) {
		return 0, false
	}
	return stats.GPUPercent, true
}

// CollectBattery reads battery state via pmset. Called on the slow battery
// cadence — pmset is the highest-latency probe for a once-per-minute value.
func CollectBattery(ctx context.Context) SystemStats {
	out, _ := execCmd(ctx, "pmset", "-g", "batt")
	var stats SystemStats
	parseBattery(out, &stats)
	return stats
}

// gpuCadence returns the wait before the next GPU read: a flat (idle) GPU backs
// off to GPUIdleInterval; an active GPU polls every base tick so load is tracked.
func gpuCadence(lastReading float64) time.Duration {
	if lastReading < config.GPUFlatThreshold {
		return config.GPUIdleInterval
	}
	return config.GPUActiveInterval
}

// netCadence returns the wait before the next connectivity check: slow while
// reachable, fast after a failure so an outage surfaces promptly.
func netCadence(online bool) time.Duration {
	if online {
		return config.NetOnlineInterval
	}
	return config.NetFailInterval
}

func execCmd(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, config.SysExecTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// parseVMStatField extracts a page count from vm_stat output.
// Lines look like: "Pages active:                  123456."
func parseVMStatField(vmstat, label string) int64 {
	for _, line := range strings.Split(vmstat, "\n") {
		if strings.Contains(line, label) {
			// Extract trailing number (remove dots)
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			numStr := strings.TrimSpace(parts[len(parts)-1])
			numStr = strings.TrimSuffix(numStr, ".")
			v, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				log.Printf("coolant: parse vm_stat %q field %q: %v", label, numStr, err)
				continue
			}
			return v
		}
	}
	return 0
}

var swapRe = regexp.MustCompile(`total\s*=\s*([\d.]+)M.*used\s*=\s*([\d.]+)M`)
var gpuRe = regexp.MustCompile(`"Device Utilization %"=(\d+)`)

// parseGPU extracts Device Utilization % from ioreg AGXAccelerator output.
// Returns false when no utilization value was found, so callers can tell a
// failed/empty read apart from a genuine 0% reading.
func parseGPU(s string, stats *SystemStats) bool {
	matches := gpuRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return false
	}
	v, err := strconv.Atoi(matches[1])
	if err != nil {
		log.Printf("coolant: parse GPU utilization %q: %v", matches[1], err)
		return false
	}
	stats.GPUPercent = float64(v)
	return true
}

// parseSwap extracts swap total/used from sysctl vm.swapusage output.
func parseSwap(s string, stats *SystemStats) {
	matches := swapRe.FindStringSubmatch(s)
	if len(matches) < 3 {
		return
	}
	total, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		log.Printf("coolant: parse swap total %q: %v", matches[1], err)
	} else {
		stats.SwapTotalBytes = int64(total * 1024 * 1024)
	}
	used, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		log.Printf("coolant: parse swap used %q: %v", matches[2], err)
	} else {
		stats.SwapUsedBytes = int64(used * 1024 * 1024)
	}
}
