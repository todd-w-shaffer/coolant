package stats

import (
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

func TestMostTokensAgentTracked(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	type fold struct {
		id    string
		in    int64
		out   int64
		tools int64
	}
	folds := []fold{
		{"a1", 30, 20, 1},  // 50
		{"a2", 150, 50, 4}, // 200
		{"a3", 80, 20, 2},  // 100
	}
	for i, f := range folds {
		ts := now.Add(time.Duration(i) * time.Second)
		a.Fold(mkEvent(1, collector.EventAgentStart, f.id, "s1", ts), 0)
		a.Fold(mkEvent(1, collector.EventAgentStop, f.id, "s1", ts.Add(time.Minute),
			WithTokens(f.in, f.out), WithToolCalls(f.tools)), 0)
	}

	rec := a.Snapshot().Records.MostTokensAgent
	if len(rec) != 3 {
		t.Fatalf("MostTokensAgent length: want 3, got %d", len(rec))
	}
	if rec[0].Value != 200 || rec[0].AgentID != "a2" {
		t.Errorf("MostTokensAgent.Top: want a2/200, got %s/%d", rec[0].AgentID, rec[0].Value)
	}
	if rec[1].Value != 100 || rec[2].Value != 50 {
		t.Errorf("MostTokensAgent ordering: got %d/%d", rec[1].Value, rec[2].Value)
	}
}

func TestMostTokensAgentLeaderboardCappedAtFive(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		id := "a" + string(rune('0'+i))
		ts := now.Add(time.Duration(i) * time.Second)
		a.Fold(mkEvent(1, collector.EventAgentStart, id, "s1", ts), 0)
		a.Fold(mkEvent(1, collector.EventAgentStop, id, "s1", ts.Add(time.Minute),
			WithTokens(int64(100*(i+1)), 0)), 0)
	}
	rec := a.Snapshot().Records.MostTokensAgent
	if len(rec) != recordListCap {
		t.Errorf("MostTokensAgent should cap at %d, got %d", recordListCap, len(rec))
	}
	want := []int64{700, 600, 500, 400, 300}
	for i, v := range want {
		if rec[i].Value != v {
			t.Errorf("rec[%d].Value: want %d, got %d", i, v, rec[i].Value)
		}
	}
}

func TestMostToolCallsAgentTracked(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	folds := []struct {
		id    string
		tools int64
	}{
		{"a1", 5},
		{"a2", 22}, // top
		{"a3", 11},
	}
	for i, f := range folds {
		ts := now.Add(time.Duration(i) * time.Second)
		a.Fold(mkEvent(1, collector.EventAgentStart, f.id, "s1", ts), 0)
		a.Fold(mkEvent(1, collector.EventAgentStop, f.id, "s1", ts.Add(time.Minute),
			WithToolCalls(f.tools)), 0)
	}
	rec := a.Snapshot().Records.MostToolCallsAgent
	if rec.Top().Value != 22 || rec.Top().AgentID != "a2" {
		t.Errorf("MostToolCallsAgent.Top: want a2/22, got %s/%d", rec.Top().AgentID, rec.Top().Value)
	}
}

func TestZeroTelemetryDoesNotEnterLeaderboard(t *testing.T) {
	// An agent.stop with literal 0 telemetry (parse succeeded, no
	// activity) shouldn't poison the leaderboard with a 0-value
	// entry — empty leaderboards stay empty.
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute),
		WithTokens(0, 0), WithToolCalls(0)), 0)
	if got := len(a.Snapshot().Records.MostTokensAgent); got != 0 {
		t.Errorf("zero telemetry entered MostTokensAgent: %d entries", got)
	}
	if got := len(a.Snapshot().Records.MostToolCallsAgent); got != 0 {
		t.Errorf("zero telemetry entered MostToolCallsAgent: %d entries", got)
	}
}

func TestMkEventVariadicComposesTokensAndToolCalls(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	ev := mkEvent(1, collector.EventAgentStop, "a1", "s1", now,
		WithTokens(100, 200), WithToolCalls(15))
	if ev.TokensIn != 100 || ev.TokensOut != 200 {
		t.Errorf("WithTokens: got in=%d out=%d", ev.TokensIn, ev.TokensOut)
	}
	if ev.ToolCallCount != 15 {
		t.Errorf("WithToolCalls: got %d", ev.ToolCallCount)
	}
}
