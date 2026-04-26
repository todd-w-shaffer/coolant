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

// TestStatsdumpRecordShapeIsSlice pins the cache-schema-2 contract on
// the user-facing JSON: every Records.* field must encode as an
// array, not a single object. External scripts that parse statsdump
// output (e.g., dashboards, future thermo stats subcommand) rely on
// this shape — a regression flipping back to top-1 objects would
// silently break them.
func TestStatsdumpRecordShapeIsSlice(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "events.jsonl")
	lines := []string{
		`{"ts":"2026-04-25T10:00:00Z","schema":1,"event":"agent.start","agent_id":"a1","session_id":"s1","agent_type":"Explore","project":"coolant"}`,
		`{"ts":"2026-04-25T10:00:30Z","schema":1,"event":"agent.stop","agent_id":"a1","session_id":"s1","agent_type":"Explore","project":"coolant"}`,
	}
	if err := os.WriteFile(jsonl, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	var buf bytes.Buffer
	if _, err := runStatsdump(&buf, stats.Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    jsonl,
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}); err != nil {
		t.Fatalf("runStatsdump: %v", err)
	}

	// Parse as raw map so we can inspect the shape (object vs array)
	// of each Records.* field directly. A v2 dump must show arrays.
	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("statsdump output not valid JSON: %v", err)
	}
	records, ok := raw["records"].(map[string]any)
	if !ok {
		t.Fatalf("records block missing or not a map: %T", raw["records"])
	}
	for _, field := range []string{"peak_concurrent", "longest_agent_s", "longest_session_s", "most_agents_session", "biggest_burst"} {
		v, ok := records[field]
		if !ok {
			t.Errorf("records.%s missing from dump", field)
			continue
		}
		if _, isArr := v.([]any); !isArr {
			t.Errorf("records.%s shape: want []any (v2), got %T", field, v)
		}
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
