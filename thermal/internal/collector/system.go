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

// CollectSlowStats gathers swap, memory (vm_stat), and GPU via subprocesses.
// Returns a partially populated SystemStats (only slow-changing fields).
// Called from the slow loop (1s) to avoid spawning 3 processes every 150ms.
func CollectSlowStats(ctx context.Context) SystemStats {
	staticSysctl.once.Do(initStaticSysctl)

	type result struct {
		key string
		val string
	}
	ch := make(chan result, 3)

	go func() {
		out, _ := execCmd(ctx, "sysctl", "-n", "vm.swapusage")
		ch <- result{"swap", out}
	}()
	go func() {
		out, _ := execCmd(ctx, "vm_stat")
		ch <- result{"vmstat", out}
	}()
	go func() {
		out, _ := execCmd(ctx, "bash", "-c", `ioreg -r -d 1 -c AGXAccelerator | grep 'Device Utilization'`)
		ch <- result{"gpu", out}
	}()

	var stats SystemStats

	for i := 0; i < 3; i++ {
		r := <-ch
		switch r.key {
		case "swap":
			parseSwap(r.val, &stats)
		case "vmstat":
			if r.val != "" {
				active := parseVMStatField(r.val, "Pages active")
				wired := parseVMStatField(r.val, "Pages wired down")
				compressed := parseVMStatField(r.val, "Pages occupied by compressor")
				stats.MemUsedBytes = (active + wired + compressed) * staticSysctl.pageSize

				cumDecomps := parseVMStatField(r.val, "Decompressions")
				stats.Decompressions = sampleDecompressions(cumDecomps)
			}
		case "gpu":
			parseGPU(r.val, &stats)
		}
	}

	return stats
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
func parseGPU(s string, stats *SystemStats) {
	matches := gpuRe.FindStringSubmatch(s)
	if len(matches) < 2 {
		return
	}
	v, err := strconv.Atoi(matches[1])
	if err != nil {
		log.Printf("coolant: parse GPU utilization %q: %v", matches[1], err)
		return
	}
	stats.GPUPercent = float64(v)
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
