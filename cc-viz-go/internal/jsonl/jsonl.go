package jsonl

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"time"
)

// Proc represents a single process in a snapshot.
type Proc struct {
	PID  int    `json:"pid"`
	Type string `json:"type"`
	Age  int    `json:"age"`
}

// Tick represents one JSONL snapshot line.
type Tick struct {
	Tick  int    `json:"tick"`
	TS    int64  `json:"ts"`
	Count int    `json:"count"`
	Procs []Proc `json:"procs"`
}

// TypeCounts returns a map of type letter -> count for this tick.
func (t *Tick) TypeCounts() map[string]int {
	counts := make(map[string]int)
	for _, p := range t.Procs {
		counts[p.Type]++
	}
	return counts
}

// Spawns returns the number of processes with age == 0.
func (t *Tick) Spawns() int {
	n := 0
	for _, p := range t.Procs {
		if p.Age == 0 {
			n++
		}
	}
	return n
}

// Parse parses a single JSONL line into a Tick.
func Parse(line []byte) (Tick, error) {
	var t Tick
	err := json.Unmarshal(line, &t)
	return t, err
}

// Tail follows a JSONL file and sends parsed ticks to the channel.
// Reads existing data first (backfill), then polls for new lines.
// Closes the channel when done is closed.
func Tail(path string, ch chan<- Tick, done <-chan struct{}) {
	defer close(ch)

	// Wait for the file to exist
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		select {
		case <-done:
			return
		case <-time.After(250 * time.Millisecond):
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	for {
		select {
		case <-done:
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// No new data yet — poll
				time.Sleep(200 * time.Millisecond)
				continue
			}
			return
		}

		// Trim trailing newline
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) == 0 {
			continue
		}

		tick, parseErr := Parse(line)
		if parseErr != nil {
			continue
		}

		select {
		case ch <- tick:
		case <-done:
			return
		}
	}
}
