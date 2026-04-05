package model

import (
	"strings"
	"testing"
	"time"
)

func TestThreatQuipsLoadedFromCSV(t *testing.T) {
	// The init() in personality.go loads embedded messages.csv into threatQuips.
	// Every threat level should have at least one quip.
	for _, level := range []ThreatLevel{ThreatCool, ThreatWarm, ThreatHot, ThreatMeltdown} {
		quips := threatQuips[level]
		if len(quips) == 0 {
			t.Errorf("threatQuips[%v] is empty — CSV may have failed to load", level)
		}
	}
}

func TestThreatQuipReturnsPrefixed(t *testing.T) {
	for _, level := range []ThreatLevel{ThreatCool, ThreatWarm, ThreatHot, ThreatMeltdown} {
		got := ThreatQuip(level)
		if !strings.HasPrefix(got, ":: ") {
			t.Errorf("ThreatQuip(%v) = %q, want ':: ' prefix", level, got)
		}
	}
}

func TestThreatQuipStableReturnsDeterministic(t *testing.T) {
	// stable=true should always return the same (first) quip.
	first := ThreatQuip(ThreatCool, true)
	for i := 0; i < 10; i++ {
		got := ThreatQuip(ThreatCool, true)
		if got != first {
			t.Errorf("stable quip changed on iteration %d: %q vs %q", i, got, first)
		}
	}
}

func TestThreatQuipUnknownLevelFallback(t *testing.T) {
	// Unknown threat level has no quips — should fall back to level.String().
	got := ThreatQuip(ThreatLevel(99))
	if got != ":: UNKNOWN" {
		t.Errorf("ThreatQuip(99) = %q, want %q", got, ":: UNKNOWN")
	}
}

func TestIdleMessageCycles(t *testing.T) {
	n := len(idleMessages)
	for i := 0; i < n*2; i++ {
		got := IdleMessage(i)
		want := idleMessages[i%n]
		if got != want {
			t.Errorf("IdleMessage(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestOfflineMessageIncludesDuration(t *testing.T) {
	got := OfflineMessage(30*time.Second, 0)
	if !strings.Contains(got, "30s") {
		t.Errorf("OfflineMessage(30s) = %q, want duration in seconds", got)
	}

	got = OfflineMessage(5*time.Minute, 0)
	if !strings.Contains(got, "5m") {
		t.Errorf("OfflineMessage(5m) = %q, want duration in minutes", got)
	}
}

func TestOfflineMessageZeroDuration(t *testing.T) {
	got := OfflineMessage(0, 0)
	if strings.Contains(got, "(") {
		t.Errorf("OfflineMessage(0) = %q, want no duration suffix", got)
	}
}

func TestOfflineMessageCycles(t *testing.T) {
	n := len(offlineMessages)
	for i := 0; i < n*2; i++ {
		got := OfflineMessage(0, i)
		want := offlineMessages[i%n]
		if got != want {
			t.Errorf("OfflineMessage(0, %d) = %q, want %q", i, got, want)
		}
	}
}
