package ui

import "testing"

func TestAgentZoneID(t *testing.T) {
	got := AgentZoneID("abc123")
	want := "agent:abc123"
	if got != want {
		t.Errorf("AgentZoneID(%q) = %q, want %q", "abc123", got, want)
	}
}

func TestAgentZoneIDEmpty(t *testing.T) {
	got := AgentZoneID("")
	want := "agent:"
	if got != want {
		t.Errorf("AgentZoneID(%q) = %q, want %q", "", got, want)
	}
}
