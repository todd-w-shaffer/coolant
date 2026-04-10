package demo

import (
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

// collectDemo runs the demo for n ticks and returns snapshots + events.
func collectDemo(n int) ([]collector.Snapshot, []collector.GateEvent) {
	snapCh := make(chan collector.Snapshot, n+10)
	eventCh := make(chan collector.GateEvent, 100)
	done := make(chan struct{})

	go RunV2(snapCh, eventCh, 1*time.Millisecond, done)

	var snaps []collector.Snapshot
	for i := 0; i < n; i++ {
		s, ok := <-snapCh
		if !ok {
			break
		}
		snaps = append(snaps, s)
	}
	close(done)
	// drain remaining
	for range snapCh {
	}

	close(eventCh)
	var events []collector.GateEvent
	for e := range eventCh {
		events = append(events, e)
	}
	return snaps, events
}

func TestNarrativeArc(t *testing.T) {
	// 80 ticks = 20s at 250ms interval
	snaps, events := collectDemo(80)
	if len(snaps) < 80 {
		t.Fatalf("expected 80 snapshots, got %d", len(snaps))
	}

	// ── Agents spawn before language procs appear ─────────────
	t.Run("agents spawn first", func(t *testing.T) {
		if len(events) == 0 {
			t.Fatal("no agent events emitted")
		}
		if events[0].Event != collector.EventAgentStart {
			t.Errorf("first event should be agent.start, got %s", events[0].Event)
		}
	})

	// ── Language phase uses only node + go ────────────────────
	t.Run("language phase is node and go", func(t *testing.T) {
		foundLanguage := false
		for _, s := range snaps[:40] { // first half
			for _, p := range s.AllProcs {
				if p.TypeCode == "N" || p.TypeCode == "GO" {
					foundLanguage = true
				}
				// No rust, swift, python in the narrative
				if p.TypeCode == "RS" || p.TypeCode == "SW" || p.TypeCode == "P" {
					t.Errorf("unexpected language type %s in narrative demo", p.TypeCode)
				}
			}
		}
		if !foundLanguage {
			t.Error("no node or go procs found in first half")
		}
	})

	// ── Phases happen in order: language before build before shell ──
	t.Run("phases ordered", func(t *testing.T) {
		firstLang := -1
		firstBuild := -1
		firstShell := -1

		for i, s := range snaps {
			for _, p := range s.AllProcs {
				switch p.TypeCode {
				case "N", "GO":
					if firstLang == -1 {
						firstLang = i
					}
				case "T", "B":
					if firstBuild == -1 {
						firstBuild = i
					}
				case "S", "C", "X":
					if firstShell == -1 {
						firstShell = i
					}
				}
			}
		}

		if firstLang == -1 {
			t.Fatal("no language procs seen")
		}
		if firstBuild == -1 {
			t.Fatal("no build procs seen")
		}
		if firstShell == -1 {
			t.Fatal("no shell procs seen")
		}
		if firstLang >= firstBuild {
			t.Errorf("language (tick %d) should appear before build (tick %d)", firstLang, firstBuild)
		}
		if firstBuild >= firstShell {
			t.Errorf("build (tick %d) should appear before shell (tick %d)", firstBuild, firstShell)
		}
	})

	// ── Shell explosion reaches 80+ procs ────────────────────
	t.Run("shell reaches 80+", func(t *testing.T) {
		maxShell := 0
		for _, s := range snaps {
			count := 0
			for _, p := range s.AllProcs {
				if p.TypeCode == "S" || p.TypeCode == "C" || p.TypeCode == "X" {
					count++
				}
			}
			if count > maxShell {
				maxShell = count
			}
		}
		if maxShell < 80 {
			t.Errorf("shell peak was %d, want 80+", maxShell)
		}
	})

	// ── System stays calm before shell phase ─────────────────
	t.Run("system calm before shell", func(t *testing.T) {
		// Shell phase starts at tick 38. Check that CPU and MEM stay
		// moderate during language/build (ticks 0-37).
		shellStart := 38
		for i := 0; i < shellStart && i < len(snaps); i++ {
			if snaps[i].System.CPUPercent > 50 {
				t.Errorf("tick %d: CPU %.1f%% too high before shell phase", i, snaps[i].System.CPUPercent)
				break
			}
			if snaps[i].System.MemPercent() > 55 {
				t.Errorf("tick %d: MEM %.1f%% too high before shell phase", i, snaps[i].System.MemPercent())
				break
			}
		}
	})

	// ── CPU and memory pin during shell escalation ───────────
	t.Run("stats pin at shell peak", func(t *testing.T) {
		var maxCPU float64
		var maxMem float64
		for _, s := range snaps {
			if s.System.CPUPercent > maxCPU {
				maxCPU = s.System.CPUPercent
			}
			memPct := s.System.MemPercent()
			if memPct > maxMem {
				maxMem = memPct
			}
		}
		if maxCPU < 80 {
			t.Errorf("peak CPU was %.1f%%, want 80+%%", maxCPU)
		}
		if maxMem < 80 {
			t.Errorf("peak memory was %.1f%%, want 80+%%", maxMem)
		}
	})

	// ── Agents eventually die (cooldown) ─────────────────────
	t.Run("agents die in cooldown", func(t *testing.T) {
		hasStop := false
		for _, e := range events {
			if e.Event == collector.EventAgentStop {
				hasStop = true
				break
			}
		}
		if !hasStop {
			t.Error("no agent.stop events — demo never cools down")
		}
	})
}
