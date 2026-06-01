// Package demo generates synthetic snapshots for the --demo flag,
// simulating a single clear narrative arc: calm → ramp → meltdown → cooldown.
package demo

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// ── Narrative phases (single linear arc) ─────────────────────

const (
	phaseCalm     = 0 // 1 agent idle
	phaseSpawn    = 1 // agents 2+3 appear
	phaseLang     = 2 // node + go runtimes
	phaseBuild    = 3 // tsc + esbuild join
	phaseShell    = 4 // shell explosion to 80+
	phaseCooldown = 5 // everything winds down
)

// Phase boundaries in ticks (250ms each). Total 80 ticks = 20s.
var phaseBoundaries = [7]int{0, 8, 16, 28, 38, 60, 80}

func phaseAt(tick int) int {
	for i := len(phaseBoundaries) - 1; i >= 0; i-- {
		if tick >= phaseBoundaries[i] {
			if i >= len(phaseBoundaries)-1 {
				return phaseCooldown
			}
			return i
		}
	}
	return phaseCalm
}

// RunV2 generates synthetic Snapshots with a clear narrative arc:
// calm → agents spawn → language (node+go) → build → shell explosion → cooldown.
func RunV2(ch chan<- collector.Snapshot, eventCh chan<- collector.GateEvent, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	nextPID := 10000
	allSessionPIDs := []int{1001, 1002, 1003}
	prevAgents := 0
	tick := 0

	var procs []collector.ProcessInfo

	// Synthetic token accumulator — rate scales with totalCPU each phase.
	var tokInput, tokOutput, tokCacheCreate, tokCacheRead int64
	var tokRateEMA float64

	// Base system stats (realistic M-series Mac).
	baseMem := int64(6 * model.GB)
	totalMem := int64(16 * model.GB)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}

		// Loop the 20-second arc indefinitely. Raw tick still drives
		// independent oscillators (offline cycle, agent IDs); phase and
		// per-phase progress use arcTick so coolProgress never exceeds 1.0
		// and the synthetic stats don't go negative post-arc.
		arcLen := phaseBoundaries[len(phaseBoundaries)-1]
		arcTick := tick % arcLen
		phase := phaseAt(arcTick)

		// ── Agent count per phase ────────────────────────────
		var agentCount int
		switch phase {
		case phaseCalm:
			agentCount = 1
		case phaseSpawn:
			// Ramp from 1 to 3 over the phase
			progress := arcTick - phaseBoundaries[phaseSpawn]
			phaseDur := phaseBoundaries[phaseSpawn+1] - phaseBoundaries[phaseSpawn]
			if progress < phaseDur/2 {
				agentCount = 2
			} else {
				agentCount = 3
			}
		case phaseLang, phaseBuild, phaseShell:
			agentCount = 3
		case phaseCooldown:
			// Ramp from 3 to 1
			progress := arcTick - phaseBoundaries[phaseCooldown]
			phaseDur := phaseBoundaries[phaseCooldown+1] - phaseBoundaries[phaseCooldown]
			third := phaseDur / 3
			switch {
			case progress < third:
				agentCount = 3
			case progress < third*2:
				agentCount = 2
			default:
				agentCount = 1
			}
		}

		// ── Agent start/stop events ──────────────────────────
		if eventCh != nil && agentCount != prevAgents {
			now := time.Now()
			if agentCount > prevAgents {
				for i := prevAgents; i < agentCount; i++ {
					eventCh <- collector.GateEvent{
						Schema:    1,
						Timestamp: now,
						Event:     collector.EventAgentStart,
						AgentID:   fmt.Sprintf("demo-agent-%d", allSessionPIDs[i]),
						AgentType: "general-purpose",
						Project:   "demo",
						SessionID: "demo-session",
					}
				}
			} else {
				for i := agentCount; i < prevAgents; i++ {
					eventCh <- collector.GateEvent{
						Schema:    1,
						Timestamp: now,
						Event:     collector.EventAgentStop,
						AgentID:   fmt.Sprintf("demo-agent-%d", allSessionPIDs[i]),
						AgentType: "general-purpose",
						Project:   "demo",
						SessionID: "demo-session",
					}
				}
			}
			prevAgents = agentCount
		}

		// ── Process spawning per phase ───────────────────────
		switch phase {
		case phaseCalm:
			// No spawns, slow decay.
			procs = decayProcs(procs, 0.3)

		case phaseSpawn:
			// Agents appearing, light activity.
			procs = decayProcs(procs, 0.2)

		case phaseLang:
			// Spawn node + go processes, 2-4 per tick.
			count := 2 + rand.Intn(3)
			for j := 0; j < count; j++ {
				tc := "N"
				if rand.Intn(2) == 0 {
					tc = "GO"
				}
				ppid := allSessionPIDs[rand.Intn(agentCount)]
				procs = append(procs, makeProc(&nextPID, ppid, tc))
			}
			procs = decayProcs(procs, 0.05)

		case phaseBuild:
			// Add build procs (tsc + esbuild), keep some language lingering.
			buildCount := 2 + rand.Intn(2)
			for j := 0; j < buildCount; j++ {
				tc := "T"
				if rand.Intn(2) == 0 {
					tc = "B"
				}
				ppid := allSessionPIDs[rand.Intn(agentCount)]
				procs = append(procs, makeProc(&nextPID, ppid, tc))
			}
			// Occasionally add a lingering language proc.
			if rand.Intn(3) == 0 {
				tc := "N"
				if rand.Intn(2) == 0 {
					tc = "GO"
				}
				ppid := allSessionPIDs[rand.Intn(agentCount)]
				procs = append(procs, makeProc(&nextPID, ppid, tc))
			}
			// Language procs die faster than build procs.
			langTypes := []string{"N", "GO"}
			procs = decayProcsByType(procs, 0.15, langTypes, 0.03)

		case phaseShell:
			// Bulk-spawn shells to 80+. Kill language/build procs.
			shellCount := countTypes(procs, []string{"S", "C", "X"})
			target := 80 + rand.Intn(21) // 80-100 target
			if shellCount < target {
				batch := target - shellCount
				if batch > 20 {
					batch = 20 // cap per-tick burst
				}
				shellTypes := []string{"S", "C", "X"}
				for j := 0; j < batch; j++ {
					tc := shellTypes[rand.Intn(len(shellTypes))]
					ppid := allSessionPIDs[rand.Intn(agentCount)]
					procs = append(procs, makeProc(&nextPID, ppid, tc))
				}
			}
			// Language and build procs die off aggressively.
			procs = decayProcsByType(procs, 0.35, []string{"N", "GO"}, 0.02)
			procs = decayProcsByType(procs, 0.30, []string{"T", "B"}, 0.02)

		case phaseCooldown:
			// No new spawns. Everything dies off steadily.
			procs = decayProcs(procs, 0.15)
		}

		// Slow RSS growth on surviving procs.
		for j := range procs {
			procs[j].RSSBytes += int64(rand.Intn(5 * model.MB))
		}

		// ── Build sessions ───────────────────────────────────
		sessionMap := make(map[int][]collector.ProcessInfo)
		for _, p := range procs {
			sessionMap[p.PPID] = append(sessionMap[p.PPID], p)
		}
		sessionPIDs := allSessionPIDs[:agentCount]
		var sessions []collector.SessionTree
		for _, spid := range sessionPIDs {
			sessions = append(sessions, collector.SessionTree{
				RootPID:     spid,
				RootComm:    "claude",
				Descendants: sessionMap[spid],
			})
		}

		// ── System stats driven by narrative phase ───────────
		// Language/build = calm system. Shell explosion = everything spikes.
		var cpuPct, gpuPct float64
		var memUsed, swapUsed, decomps int64

		switch phase {
		case phaseCalm, phaseSpawn:
			cpuPct = 8.0 + float64(rand.Intn(8))
			memUsed = baseMem + int64(rand.Intn(500))*model.MB
			decomps = 100 + int64(rand.Intn(300))
			gpuPct = 2.0 + float64(rand.Intn(3))

		case phaseLang:
			cpuPct = 20.0 + float64(rand.Intn(15))
			memUsed = baseMem + int64(1+rand.Intn(2))*model.GB
			decomps = 300 + int64(rand.Intn(700))
			gpuPct = 5.0 + float64(rand.Intn(5))

		case phaseBuild:
			cpuPct = 30.0 + float64(rand.Intn(15))
			memUsed = baseMem + int64(1+rand.Intn(1))*model.GB
			decomps = 500 + int64(rand.Intn(1500))
			gpuPct = 8.0 + float64(rand.Intn(5))

		case phaseShell:
			// Ramp within the shell phase itself
			shellProgress := float64(arcTick-phaseBoundaries[phaseShell]) / float64(phaseBoundaries[phaseShell+1]-phaseBoundaries[phaseShell])
			cpuPct = 50.0 + shellProgress*50.0 + float64(rand.Intn(5))
			if cpuPct > 100 {
				cpuPct = 100
			}
			memRamp := baseMem + int64(shellProgress*float64(totalMem-baseMem))
			memUsed = memRamp + int64(rand.Intn(500))*model.MB
			if memUsed > totalMem {
				memUsed = totalMem
				swapUsed = int64(shellProgress*8) * model.GB
			}
			decomps = 5000 + int64(shellProgress*50000) + int64(rand.Intn(10000))
			gpuPct = 30.0 + shellProgress*60.0 + float64(rand.Intn(5))
			if gpuPct > 100 {
				gpuPct = 100
			}

		case phaseCooldown:
			coolProgress := float64(arcTick-phaseBoundaries[phaseCooldown]) / float64(phaseBoundaries[phaseCooldown+1]-phaseBoundaries[phaseCooldown])
			cpuPct = 80.0 - coolProgress*60.0 + float64(rand.Intn(5))
			memUsed = totalMem - int64(coolProgress*float64(totalMem-baseMem))
			decomps = 30000 - int64(coolProgress*28000) + int64(rand.Intn(2000))
			gpuPct = 60.0 - coolProgress*50.0 + float64(rand.Intn(5))
		}

		// Simulate offline every ~40 ticks for 10 ticks.
		online := true
		if (tick/40)%3 == 2 && (tick%40) < 10 {
			online = false
		}

		// Synthetic battery: drains from 65% across the arc, giving the
		// battery cell a visible narrative even in --demo mode.
		battPct := 65.0 - float64(arcTick)*0.5
		if battPct < 3 {
			battPct = 3
		}
		battState := collector.BatteryDischarging
		battRemaining := time.Duration(battPct/100.0*6.0) * time.Hour

		// Synthetic token throughput — scales with CPU + active sessions, oscillates per-tick.
		activeSessions := 0
		for _, sess := range sessions {
			if len(sess.Descendants) > 0 {
				activeSessions++
			}
		}
		var tokens collector.TokenStats
		if activeSessions > 0 {
			targetRate := 400.0 + cpuPct*8 + float64(rand.Intn(200))
			tokRateEMA = 0.3*targetRate + 0.7*tokRateEMA
			perTick := int64(targetRate * interval.Seconds())
			tokInput += perTick / 8
			tokOutput += perTick / 16
			tokCacheCreate += perTick / 8
			tokCacheRead += perTick * 6 / 10
			tokens = collector.TokenStats{
				InputTotal:       tokInput,
				OutputTotal:      tokOutput,
				CacheCreateTotal: tokCacheCreate,
				CacheReadTotal:   tokCacheRead,
				IOTokensPerSec:   tokRateEMA,
				CacheHitRatio:    float64(tokCacheRead) / float64(tokInput+tokCacheCreate+tokCacheRead),
				ActiveSessions:   activeSessions,
			}
		}

		snap := collector.Snapshot{
			System: collector.SystemStats{
				CPUPercent:           cpuPct,
				MemUsedBytes:         memUsed,
				MemTotalBytes:        totalMem,
				SwapUsedBytes:        swapUsed,
				SwapTotalBytes:       8 * model.GB,
				Decompressions:       decomps,
				GPUPercent:           gpuPct,
				NCPUs:                8,
				Timestamp:            time.Now(),
				BatteryPresent:       true,
				BatteryPercent:       battPct,
				BatteryState:         battState,
				BatteryTimeRemaining: battRemaining,
			},
			Sessions:  sessions,
			AllProcs:  procs,
			Tokens:    tokens,
			Online:    online,
			Timestamp: time.Now(),
			// Each demo tick recomputes a fresh proc set, so it's a fresh proc
			// sample — bump ProcSeq so the model's spawn/death gate (which skips
			// stale re-deliveries from the live 1s scan) counts demo spawns.
			ProcSeq: uint64(tick) + 1,
		}

		select {
		case ch <- snap:
		case <-done:
			return
		}

		tick++
	}
}

// ── Process helpers ───────────────────────────────────────────

// makeProc creates a new ProcessInfo with type-appropriate RSS and CPU.
func makeProc(nextPID *int, ppid int, typeCode string) collector.ProcessInfo {
	var rss int64
	var cpu float64
	switch typeCode {
	case "N":
		rss = int64(500+rand.Intn(1000)) * model.MB // 500MB-1.5GB
		cpu = float64(10 + rand.Intn(20))
	case "GO":
		rss = int64(200+rand.Intn(600)) * model.MB // 200MB-800MB
		cpu = float64(10 + rand.Intn(20))
	case "T":
		rss = int64(100+rand.Intn(300)) * model.MB // 100-400MB
		cpu = float64(15 + rand.Intn(20))
	case "B":
		rss = int64(80+rand.Intn(200)) * model.MB // 80-280MB
		cpu = float64(10 + rand.Intn(15))
	default: // S, C, X — shells
		rss = int64(5+rand.Intn(30)) * model.MB // 5-35MB
		cpu = float64(rand.Intn(5))
	}

	p := collector.ProcessInfo{
		PID:      *nextPID,
		PPID:     ppid,
		CPUPct:   cpu,
		RSSBytes: rss,
		Comm:     typeCodeToComm(typeCode),
		TypeCode: typeCode,
	}
	*nextPID++
	return p
}

// decayProcs randomly kills processes at the given death rate.
func decayProcs(procs []collector.ProcessInfo, deathRate float64) []collector.ProcessInfo {
	var alive []collector.ProcessInfo
	for _, p := range procs {
		if rand.Float64() > deathRate {
			alive = append(alive, p)
		}
	}
	return alive
}

// decayProcsByType applies a high death rate to target types and a low rate to others.
func decayProcsByType(procs []collector.ProcessInfo, targetRate float64, targetTypes []string, otherRate float64) []collector.ProcessInfo {
	targets := make(map[string]bool, len(targetTypes))
	for _, t := range targetTypes {
		targets[t] = true
	}
	var alive []collector.ProcessInfo
	for _, p := range procs {
		rate := otherRate
		if targets[p.TypeCode] {
			rate = targetRate
		}
		if rand.Float64() > rate {
			alive = append(alive, p)
		}
	}
	return alive
}

// countTypes counts how many procs match any of the given type codes.
func countTypes(procs []collector.ProcessInfo, types []string) int {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[t] = true
	}
	n := 0
	for _, p := range procs {
		if m[p.TypeCode] {
			n++
		}
	}
	return n
}

func typeCodeToComm(code string) string {
	switch code {
	case "N":
		return "node"
	case "T":
		return "tsc"
	case "B":
		return "esbuild"
	case "GO":
		return "go"
	case "S":
		return "bash"
	case "C":
		return "cat"
	case "X":
		return "unknown"
	default:
		return "unknown"
	}
}
