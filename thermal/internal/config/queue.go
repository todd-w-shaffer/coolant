package config

import "fmt"

// Queue modes. Observe records every job and holds none; enforce admits
// jobs against the core budget and memory ceiling. workq.ModeObserve /
// ModeEnforce and workq.Class alias these, so the TOML value, the
// COOLANT_QUEUE_MODE override and the door all speak one vocabulary.
const (
	QueueModeObserve = "observe"
	QueueModeEnforce = "enforce"
)

// QueueConfig is the [queue] section: the machine-wide work queue's
// mode, budget and class weights. Cores and MemoryMB of 0 mean "derive
// from the machine" (hw.ncpu, half of physical RAM) — resolved by the
// door at run time, so one config file travels between machines.
type QueueConfig struct {
	Mode     string
	Cores    int
	MemoryMB int
	Classes  map[string]QueueClass
}

// QueueClass is one job class's weight. A Whole class takes the whole
// core budget, so it runs alone; Cores is then informational.
type QueueClass struct {
	Cores    int
	MemoryMB int
	Whole    bool
}

// defaultQueueClasses are sized for the 8-core / 16 GB machine the
// queue was measured on: one whole vitest suite is ~7 workerd at
// ~250 MB plus ~1.3 GB of node. They are starting weights — a day of
// observe-mode records in ~/.coolant/runs.jsonl is what should set them.
func defaultQueueClasses() map[string]QueueClass {
	return map[string]QueueClass{
		"suite":     {Cores: 8, MemoryMB: 3584, Whole: true},
		"check":     {Cores: 8, MemoryMB: 4608, Whole: true},
		"files":     {Cores: 2, MemoryMB: 1536},
		"typecheck": {Cores: 1, MemoryMB: 1536},
		"lint":      {Cores: 1, MemoryMB: 2048},
		"build":     {Cores: 1, MemoryMB: 512},
	}
}

func defaultQueue() QueueConfig {
	return QueueConfig{Mode: QueueModeObserve, Classes: defaultQueueClasses()}
}

// mergeQueue overlays a raw [queue] table. A [queue.classes.<name>]
// table overrides only the keys it sets on an existing class, or adds
// a new class starting from zero weights.
func mergeQueue(dst *QueueConfig, q map[string]any) {
	if v, ok := q["mode"].(string); ok {
		dst.Mode = v
	}
	coerceInt(&dst.Cores, q, "cores")
	coerceInt(&dst.MemoryMB, q, "memory_mb")
	classes, ok := section(q, "classes")
	if !ok {
		return
	}
	for name := range classes {
		m, ok := section(classes, name)
		if !ok {
			continue
		}
		c := dst.Classes[name]
		coerceInt(&c.Cores, m, "cores")
		coerceInt(&c.MemoryMB, m, "memory_mb")
		coerceBool(&c.Whole, m, "whole")
		dst.Classes[name] = c
	}
}

func validateQueue(q QueueConfig) error {
	if q.Mode != QueueModeObserve && q.Mode != QueueModeEnforce {
		return fmt.Errorf("queue: mode %q must be %q or %q", q.Mode, QueueModeObserve, QueueModeEnforce)
	}
	if q.Cores < 0 || q.MemoryMB < 0 {
		return fmt.Errorf("queue: cores and memory_mb must be non-negative")
	}
	for name, c := range q.Classes {
		if c.Cores < 0 || c.MemoryMB < 0 {
			return fmt.Errorf("queue: class %q weights must be non-negative", name)
		}
	}
	return nil
}
