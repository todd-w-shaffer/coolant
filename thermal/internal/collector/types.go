package collector

import "time"

// SystemStats holds system-wide resource utilization from sysctl/vm_stat.
type SystemStats struct {
	CPUPercent     float64 // mach host_statistics tick delta, 0-100
	MemUsedBytes   int64   // active + wired + compressed pages
	MemTotalBytes  int64   // hw.memsize
	SwapUsedBytes  int64
	SwapTotalBytes int64
	Decompressions int64   // vm_stat delta: decompressions since last tick
	GPUPercent     float64 // ioreg AGXAccelerator Device Utilization, 0-100
	NCPUs          int
	Timestamp      time.Time
}

// MemPercent returns memory utilization as 0-100.
func (s SystemStats) MemPercent() float64 {
	if s.MemTotalBytes == 0 {
		return 0
	}
	return float64(s.MemUsedBytes) / float64(s.MemTotalBytes) * 100
}

// SwapPercent returns swap as a percentage of physical RAM (not swap partition).
// 85GB swap on 16GB RAM = 531%. Tells you how far past physical memory you are.
func (s SystemStats) SwapPercent() float64 {
	if s.MemTotalBytes == 0 {
		return 0
	}
	return float64(s.SwapUsedBytes) / float64(s.MemTotalBytes) * 100
}

// ProcessInfo holds a single process from the Claude descendant tree.
type ProcessInfo struct {
	PID      int
	PPID     int
	CPUPct   float64
	RSSBytes int64
	Comm     string // short name: "node", "vitest", etc.
	TypeCode string // single char: N, V, T, etc.
}

// SessionTree holds one Claude root session and its descendant processes.
type SessionTree struct {
	RootPID     int
	RootComm    string
	Descendants []ProcessInfo
}

// TotalRSS returns the sum of RSS across all descendants.
func (s SessionTree) TotalRSS() int64 {
	var total int64
	for _, p := range s.Descendants {
		total += p.RSSBytes
	}
	return total
}

// TotalCPU returns the sum of CPU% across all descendants.
func (s SessionTree) TotalCPU() float64 {
	var total float64
	for _, p := range s.Descendants {
		total += p.CPUPct
	}
	return total
}

// Snapshot is the unified data model produced by the collector goroutine.
type Snapshot struct {
	System      SystemStats
	Sessions    []SessionTree // one per Claude root process
	AllProcs    []ProcessInfo // flat list of all Claude descendants
	Online      bool          // can we reach the Claude API?
	Timestamp   time.Time
	CollectErrs []string // non-nil when collection partially failed
}

// TotalProcs returns the total number of Claude descendant processes.
func (s Snapshot) TotalProcs() int {
	return len(s.AllProcs)
}

// Category represents what Claude is doing, not what executable is running.
type Category struct {
	Name  string // "test", "build", "run", "search", "shell"
	Label string // Display label
	Order int    // Sort order (0 = first/most dangerous)
}

// Default categories — hardcoded V1, will be config-driven later.
var Categories = []Category{
	{Name: "test", Label: "test", Order: 0},
	{Name: "build", Label: "build", Order: 1},
	{Name: "run", Label: "run", Order: 2},
	{Name: "search", Label: "search", Order: 3},
	{Name: "shell", Label: "shell", Order: 4},
}

// TypeToCategory maps single-char type codes to category names.
// Designed as a lookup table so it's easy to swap for config-driven mapping later.
var TypeToCategory = map[string]string{
	"V": "test",   // vitest, jest, mocha, pytest
	"T": "build",  // tsc
	"B": "build",  // bundlers, linters, compilers
	"N": "run",    // node, deno, bun
	"P": "run",    // python, ruby, java, docker
	"G": "search", // grep, ag
	"R": "search", // ripgrep
	"F": "search", // find, fd
	"S": "shell",  // bash, sh, zsh, sed, awk
	"C": "shell",  // cat, git, curl, wget
	"X": "shell",  // unknown
}
