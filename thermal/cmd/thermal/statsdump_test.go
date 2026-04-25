package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

func TestStatsdumpFoldsSchema1Events(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "events.jsonl")
	// Two schema:1 starts, one pre-versioning start (should be filtered).
	lines := []string{
		`{"ts":"2026-04-25T10:00:00Z","schema":1,"event":"agent.start","agent_id":"a1","session_id":"s1","agent_type":"Explore","project":"coolant"}`,
		`{"ts":"2026-04-25T10:00:01Z","schema":1,"event":"agent.start","agent_id":"a2","session_id":"s1","agent_type":"Explore","project":"coolant"}`,
		// No schema field — pre-versioning, gated out.
		`{"ts":"2026-04-24T10:00:00Z","event":"agent.start","agent_id":"a0","session_id":"s0","agent_type":"Plan"}`,
	}
	if err := os.WriteFile(jsonl, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	var buf bytes.Buffer
	folded, err := runStatsdump(&buf, stats.Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    jsonl,
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	if err != nil {
		t.Fatalf("runStatsdump: %v", err)
	}
	if folded != 2 {
		t.Errorf("folded count: want 2 (schema:1 only), got %d", folded)
	}

	var snap stats.Snapshot
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		t.Fatalf("snapshot output not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if got := snap.Lifetime().AgentsStarted; got != 2 {
		t.Errorf("AgentsStarted: want 2, got %d", got)
	}
	if snap.ByType["Explore"] != 2 {
		t.Errorf("ByType[Explore]: want 2, got %d", snap.ByType["Explore"])
	}
	if _, gated := snap.ByType["Plan"]; gated {
		t.Errorf("Plan should have been filtered by schema gate")
	}
}

func TestStatsdumpHandlesMissingJSONL(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	folded, err := runStatsdump(&buf, stats.Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "no-such-events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	if err != nil {
		t.Fatalf("missing JSONL should not error: %v", err)
	}
	if folded != 0 {
		t.Errorf("folded count for missing JSONL: want 0, got %d", folded)
	}
	// Output should still be a valid empty snapshot.
	var snap stats.Snapshot
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		t.Fatalf("snapshot output not valid JSON: %v", err)
	}
}
