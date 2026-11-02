package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/workq"
)

func doorEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDoorSettingsDefaults(t *testing.T) {
	home := t.TempDir()
	var warn bytes.Buffer
	s := doorSettings(doorEnv(map[string]string{"HOME": home}), home, &warn)
	if s.Mode != workq.ModeObserve {
		t.Errorf("mode = %q, want observe", s.Mode)
	}
	if s.Budget.Cores != runtime.NumCPU() || s.Budget.MemMB <= 0 {
		t.Errorf("budget = %+v", s.Budget)
	}
	if s.QueueDir != filepath.Join(home, ".coolant", "queue") || s.RecordsPath != filepath.Join(home, ".coolant", "runs.jsonl") {
		t.Errorf("paths = %s, %s", s.QueueDir, s.RecordsPath)
	}
	if c, ok := s.Classes["suite"]; !ok || !c.Whole {
		t.Errorf("suite class = %+v", c)
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warning %q", warn.String())
	}
}

func TestDoorSettingsReadsConfig(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "c.toml")
	if err := os.WriteFile(cfg, []byte("[queue]\nmode = \"enforce\"\ncores = 5\nmemory_mb = 4000\n[queue.classes.e2e]\ncores = 3\nmemory_mb = 900\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := doorSettings(doorEnv(map[string]string{"COOLANT_CONFIG": cfg}), home, &bytes.Buffer{})
	if s.Mode != workq.ModeEnforce || s.Budget.Cores != 5 || s.Budget.MemMB != 4000 {
		t.Errorf("settings = %+v / %+v", s.Mode, s.Budget)
	}
	if e := s.Classes["e2e"]; e.Cores != 3 || e.MemoryMB != 900 {
		t.Errorf("e2e = %+v", e)
	}
}

func TestDoorSettingsEnvOverridesModeAndPaths(t *testing.T) {
	home := t.TempDir()
	s := doorSettings(doorEnv(map[string]string{
		"COOLANT_QUEUE_MODE": "enforce",
		"COOLANT_QUEUE_DIR":  "/q",
		"COOLANT_RUNS":       "/r.jsonl",
	}), home, &bytes.Buffer{})
	if s.Mode != workq.ModeEnforce || s.QueueDir != "/q" || s.RecordsPath != "/r.jsonl" {
		t.Errorf("settings = %+v", s)
	}
}

func TestDoorSettingsSurvivesABadConfig(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "c.toml")
	if err := os.WriteFile(cfg, []byte("[queue]\nmode = \"panic\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	s := doorSettings(doorEnv(map[string]string{"COOLANT_CONFIG": cfg}), home, &warn)
	if s.Mode != workq.ModeObserve {
		t.Errorf("mode = %q, want the observe default", s.Mode)
	}
	if !strings.Contains(warn.String(), "config") {
		t.Errorf("warning = %q", warn.String())
	}
}

func TestDoorSettingsIgnoresABadModeOverride(t *testing.T) {
	var warn bytes.Buffer
	s := doorSettings(doorEnv(map[string]string{"COOLANT_QUEUE_MODE": "yolo"}), t.TempDir(), &warn)
	if s.Mode != workq.ModeObserve || !strings.Contains(warn.String(), "yolo") {
		t.Errorf("mode = %q, warning = %q", s.Mode, warn.String())
	}
}

func TestDoorVersionIsTheProtocolTheShimProbes(t *testing.T) {
	if doorVersionFlag != "--door-version" || doorProtocol != "1" {
		t.Errorf("probe = %s → %s; the shim scripts/coolant pins both", doorVersionFlag, doorProtocol)
	}
}
