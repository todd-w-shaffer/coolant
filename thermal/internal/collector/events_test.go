package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailEventsReadsNewLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// Write two events
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"ts":"2026-04-04T10:00:00Z","event":"gate.suppress","command":"tsc"}` + "\n")
	f.WriteString(`{"ts":"2026-04-04T10:00:01Z","event":"agent.start","agent_type":"Explore"}` + "\n")
	f.Close()

	ch := make(chan GateEvent, 16)
	done := make(chan struct{})

	go TailEvents(ch, path, 50*time.Millisecond, done)

	// Collect events with timeout
	var events []GateEvent
	timeout := time.After(2 * time.Second)
	for len(events) < 2 {
		select {
		case ev := <-ch:
			events = append(events, ev)
		case <-timeout:
			t.Fatalf("timed out waiting for events, got %d", len(events))
		}
	}
	close(done)

	if events[0].Event != "gate.suppress" {
		t.Errorf("expected gate.suppress, got %s", events[0].Event)
	}
	if events[0].Command != "tsc" {
		t.Errorf("expected command tsc, got %s", events[0].Command)
	}
	if events[1].Event != "agent.start" {
		t.Errorf("expected agent.start, got %s", events[1].Event)
	}
	if events[1].AgentType != "Explore" {
		t.Errorf("expected Explore, got %s", events[1].AgentType)
	}
}

func TestTailEventsHandlesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.jsonl")

	ch := make(chan GateEvent, 16)
	done := make(chan struct{})

	go TailEvents(ch, path, 50*time.Millisecond, done)

	// Wait a bit — should not crash
	time.Sleep(200 * time.Millisecond)

	// Now create the file with an event
	f, _ := os.Create(path)
	f.WriteString(`{"ts":"2026-04-04T10:00:00Z","event":"test.late"}` + "\n")
	f.Close()

	timeout := time.After(2 * time.Second)
	select {
	case ev := <-ch:
		if ev.Event != "test.late" {
			t.Errorf("expected test.late, got %s", ev.Event)
		}
	case <-timeout:
		t.Fatal("timed out waiting for late event")
	}
	close(done)
}

func TestTailEventsSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	f, _ := os.Create(path)
	f.WriteString("this is not json\n")
	f.WriteString(`{"ts":"2026-04-04T10:00:00Z","event":"valid"}` + "\n")
	f.WriteString("{broken json\n")
	f.Close()

	ch := make(chan GateEvent, 16)
	done := make(chan struct{})

	go TailEvents(ch, path, 50*time.Millisecond, done)

	timeout := time.After(2 * time.Second)
	select {
	case ev := <-ch:
		if ev.Event != "valid" {
			t.Errorf("expected valid, got %s", ev.Event)
		}
	case <-timeout:
		t.Fatal("timed out waiting for valid event")
	}

	// Should not receive more events
	select {
	case ev := <-ch:
		t.Errorf("unexpected extra event: %s", ev.Event)
	case <-time.After(200 * time.Millisecond):
		// good — no extra events
	}
	close(done)
}

func TestTailEventsParsesEnrichedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Enriched agent.start with cwd, project, and permission_mode
	f.WriteString(`{"ts":"2026-04-20T20:00:00Z","event":"agent.start","session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/myproject","project":"myproject","permission_mode":"auto","agent_count":1}` + "\n")
	// Enriched agent.stop with transcript_path
	f.WriteString(`{"ts":"2026-04-20T20:01:00Z","event":"agent.stop","session_id":"s1","agent_id":"a1","agent_type":"Explore","cwd":"/Users/dev/myproject","project":"myproject","permission_mode":"auto","transcript_path":"/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl","agent_count":0}` + "\n")
	// Old-format event without new fields (backwards compat)
	f.WriteString(`{"ts":"2026-04-14T10:00:00Z","event":"agent.start","session_id":"s2","agent_id":"a2","agent_type":"Plan","agent_count":1}` + "\n")
	f.Close()

	ch := make(chan GateEvent, 16)
	done := make(chan struct{})

	go TailEvents(ch, path, 50*time.Millisecond, done)

	var events []GateEvent
	timeout := time.After(2 * time.Second)
	for len(events) < 3 {
		select {
		case ev := <-ch:
			events = append(events, ev)
		case <-timeout:
			t.Fatalf("timed out waiting for events, got %d", len(events))
		}
	}
	close(done)

	// Enriched start
	if events[0].Cwd != "/Users/dev/myproject" {
		t.Errorf("start cwd: got %q, want /Users/dev/myproject", events[0].Cwd)
	}
	if events[0].Project != "myproject" {
		t.Errorf("start project: got %q, want myproject", events[0].Project)
	}
	if events[0].PermissionMode != "auto" {
		t.Errorf("start permission_mode: got %q, want auto", events[0].PermissionMode)
	}

	// Enriched stop
	if events[1].TranscriptPath != "/Users/dev/.claude/projects/abc/subagents/agent-a1.jsonl" {
		t.Errorf("stop transcript_path: got %q", events[1].TranscriptPath)
	}
	if events[1].Cwd != "/Users/dev/myproject" {
		t.Errorf("stop cwd: got %q", events[1].Cwd)
	}

	// Old-format event — new fields should be zero values
	if events[2].Cwd != "" {
		t.Errorf("old event cwd should be empty, got %q", events[2].Cwd)
	}
	if events[2].Project != "" {
		t.Errorf("old event project should be empty, got %q", events[2].Project)
	}
	if events[2].PermissionMode != "" {
		t.Errorf("old event permission_mode should be empty, got %q", events[2].PermissionMode)
	}
	if events[2].TranscriptPath != "" {
		t.Errorf("old event transcript_path should be empty, got %q", events[2].TranscriptPath)
	}
}

func TestTailEventsHandlesTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// Write initial event
	f, _ := os.Create(path)
	f.WriteString(`{"ts":"2026-04-04T10:00:00Z","event":"before"}` + "\n")
	f.Close()

	ch := make(chan GateEvent, 16)
	done := make(chan struct{})

	go TailEvents(ch, path, 50*time.Millisecond, done)

	// Wait for first event
	timeout := time.After(2 * time.Second)
	select {
	case ev := <-ch:
		if ev.Event != "before" {
			t.Errorf("expected before, got %s", ev.Event)
		}
	case <-timeout:
		t.Fatal("timed out waiting for first event")
	}

	// Truncate and write new content (shorter than original)
	f, _ = os.Create(path) // os.Create truncates
	f.WriteString(`{"ts":"2026-04-04T10:00:01Z","event":"after"}` + "\n")
	f.Close()

	select {
	case ev := <-ch:
		if ev.Event != "after" {
			t.Errorf("expected after, got %s", ev.Event)
		}
	case <-timeout:
		t.Fatal("timed out waiting for event after truncation")
	}
	close(done)
}
