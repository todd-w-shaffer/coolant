package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// redirectLog must send the standard logger's output to the given file
// so diagnostics (drift guard, collector parse errors, checkpoint
// refusals) can never paint over the bubbletea render. The TUI runs on
// the same stdout/stderr the logger defaults to, so an unredirected
// log.Printf corrupts the screen — see the by_project drift report that
// bled into the dashboard as raw text.
func TestRedirectLogWritesToFile(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	path := filepath.Join(t.TempDir(), "thermo.log")
	f, err := redirectLog(path)
	if err != nil {
		t.Fatalf("redirectLog: %v", err)
	}
	defer f.Close()

	log.Printf("stats: by_project drift detected (lifetime=%d, daily=%d)", 1126, 501)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "by_project drift detected (lifetime=1126, daily=501)") {
		t.Errorf("log file missing redirected line; got %q", string(data))
	}
}

// A second redirect into the same file must append, not truncate — a
// thermo restart should not wipe the prior session's diagnostics.
func TestRedirectLogAppends(t *testing.T) {
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	path := filepath.Join(t.TempDir(), "thermo.log")

	f1, err := redirectLog(path)
	if err != nil {
		t.Fatalf("redirectLog (first): %v", err)
	}
	log.Print("first line")
	f1.Close()

	f2, err := redirectLog(path)
	if err != nil {
		t.Fatalf("redirectLog (second): %v", err)
	}
	defer f2.Close()
	log.Print("second line")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") {
		t.Errorf("expected both lines retained; got %q", got)
	}
}
