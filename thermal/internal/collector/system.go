package collector

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CollectSystem gathers CPU, memory, and swap stats from macOS system tools.
func CollectSystem(ctx context.Context) (SystemStats, error) {
	var stats SystemStats
	stats.Timestamp = time.Now()

	// All sysctl calls in parallel
	type result struct {
		key string
		val string
		err error
	}
	ch := make(chan result, 5)

	sysctls := map[string]string{
		"memsize":  "hw.memsize",
		"ncpu":     "hw.ncpu",
		"pagesize": "hw.pagesize",
		"loadavg":  "vm.loadavg",
		"swap":     "vm.swapusage",
	}
	for key, name := range sysctls {
		go func(k, n string) {
			out, err := execCmd(ctx, "sysctl", "-n", n)
			ch <- result{k, out, err}
		}(key, name)
	}

	// vm_stat separately (different output format)
	go func() {
		out, err := execCmd(ctx, "vm_stat")
		ch <- result{"vmstat", out, err}
	}()

	vals := make(map[string]string)
	for i := 0; i < 6; i++ {
		r := <-ch
		if r.err == nil {
			vals[r.key] = r.val
		}
	}

	// Parse total RAM
	if v, err := strconv.ParseInt(strings.TrimSpace(vals["memsize"]), 10, 64); err == nil {
		stats.MemTotalBytes = v
	}

	// Parse CPU count
	if v, err := strconv.Atoi(strings.TrimSpace(vals["ncpu"])); err == nil {
		stats.NCPUs = v
	}

	// Parse page size
	pageSize := int64(16384) // default for modern macOS
	if v, err := strconv.ParseInt(strings.TrimSpace(vals["pagesize"]), 10, 64); err == nil {
		pageSize = v
	}

	// Parse load average → CPU%
	if stats.NCPUs > 0 {
		load := parseLoadAvg(vals["loadavg"])
		stats.CPUPercent = (load / float64(stats.NCPUs)) * 100
		if stats.CPUPercent > 100 {
			stats.CPUPercent = 100
		}
	}

	// Parse vm_stat → memory used
	if vmstat := vals["vmstat"]; vmstat != "" {
		active := parseVMStatField(vmstat, "Pages active")
		wired := parseVMStatField(vmstat, "Pages wired down")
		compressed := parseVMStatField(vmstat, "Pages occupied by compressor")
		stats.MemUsedBytes = (active + wired + compressed) * pageSize
	}

	// Parse swap
	parseSwap(vals["swap"], &stats)

	return stats, nil
}

func execCmd(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// parseLoadAvg extracts the 1-minute load from "{ 1.23 4.56 7.89 }".
func parseLoadAvg(s string) float64 {
	s = strings.Trim(strings.TrimSpace(s), "{}")
	fields := strings.Fields(s)
	if len(fields) >= 1 {
		if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
			return v
		}
	}
	return 0
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
