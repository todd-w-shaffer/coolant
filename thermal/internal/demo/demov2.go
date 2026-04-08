// Package demo generates synthetic snapshots for the --demo flag,
// simulating realistic multi-session escalation cycles.
package demo

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// ── Per-session escalation phases ─────────────────────────────

const (
	phaseIdle     = 0
	phaseLanguage = 1
	phaseBuild    = 2
	phaseShell    = 3
	phaseCooldown = 4
)

// Phase durations in ticks (each tick = 250ms).
const (
	idleTicks     = 8                                                                   // 2.0s
	languageTicks = 6                                                                   // 1.5s
	buildTicks    = 6                                                                   // 1.5s
	shellTicks    = 10                                                                  // 2.5s
	cooldownTicks = 6                                                                   // 1.5s
	phaseCycle    = idleTicks + languageTicks + buildTicks + shellTicks + cooldownTicks // 36
)

// Session stagger offsets (in ticks) so sessions overlap at different phases.
var sessionOffsets = [3]int{0, 12, 24}

// Type pools per phase.
var (
	languageTypes = []string{"GO", "N", "RS", "SW", "P"}
	buildTypes    = []string{"T", "B"}
	shellTypes    = []string{"S", "C", "X"}
)

type sessionPhaseState struct {
	phase     int // phaseIdle..phaseCooldown
	ticksLeft int // ticks remaining in current phase
	started   bool
}

// advance moves to the next phase when ticksLeft reaches zero.
func (s *sessionPhaseState) advance() {
	s.ticksLeft--
	if s.ticksLeft <= 0 {
		s.phase = (s.phase + 1) % 5
		switch s.phase {
		case phaseIdle:
			s.ticksLeft = idleTicks
		case phaseLanguage:
			s.ticksLeft = languageTicks
		case phaseBuild:
			s.ticksLeft = buildTicks
		case phaseShell:
			s.ticksLeft = shellTicks
		case phaseCooldown:
			s.ticksLeft = cooldownTicks
		}
	}
}

// initPhase sets a session to the correct phase for a given tick offset.
func initPhase(offset int) sessionPhaseState {
	st := sessionPhaseState{phase: phaseIdle, ticksLeft: idleTicks}
	for i := 0; i < offset; i++ {
		st.advance()
	}
	st.started = false
	return st
}

// RunV2 generates synthetic Snapshots for the new layout modes.
// It simulates a realistic scenario: calm → ramp → hot → cool down, cycling.
// Each active session independently cycles through escalation phases:
// idle → language → build → shell explosion → cooldown.
func RunV2(ch chan<- collector.Snapshot, eventCh chan<- collector.GateEvent, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	nextPID := 10000
	allSessionPIDs := []int{1001, 1002, 1003}
	prevActiveCount := 0
	tick := 0

	// Per-session process pools and phase states.
	sessionProcs := [3][]collector.ProcessInfo{}
	sessionPhases := [3]sessionPhaseState{}
	for i := range sessionPhases {
		sessionPhases[i] = initPhase(sessionOffsets[i])
	}

	// Base system stats (realistic M-series Mac).
	baseMem := int64(6 * model.GB)
	totalMem := int64(16 * model.GB)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}

		// ── Macro phase: how many sessions are active ─────────
		// calm (0-30), ramp (30-60), hot (60-90), cool (90-120)
		macroPhase := (tick / 30) % 4
		var activeCount int
		switch macroPhase {
		case 0:
			activeCount = 1
		case 1:
			activeCount = 2
		case 2:
			activeCount = 3
		case 3:
			activeCount = 2
		}

		// ── Agent start/stop events ───────────────────────────
		if eventCh != nil && activeCount != prevActiveCount {
			now := time.Now()
			if activeCount > prevActiveCount {
				for i := prevActiveCount; i < activeCount; i++ {
					eventCh <- collector.GateEvent{
						Timestamp: now,
						Event:     collector.EventAgentStart,
						AgentID:   fmt.Sprintf("demo-agent-%d", allSessionPIDs[i]),
						AgentType: "general-purpose",
					}
				}
			} else {
				for i := activeCount; i < prevActiveCount; i++ {
					eventCh <- collector.GateEvent{
						Timestamp: now,
						Event:     collector.EventAgentStop,
						AgentID:   fmt.Sprintf("demo-agent-%d", allSessionPIDs[i]),
						AgentType: "general-purpose",
					}
				}
			}
			prevActiveCount = activeCount
		}

		// ── Per-session tick: spawn/kill based on escalation phase ──
		for i := 0; i < 3; i++ {
			active := i < activeCount
			if !active {
				// Inactive sessions shed all processes and reset.
				sessionProcs[i] = nil
				sessionPhases[i] = initPhase(sessionOffsets[i])
				continue
			}

			st := &sessionPhases[i]

			// Emit agent events on phase transitions (language = start, idle = stop).
			if !st.started && st.phase != phaseIdle {
				st.started = true
			}
			if st.started && st.phase == phaseIdle && st.ticksLeft == idleTicks {
				// Just transitioned back to idle — reset started flag.
				st.started = false
			}

			spid := allSessionPIDs[i]
			procs := sessionProcs[i]

			switch st.phase {
			case phaseIdle:
				// No spawns. Residual processes die quickly.
				procs = decayProcs(procs, 0.4)

			case phaseLanguage:
				// Spawn 2-4 runtime processes per tick, slow death rate.
				count := 2 + rand.Intn(3)
				for j := 0; j < count; j++ {
					tc := languageTypes[rand.Intn(len(languageTypes))]
					procs = append(procs, makeProc(&nextPID, spid, tc))
				}
				procs = decayProcs(procs, 0.05)

			case phaseBuild:
				// Spawn 2-3 build processes + keep 1-2 lingering language procs.
				count := 2 + rand.Intn(2)
				for j := 0; j < count; j++ {
					tc := buildTypes[rand.Intn(len(buildTypes))]
					procs = append(procs, makeProc(&nextPID, spid, tc))
				}
				// Occasionally add a language proc (lingering).
				if rand.Intn(3) == 0 {
					tc := languageTypes[rand.Intn(len(languageTypes))]
					procs = append(procs, makeProc(&nextPID, spid, tc))
				}
				// Kill language procs faster than build procs.
				procs = decayProcsByType(procs, 0.20, languageTypes, 0.05)

			case phaseShell:
				// Bulk-spawn shells to hit 30+ total. Kill language/build procs.
				shellCount := countTypes(procs, shellTypes)
				target := 30 + rand.Intn(11) // 30-40 target
				if shellCount < target {
					batch := target - shellCount
					if batch > 12 {
						batch = 12 // cap per-tick burst
					}
					for j := 0; j < batch; j++ {
						tc := shellTypes[rand.Intn(len(shellTypes))]
						procs = append(procs, makeProc(&nextPID, spid, tc))
					}
				}
				// Language and build procs die off aggressively.
				procs = decayProcsByType(procs, 0.35, languageTypes, 0.02)
				procs = decayProcsByType(procs, 0.35, buildTypes, 0.02)

			case phaseCooldown:
				// No new spawns. Shells die off steadily.
				procs = decayProcs(procs, 0.20)
			}

			// Slow RSS growth on surviving procs.
			for j := range procs {
				procs[j].RSSBytes += int64(rand.Intn(5 * model.MB))
			}

			sessionProcs[i] = procs
			st.advance()
		}

		// ── Flatten all procs ─────────────────────────────────
		var allProcs []collector.ProcessInfo
		for i := 0; i < 3; i++ {
			allProcs = append(allProcs, sessionProcs[i]...)
		}

		// ── Build sessions ────────────────────────────────────
		sessionMap := make(map[int][]collector.ProcessInfo)
		for _, p := range allProcs {
			sessionMap[p.PPID] = append(sessionMap[p.PPID], p)
		}
		sessionPIDs := allSessionPIDs[:activeCount]
		var sessions []collector.SessionTree
		for _, spid := range sessionPIDs {
			sessions = append(sessions, collector.SessionTree{
				RootPID:     spid,
				RootComm:    "claude",
				Descendants: sessionMap[spid],
			})
		}

		// ── System stats correlated with process load ─────────
		var totalRSS int64
		var totalCPU float64
		for _, p := range allProcs {
			totalRSS += p.RSSBytes
			totalCPU += p.CPUPct
		}

		memUsed := baseMem + totalRSS
		if memUsed > totalMem {
			memUsed = totalMem
		}

		swapUsed := int64(0)
		if baseMem+totalRSS > totalMem {
			swapUsed = baseMem + totalRSS - totalMem
		}

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

		cpuPct := 15.0 + totalCPU/8
		if cpuPct > 100 {
			cpuPct = 100
		}

		gpuPct := 3.0 + float64(len(allProcs))*0.8 + float64(rand.Intn(5))
		if gpuPct > 100 {
			gpuPct = 100
		}

		// Simulate offline every ~40 ticks for 10 ticks.
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
				SwapTotalBytes: 8 * model.GB,
				Decompressions: decomps,
				GPUPercent:     gpuPct,
				NCPUs:          8,
				Timestamp:      time.Now(),
			},
			Sessions:  sessions,
			AllProcs:  allProcs,
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

// ── Process helpers ───────────────────────────────────────────

// makeProc creates a new ProcessInfo with type-appropriate RSS and CPU.
func makeProc(nextPID *int, ppid int, typeCode string) collector.ProcessInfo {
	var rss int64
	var cpu float64
	switch typeCode {
	case "N":
		rss = int64(500+rand.Intn(1000)) * model.MB // 500MB-1.5GB
		cpu = float64(10 + rand.Intn(20))
	case "SW":
		rss = int64(200+rand.Intn(600)) * model.MB // 200MB-800MB
		cpu = float64(15 + rand.Intn(25))
	case "GO":
		rss = int64(200+rand.Intn(600)) * model.MB // 200MB-800MB
		cpu = float64(10 + rand.Intn(20))
	case "RS":
		rss = int64(300+rand.Intn(500)) * model.MB // 300MB-800MB
		cpu = float64(20 + rand.Intn(25))
	case "P":
		rss = int64(100+rand.Intn(300)) * model.MB // 100-400MB
		cpu = float64(5 + rand.Intn(15))
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
	case "P":
		return "python3"
	case "GO":
		return "go"
	case "RS":
		return "cargo"
	case "SW":
		return "swiftc"
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
