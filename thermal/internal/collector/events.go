package collector

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"time"
)

// Event name constants — must match the strings emitted by bash hook scripts.
const (
	EventGateSuppress       = "gate.suppress"
	EventGateCap            = "gate.cap"
	EventGateDebounce       = "gate.debounce"
	EventAgentStart         = "agent.start"
	EventAgentStop          = "agent.stop"
	EventParallelEngaged    = "parallel.engaged"
	EventParallelDisengaged = "parallel.disengaged"
)

// GateEvent represents a parsed JSONL event from the coolant event log.
type GateEvent struct {
	Timestamp  time.Time `json:"ts"`
	Event      string    `json:"event"`
	SessionID  string    `json:"session_id,omitempty"`
	AgentID    string    `json:"agent_id,omitempty"`
	AgentType  string    `json:"agent_type,omitempty"`
	Tool       string    `json:"tool,omitempty"`
	Command    string    `json:"command,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	AgentCount int       `json:"agent_count,omitempty"`
	Threshold  int       `json:"threshold,omitempty"`
	Original   string    `json:"original,omitempty"`
	Rewritten  string    `json:"rewritten,omitempty"`
}

// TailEvents tails the JSONL event file, sending parsed events to ch.
// It polls at the given interval, seeking past previously-read bytes.
// Closes ch when done is closed.
func TailEvents(ch chan<- GateEvent, path string, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	var offset int64
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			offset = readNewLines(ch, path, offset, done)
		}
	}
}

// readNewLines reads any new lines appended since offset, parses them,
// and sends events to ch. Returns the updated offset.
func readNewLines(ch chan<- GateEvent, path string, offset int64, done <-chan struct{}) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset // file doesn't exist yet
	}
	defer f.Close()

	// Handle truncation: if file shrunk, reset to beginning
	info, err := f.Stat()
	if err != nil {
		return offset
	}
	if info.Size() < offset {
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev GateEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // skip malformed lines
		}
		select {
		case ch <- ev:
		case <-done:
			return offset
		}
	}

	// Update offset to current position
	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return offset
	}
	return newOffset
}
