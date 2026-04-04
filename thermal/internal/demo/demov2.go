package demo

import (
	"math/rand"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

var demoTypes = []string{"N", "G", "V", "S", "R", "F", "C", "P", "T", "X"}

// RunV2 generates synthetic Snapshots for the new layout modes.
// It simulates a realistic scenario: calm → ramp → hot → cool down, cycling.
func RunV2(ch chan<- collector.Snapshot, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var procs []collector.ProcessInfo
	nextPID := 10000
	sessionPIDs := []int{1001, 1002} // simulate 2 Claude sessions
	tick := 0

	// Base system stats (realistic M-series Mac)
	baseMem := int64(6 * model.GB)   // 6GB base usage
	totalMem := int64(16 * model.GB) // 16GB total

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}

		// Phase cycling: calm (0-30), ramp (30-60), hot (60-90), cool (90-120)
		phase := (tick / 30) % 4

		// Age existing procs
		for i := range procs {
			procs[i].RSSBytes += int64(rand.Intn(10 * model.MB)) // slow RSS growth
		}

		// Spawn new procs based on phase
		var spawnCount int
		switch phase {
		case 0: // calm
			spawnCount = rand.Intn(3)
		case 1: // ramp
			spawnCount = 2 + rand.Intn(5)
		case 2: // hot
			spawnCount = 3 + rand.Intn(8)
			if rand.Intn(5) == 0 {
				spawnCount = 12 + rand.Intn(10) // burst
			}
		case 3: // cool down
			spawnCount = rand.Intn(2)
		}

		for i := 0; i < spawnCount; i++ {
			typeCode := demoTypes[rand.Intn(len(demoTypes))]
			// Weight RSS by type
			var rss int64
			switch typeCode {
			case "V", "N":
				rss = int64(500+rand.Intn(1000)) * model.MB // 500MB-1.5GB
			case "T", "P":
				rss = int64(100+rand.Intn(300)) * model.MB // 100-400MB
			default:
				rss = int64(5+rand.Intn(30)) * model.MB // 5-35MB
			}

			procs = append(procs, collector.ProcessInfo{
				PID:      nextPID,
				PPID:     sessionPIDs[rand.Intn(len(sessionPIDs))],
				CPUPct:   float64(rand.Intn(30)),
				RSSBytes: rss,
				Comm:     typeCodeToComm(typeCode),
				TypeCode: typeCode,
			})
			nextPID++
		}

		// Kill procs based on phase
		deathRate := 0.1
		switch phase {
		case 1:
			deathRate = 0.05
		case 2:
			deathRate = 0.08
		case 3:
			deathRate = 0.25
		}
		var alive []collector.ProcessInfo
		for _, p := range procs {
			if rand.Float64() > deathRate {
				alive = append(alive, p)
			}
		}
		procs = alive

		// Build sessions
		sessionMap := make(map[int][]collector.ProcessInfo)
		for _, p := range procs {
			sessionMap[p.PPID] = append(sessionMap[p.PPID], p)
		}

		var sessions []collector.SessionTree
		for _, spid := range sessionPIDs {
			sessions = append(sessions, collector.SessionTree{
				RootPID:     spid,
				RootComm:    "claude",
				Descendants: sessionMap[spid],
			})
		}

		// Calculate simulated system stats
		var totalRSS int64
		var totalCPU float64
		for _, p := range procs {
			totalRSS += p.RSSBytes
			totalCPU += p.CPUPct
		}

		memUsed := baseMem + totalRSS
		if memUsed > totalMem {
			memUsed = totalMem // cap at total (rest goes to swap)
		}

		swapUsed := int64(0)
		if baseMem+totalRSS > totalMem {
			swapUsed = baseMem + totalRSS - totalMem
		}

		// Compressor decompressions correlate with memory pressure.
		// Light load: 200-800/tick. Heavy: 5K-30K. Swap territory: 50K+.
		memRatio := float64(baseMem+totalRSS) / float64(totalMem)
		var decomps int64
		switch {
		case memRatio > 1.0:
			decomps = 50000 + int64(rand.Intn(30000))
		case memRatio > 0.8:
			decomps = 5000 + int64(rand.Intn(25000))
		case memRatio > 0.5:
			decomps = 800 + int64(rand.Intn(4000))
		default:
			decomps = 200 + int64(rand.Intn(600))
		}

		cpuPct := 15.0 + totalCPU/8 // base 15% + Claude load distributed across 8 cores
		if cpuPct > 100 {
			cpuPct = 100
		}

		// Simulate offline every ~40 ticks for 10 ticks
		online := true
		if (tick/40)%3 == 2 && (tick%40) < 10 {
			online = false
		}

		snap := collector.Snapshot{
			System: collector.SystemStats{
				CPUPercent:     cpuPct,
				MemUsedBytes:   memUsed,
				MemTotalBytes:  totalMem,
				SwapUsedBytes:  swapUsed,
				SwapTotalBytes: 8 * model.GB, // 8GB swap
				Decompressions: decomps,
				NCPUs:          8,
				Timestamp:      time.Now(),
			},
			Sessions:  sessions,
			AllProcs:  procs,
			Online:    online,
			Timestamp: time.Now(),
		}

		select {
		case ch <- snap:
		case <-done:
			return
		}

		tick++
	}
}

func typeCodeToComm(code string) string {
	switch code {
	case "N":
		return "node"
	case "V":
		return "vitest"
	case "T":
		return "tsc"
	case "P":
		return "python3"
	case "G":
		return "grep"
	case "R":
		return "rg"
	case "F":
		return "find"
	case "S":
		return "bash"
	case "C":
		return "cat"
	default:
		return "unknown"
	}
}
