package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sys/unix"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/workq"
)

// The door: `thermo run` and `thermo queue` (reached as `coolant run`
// and `coolant queue` through the scripts/coolant shim).
//
// doorVersionFlag is how the shim asks a thermo whether it has the door.
// It is flag-shaped on purpose: a thermo too old to know it fails
// flag.Parse and exits at once, where a bare word would start the TUI.
const (
	doorRunVerb     = "run"
	doorQueueVerb   = "queue"
	doorVersionFlag = "--door-version"
	doorProtocol    = "1"
)

func runDoor(verb string, args []string) int {
	s := doorSettings(os.Getenv, homeDir(), os.Stderr)
	s.Env = os.Environ()
	s.Stdin, s.Stdout, s.Stderr = os.Stdin, os.Stdout, os.Stderr
	if verb == doorQueueVerb {
		return workq.ShowQueue(s, args, time.Now())
	}
	return workq.Run(context.Background(), s, args)
}

// configPath is the user config: COOLANT_CONFIG, else
// ~/.config/coolant/config.toml. The dashboard and the door share it.
func configPath(getenv func(string) string, home string) string {
	if p := getenv("COOLANT_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(home, ".config", "coolant", "config.toml")
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// doorSettings resolves the door's settings: COOLANT_CONFIG (or
// ~/.config/coolant/config.toml) for mode, budget and class weights,
// COOLANT_QUEUE_MODE / COOLANT_QUEUE_DIR / COOLANT_RUNS as overrides.
// A config that fails to load warns and falls back to the defaults —
// the door never fails a caller over configuration.
func doorSettings(getenv func(string) string, home string, warn io.Writer) workq.Settings {
	cfgPath := configPath(getenv, home)
	cfg := config.Defaults()
	if err := config.Load(cfgPath); err != nil {
		fmt.Fprintf(warn, "coolant: config %s: %v; using queue defaults\n", cfgPath, err)
	} else {
		cfg = config.C
	}
	q := cfg.Queue

	mode := q.Mode
	if m := getenv("COOLANT_QUEUE_MODE"); m != "" {
		if m == workq.ModeObserve || m == workq.ModeEnforce {
			mode = m
		} else {
			fmt.Fprintf(warn, "coolant: ignoring COOLANT_QUEUE_MODE=%s (want observe or enforce)\n", m)
		}
	}

	b := workq.Budget{Cores: q.Cores, MemMB: q.MemoryMB}
	if b.Cores == 0 {
		b.Cores = runtime.NumCPU()
	}
	if b.MemMB == 0 {
		b.MemMB = physicalMB() / 2
	}

	queueDir := getenv("COOLANT_QUEUE_DIR")
	if queueDir == "" && home != "" {
		queueDir = filepath.Join(home, ".coolant", "queue")
	}
	runs := getenv("COOLANT_RUNS")
	if runs == "" && home != "" {
		runs = filepath.Join(home, ".coolant", "runs.jsonl")
	}
	return workq.Settings{
		QueueDir:    queueDir,
		RecordsPath: runs,
		Mode:        mode,
		Budget:      b,
		Classes:     q.Classes,
	}
}

// physicalMB answers hw.memsize in MB, or 8 GB when it cannot be read.
func physicalMB() int {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil || n == 0 {
		return 8192
	}
	return int(n >> 20)
}
