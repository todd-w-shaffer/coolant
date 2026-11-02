package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadQueueTOML(t *testing.T, body string) error {
	t.Helper()
	old := C
	t.Cleanup(func() { C = old })
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestQueueDefaultsShipObserveMode(t *testing.T) {
	d := Defaults()
	if d.Queue.Mode != QueueModeObserve {
		t.Errorf("Queue.Mode = %q, want %q", d.Queue.Mode, QueueModeObserve)
	}
	if d.Queue.Cores != 0 || d.Queue.MemoryMB != 0 {
		t.Errorf("budget = %d cores / %d MB, want 0/0 (machine-derived)", d.Queue.Cores, d.Queue.MemoryMB)
	}
}

func TestQueueDefaultClassesCoverTheBrief(t *testing.T) {
	d := Defaults()
	for _, name := range []string{"suite", "files", "typecheck", "lint", "check", "build"} {
		c, ok := d.Queue.Classes[name]
		if !ok {
			t.Errorf("default class %q missing", name)
			continue
		}
		if c.MemoryMB <= 0 || (!c.Whole && c.Cores <= 0) {
			t.Errorf("class %q has no weight: %+v", name, c)
		}
	}
	if !d.Queue.Classes["suite"].Whole || !d.Queue.Classes["check"].Whole {
		t.Error("suite and check must take the whole budget")
	}
}

func TestQueueDefaultsAreNotShared(t *testing.T) {
	a, b := Defaults(), Defaults()
	a.Queue.Classes["suite"] = QueueClass{Cores: 1}
	if b.Queue.Classes["suite"].Cores == 1 {
		t.Fatal("Defaults() returned a shared classes map")
	}
}

func TestLoadQueueSection(t *testing.T) {
	err := loadQueueTOML(t, `
[queue]
mode = "enforce"
cores = 6
memory_mb = 6000

[queue.classes.files]
cores = 3

[queue.classes.e2e]
cores = 4
memory_mb = 2500
whole = true
`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	q := C.Queue
	if q.Mode != QueueModeEnforce || q.Cores != 6 || q.MemoryMB != 6000 {
		t.Errorf("queue = %+v", q)
	}
	if f := q.Classes["files"]; f.Cores != 3 || f.MemoryMB != Defaults().Queue.Classes["files"].MemoryMB {
		t.Errorf("files override = %+v, want cores 3 and default memory kept", f)
	}
	if e := q.Classes["e2e"]; e.Cores != 4 || e.MemoryMB != 2500 || !e.Whole {
		t.Errorf("new class e2e = %+v", e)
	}
}

func TestValidationRejectsUnknownQueueMode(t *testing.T) {
	if err := loadQueueTOML(t, "[queue]\nmode = \"panic\"\n"); err == nil {
		t.Fatal("expected error for unknown queue mode")
	}
}

func TestValidationRejectsNegativeQueueBudget(t *testing.T) {
	if err := loadQueueTOML(t, "[queue]\ncores = -1\n"); err == nil {
		t.Fatal("expected error for negative core budget")
	}
}
