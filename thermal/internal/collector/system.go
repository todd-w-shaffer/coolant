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

// CollectSystem gathers CPU, memory, and swap stats from macOS system tools.
// CPU% uses mach host_statistics (same as Activity Monitor) via SampleCPUPercent.
// Memory and swap use sysctl/vm_stat.
func CollectSystem(ctx context.Context) (SystemStats, error) {
	// Cache static values (hw.memsize, hw.ncpu, hw.pagesize) — they never change.
	staticSysctl.once.Do(initStaticSysctl)

	var stats SystemStats
	stats.Timestamp = time.Now()
	stats.MemTotalBytes = staticSysctl.memTotal
	stats.NCPUs = staticSysctl.ncpu

	// CPU% from mach kernel ticks — no subprocess needed
	stats.CPUPercent = SampleCPUPercent()

	// Only dynamic values need per-tick subprocess calls: vm.swapusage + vm_stat
	type result struct {
		key string
		val string
	}
	ch := make(chan result, 2)

	go func() {
		out, _ := execCmd(ctx, "sysctl", "-n", "vm.swapusage")
		ch <- result{"swap", out}
	}()
	go func() {
		out, _ := execCmd(ctx, "vm_stat")
		ch <- result{"vmstat", out}
	}()

	for i := 0; i < 2; i++ {
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
		}
	}

	return stats, nil
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
