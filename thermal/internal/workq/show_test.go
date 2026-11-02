package workq

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func seedQueue(t *testing.T, s Settings) {
	t.Helper()
	if err := AppendRecord(s.RecordsPath, Record{Class: "suite", WallS: 240}); err != nil {
		t.Fatal(err)
	}
	q := openQ(t, s.QueueDir)
	h, err := q.Take(context.Background(), Ticket{PID: 111, Lane: "epic", Class: "suite", Whole: true, MemMB: 3000, Cwd: "/w/a", Cmd: "npm run check"}, budget8, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Release)
	takeAsync(t, openQ(t, s.QueueDir), Ticket{PID: 222, Class: "suite", Whole: true, MemMB: 3000, Cwd: "/w/b", Cmd: "vitest run"}, nil)
	waitForWaiters(t, q, 1)
}

func TestShowQueueJSON(t *testing.T) {
	s, out, _ := testSettings(t, ModeEnforce)
	seedQueue(t, s)
	if code := ShowQueue(s, []string{"--json"}, time.Now()); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var v struct {
		Mode   string `json:"mode"`
		Budget struct {
			Cores    int `json:"cores"`
			MemoryMB int `json:"memory_mb"`
		} `json:"budget"`
		Running []HorizonEntry `json:"running"`
		Waiting []HorizonEntry `json:"waiting"`
	}
	if err := json.Unmarshal([]byte(out.String()), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if v.Mode != ModeEnforce || v.Budget.Cores != 8 || v.Budget.MemoryMB != 8192 {
		t.Errorf("header = %+v", v)
	}
	if len(v.Running) != 1 || v.Running[0].PID != 111 || !v.Running[0].Known || v.Running[0].EstimateS != 240 {
		t.Errorf("running = %+v", v.Running)
	}
	if len(v.Waiting) != 1 || v.Waiting[0].PID != 222 || !v.Waiting[0].Known {
		t.Errorf("waiting = %+v", v.Waiting)
	}
}

func TestShowQueueHuman(t *testing.T) {
	s, out, _ := testSettings(t, ModeEnforce)
	seedQueue(t, s)
	ShowQueue(s, nil, time.Now())
	got := out.String()
	for _, want := range []string{"enforce", "8 cores", "running", "epic", "suite", "/w/a", "npm run check", "waiting", "interactive", "/w/b", "~4m"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

func TestShowQueueEmpty(t *testing.T) {
	s, out, _ := testSettings(t, ModeObserve)
	ShowQueue(s, nil, time.Now())
	if !strings.Contains(out.String(), "idle") {
		t.Errorf("empty queue output = %q", out.String())
	}
}
