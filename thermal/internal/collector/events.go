package collector

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Event name constants — must match the strings emitted by bash hook scripts.
const (
	EventGateSuppress       = "gate.suppress"
	EventGateCap            = "gate.cap"
	EventAgentStart         = "agent.start"
	EventAgentStop          = "agent.stop"
	EventParallelEngaged    = "parallel.engaged"
	EventParallelDisengaged = "parallel.disengaged"
	EventCounterReset       = "counter.reset"
	EventPreflightWarn      = "preflight.warn"
	EventSessionStart       = "session.start"
	EventSessionEnd         = "session.end"
	EventCounterUnderflow   = "counter.underflow"
)

// GateEvent represents a parsed JSONL event from the coolant event log.
type GateEvent struct {
	Timestamp time.Time `json:"ts"`
	// Schema is the envelope shape version. 0 = pre-versioning event
	// (silently filtered by the stats schema gate). 1 = current shape
	// emitted by coolant_event in scripts/common.sh after the
	// schema-and-lock spec landed.
	Schema         int    `json:"schema,omitempty"`
	Event          string `json:"event"`
	SessionID      string `json:"session_id,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	AgentType      string `json:"agent_type,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	Project        string `json:"project,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	Tool           string `json:"tool,omitempty"`
	Command        string `json:"command,omitempty"`
	Reason         string `json:"reason,omitempty"`
	AgentCount     int    `json:"agent_count,omitempty"`
	Threshold      int    `json:"threshold,omitempty"`
	Original       string `json:"original,omitempty"`
	Rewritten      string `json:"rewritten,omitempty"`
	// Per-agent telemetry parsed from the subagent transcript on
	// agent.stop. omitempty because pre-deploy events never carry
	// these fields; the bash emission discipline guarantees that
	// any post-deploy event with a parseable transcript emits all
	// three together (literal 0 for genuinely-zero values).
	TokensIn      int64 `json:"tokens_in,omitempty"`
	TokensOut     int64 `json:"tokens_out,omitempty"`
	ToolCallCount int64 `json:"tool_call_count,omitempty"`
}

// isSessionScoped reports whether an event type is bound to a specific
// session and should therefore be filtered against the configured
// current-session id. Events without session affinity (gate.*,
// parallel.*, counter.reset, preflight.warn) are global signals that
// pass through regardless of the configured session.
func isSessionScoped(event string) bool {
	switch event {
	case EventAgentStart, EventAgentStop,
		EventSessionStart, EventSessionEnd,
		EventCounterUnderflow:
		return true
	}
	return false
}

// TailEvents tails the JSONL event file, sending parsed events to ch.
// It polls at the given interval, seeking past previously-read bytes.
// Closes ch when done is closed. sessionPath, when non-empty, points
// at the sidecar holding the current session id; session-scoped
// events whose session_id doesn't match are dropped before reaching
// ch. An empty sessionPath disables filtering (degraded fallback —
// matches pre-spec behavior).
func TailEvents(ch chan<- GateEvent, path, sessionPath string, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	var (
		offset  int64
		sid     string
		sidModT time.Time
	)
	loadSid := func() {
		if sessionPath == "" {
			return
		}
		info, err := os.Stat(sessionPath)
		if err != nil {
			return
		}
		if !sidModT.IsZero() && info.ModTime().Equal(sidModT) {
			return
		}
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			return
		}
		sid = strings.TrimRight(string(data), "\r\n")
		sidModT = info.ModTime()
	}
	loadSid()

	filter := func(evt GateEvent) bool {
		if sid == "" || !isSessionScoped(evt.Event) {
			return true
		}
		return evt.SessionID == sid
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	handler := func(line []byte) {
		var ev GateEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return // skip malformed lines
		}
		if !filter(ev) {
			return
		}
		select {
		case ch <- ev:
		case <-done:
			return
		}
	}

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			loadSid()
			offset = TailFile(path, offset, handler, done)
		}
	}
}
