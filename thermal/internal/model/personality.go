package model

import (
	"embed"
	"encoding/csv"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

//go:embed data/messages.csv
var messagesFS embed.FS

// threatQuips loaded from embedded CSV at init.
var threatQuips map[ThreatLevel][]string

func init() {
	threatQuips = map[ThreatLevel][]string{
		ThreatCool:     {},
		ThreatWarm:     {},
		ThreatHot:      {},
		ThreatMeltdown: {},
	}

	f, err := messagesFS.Open("data/messages.csv")
	if err != nil {
		return
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return
	}

	phaseMap := map[string]ThreatLevel{
		"cool":     ThreatCool,
		"warm":     ThreatWarm,
		"hot":      ThreatHot,
		"meltdown": ThreatMeltdown,
	}

	for i, rec := range records {
		if i == 0 { // skip header
			continue
		}
		if len(rec) < 2 {
			continue
		}
		phase := strings.TrimSpace(strings.ToLower(rec[0]))
		msg := strings.TrimSpace(rec[1])
		if level, ok := phaseMap[phase]; ok {
			threatQuips[level] = append(threatQuips[level], msg)
		}
	}
}

// Idle messages cycle when no Claude processes are detected.
var idleMessages = []string{
	"it's chilly out here without Claude...",
	"bundled up, waiting for some compute...",
	"not a process in sight...",
	"thermostat reads zero...",
	"ice cold, nothing brewing...",
}

// IdleMessage returns a cycling idle message.
func IdleMessage(cycle int) string {
	return idleMessages[cycle%len(idleMessages)]
}

// ThreatQuip returns a personality string for the given threat level,
// prefixed with ":: " for status bar display. Returns a random quip
// by default; pass stable=true for a deterministic first-entry pick
// (use for display that shouldn't flicker every tick).
func ThreatQuip(level ThreatLevel, stable ...bool) string {
	quips := threatQuips[level]
	if len(quips) == 0 {
		return ":: " + level.String()
	}
	if len(stable) > 0 && stable[0] {
		return ":: " + quips[0]
	}
	return ":: " + quips[rand.Intn(len(quips))]
}

// Offline messages — delightful, not alarming.
var offlineMessages = []string{
	"OFFLINE — go touch grass",
	"OFFLINE — enjoy a coffee",
	"OFFLINE — stretch your legs",
	"OFFLINE — look out the window",
	"OFFLINE — take a breath",
}

// OfflineMessage returns a cycling offline message with duration.
func OfflineMessage(dur time.Duration, cycle int) string {
	msg := offlineMessages[cycle%len(offlineMessages)]
	if dur > 0 {
		if dur < time.Minute {
			return fmt.Sprintf("%s (%ds)", msg, int(dur.Seconds()))
		}
		return fmt.Sprintf("%s (%dm)", msg, int(dur.Minutes()))
	}
	return msg
}
