package collector

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// staticSysctl caches values that never change during the process lifetime.
var staticSysctl struct {
	once     sync.Once
	memTotal int64
	ncpu     int
	pageSize int64
}

func initStaticSysctl() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		key string
		val string
	}
	ch := make(chan result, 3)
	for key, name := range map[string]string{
		"memsize":  "hw.memsize",
		"ncpu":     "hw.ncpu",
		"pagesize": "hw.pagesize",
	} {
		go func(k, n string) {
			out, _ := execCmd(ctx, "sysctl", "-n", n)
			ch <- result{k, out}
		}(key, name)
	}
	for i := 0; i < 3; i++ {
		r := <-ch
		switch r.key {
		case "memsize":
			staticSysctl.memTotal, _ = strconv.ParseInt(strings.TrimSpace(r.val), 10, 64)
		case "ncpu":
			staticSysctl.ncpu, _ = strconv.Atoi(strings.TrimSpace(r.val))
		case "pagesize":
			staticSysctl.pageSize, _ = strconv.ParseInt(strings.TrimSpace(r.val), 10, 64)
		}
	}
	if staticSysctl.pageSize == 0 {
		staticSysctl.pageSize = 16384 // default for modern macOS
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
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
			if v, err := strconv.ParseInt(numStr, 10, 64); err == nil {
				return v
			}
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
	if total, err := strconv.ParseFloat(matches[1], 64); err == nil {
		stats.SwapTotalBytes = int64(total * 1024 * 1024)
	}
	if used, err := strconv.ParseFloat(matches[2], 64); err == nil {
		stats.SwapUsedBytes = int64(used * 1024 * 1024)
	}
}
