package model

import "math/rand"

// Idle messages cycle when no Claude processes are detected.
var idleMessages = []string{
	"it's chilly out here without Claude...",
	"bundled up, waiting for some compute...",
	"not a process in sight...",
	"thermostat reads zero...",
	"ice cold, nothing brewing...",
}

// Threat quips shown alongside the threat level.
var threatQuips = map[ThreatLevel][]string{
	ThreatCool: {
		"Claude's humming along nicely",
		"smooth sailing, temps nominal",
		"cool and collected",
	},
	ThreatWarm: {
		"things are warming up...",
		"starting to feel it...",
		"the fans are noticing",
	},
	ThreatHot: {
		"Claude's cooking with gas!",
		"getting spicy in here",
		"thermal paste earning its keep",
	},
	ThreatMeltdown: {
		"Claude's gone nuclear",
		"mayday mayday mayday",
		"everything is fine (it is not fine)",
	},
}

// Alert message templates for threshold crossings.
var alertTemplates = map[string]string{
	"spawn_burst":    "spawn burst detected -- %d new procs",
	"mem_headroom":   "memory headroom below %s",
	"swap_active":    "swap is active -- %s used",
	"phase_up":       "%s -- %s",
	"phase_down":     "cooling down -- %s",
	"session_new":    "new Claude session detected (pid %d)",
	"session_gone":   "Claude session ended (pid %d)",
}

// IdleMessage returns a cycling idle message.
func IdleMessage(cycle int) string {
	return idleMessages[cycle%len(idleMessages)]
}

// ThreatQuip returns a personality string for the given threat level.
func ThreatQuip(level ThreatLevel) string {
	quips := threatQuips[level]
	if len(quips) == 0 {
		return level.String()
	}
	return quips[rand.Intn(len(quips))]
}

// ThreatQuipStable returns the first (deterministic) quip for the level.
// Use this for display that shouldn't flicker every tick.
func ThreatQuipStable(level ThreatLevel) string {
	quips := threatQuips[level]
	if len(quips) == 0 {
		return level.String()
	}
	return quips[0]
}
