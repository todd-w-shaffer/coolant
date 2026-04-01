package demo

import (
	"encoding/json"
	"math/rand"
	"os"
	"time"

	"github.com/toddwshaffer/coolant/cc-viz-go/internal/jsonl"
)

var typePool = []string{"N", "G", "V", "S", "R", "F", "C", "P", "T", "X"}

// Run writes synthetic JSONL data to path at the given interval.
func Run(path string, interval time.Duration, done <-chan struct{}) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	tick := 0
	var procs []jsonl.Proc
	nextPID := 1000

	for {
		select {
		case <-done:
			return
		default:
		}

		// Age existing processes first (before spawning, so new procs keep Age=0)
		for i := range procs {
			procs[i].Age++
		}

		// Randomly spawn new processes (occasional bursts to trigger alerts)
		spawns := rand.Intn(6)
		if rand.Intn(10) == 0 {
			spawns = 10 + rand.Intn(15) // burst: 10-24 new procs
		}
		for i := 0; i < spawns; i++ {
			procs = append(procs, jsonl.Proc{
				PID:  nextPID,
				Type: typePool[rand.Intn(len(typePool))],
				Age:  0,
			})
			nextPID++
		}

		// Randomly kill some processes (10-30% chance each)
		var alive []jsonl.Proc
		for _, p := range procs {
			deathChance := 0.1
			if p.Age > 20 {
				deathChance = 0.3
			} else if p.Age > 10 {
				deathChance = 0.2
			}
			if rand.Float64() > deathChance {
				alive = append(alive, p)
			}
		}
		procs = alive

		t := jsonl.Tick{
			Tick:  tick,
			TS:    time.Now().Unix(),
			Count: len(procs),
			Procs: procs,
		}

		enc.Encode(t)
		f.Sync()

		tick++

		select {
		case <-done:
			return
		case <-time.After(interval):
		}
	}
}
