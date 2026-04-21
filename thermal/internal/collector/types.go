package collector

import "time"

// BatteryState represents the macOS power source state.
type BatteryState int

const (
	BatteryUnknown       BatteryState = iota
	BatteryDischarging                // "discharging"
	BatteryCharging                   // "charging"
	BatteryCharged                    // "charged" (100%, plugged)
	BatteryACNotCharging              // "AC attached; not charging" (optimized-hold, cold)
	BatteryFinishing                  // "finishing charge" (near 100%)
)

// SystemStats holds system-wide resource utilization from sysctl/vm_stat.
type SystemStats struct {
	CPUPercent           float64 // mach host_statistics tick delta, 0-100
	MemUsedBytes         int64   // active + wired + compressed pages
	MemTotalBytes        int64   // hw.memsize
	SwapUsedBytes        int64
	SwapTotalBytes       int64
	Decompressions       int64   // vm_stat delta: decompressions since last tick
	GPUPercent           float64 // ioreg AGXAccelerator Device Utilization, 0-100
	NCPUs                int
	Timestamp            time.Time
	BatteryPresent       bool          // false on desktops / Mac Studio
	BatteryPercent       float64       // 0-100
	BatteryState         BatteryState  // enum: Unknown, Discharging, Charging, Charged, ACNotCharging
	BatteryTimeRemaining time.Duration // 0 when (no estimate) or AC-stable
}

// MemPercent returns memory utilization as 0-100.
func (s *SystemStats) MemPercent() float64 {
	if s.MemTotalBytes == 0 {
		return 0
	}
	return float64(s.MemUsedBytes) / float64(s.MemTotalBytes) * 100
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

// Snapshot is the unified data model produced by the collector goroutine.
type Snapshot struct {
	System            SystemStats
	Sessions          []SessionTree // one per Claude root process
	AllProcs          []ProcessInfo // flat list of all Claude descendants
	Online            bool          // can we reach the Claude API?
	DesktopRunning    bool          // Claude Desktop (Electron) main process detected
	ChromeHostRunning bool          // chrome-native-host (browser extension bridge) detected
	Timestamp         time.Time
	SlowAge           time.Duration // time since last successful slow-loop collection
	CollectErrs       []string      // non-nil when collection partially failed
}

// Category represents a process-based grouping visible in the dashboard.
type Category struct {
	Name  string // "build", "shell", "node", "go", "python", "rust", "swift"
	Label string // Display label
	Order int    // Sort order (0 = first in headline)
}

// Categories — process-based, matching what ps actually reports.
// Fixed categories (build, shell) always visible; runtimes appear dynamically.
var Categories = []Category{
	{Name: "build", Label: "build", Order: 0},
	{Name: "shell", Label: "shell", Order: 1},
	{Name: "node", Label: "node", Order: 2},
	{Name: "go", Label: "go", Order: 3},
	{Name: "python", Label: "python", Order: 4},
	{Name: "rust", Label: "rust", Order: 5},
	{Name: "swift", Label: "swift", Order: 6},
}

// FixedCategories are always visible in the headline bar even when count is zero.
var FixedCategories = map[string]bool{"build": true, "shell": true}

// RuntimeCategories are the dynamic language/runtime category names (non-fixed).
var RuntimeCategories []string

func init() {
	for _, cat := range Categories {
		if !FixedCategories[cat.Name] {
			RuntimeCategories = append(RuntimeCategories, cat.Name)
		}
	}
}

// TypeToCategory maps type codes to process-based category names.
var TypeToCategory = map[string]string{
	"V":  "node",   // vitest/jest show up as node in ps
	"T":  "build",  // tsc
	"B":  "build",  // bundlers, linters, compilers
	"N":  "node",   // node, deno, bun
	"P":  "python", // python, ruby, java, docker
	"GO": "go",     // go binary
	"RS": "rust",   // cargo, rustc
	"SW": "swift",  // swift, swiftc, xcodebuild
	"G":  "shell",  // grep, ag
	"R":  "shell",  // ripgrep
	"F":  "shell",  // find, fd
	"S":  "shell",  // bash, sh, zsh, sed, awk
	"C":  "shell",  // cat, git, curl, wget
	"X":  "shell",  // unknown
}
