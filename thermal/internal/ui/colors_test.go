package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

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

func TestPathZoneID(t *testing.T) {
	got := PathZoneID("abc123")
	want := "path:abc123"
	if got != want {
		t.Errorf("PathZoneID(%q) = %q, want %q", "abc123", got, want)
	}
}

func TestOSC8LinkFraming(t *testing.T) {
	got := OSC8Link("file:///tmp/test.jsonl", "test.jsonl")
	// Must start with OSC 8 open: ESC ] 8 ; ; uri BEL
	wantOpen := "\033]8;;file:///tmp/test.jsonl\a"
	wantClose := "\033]8;;\a"
	if !strings.HasPrefix(got, wantOpen) {
		t.Errorf("OSC8Link missing open sequence\ngot:  %q\nwant prefix: %q", got, wantOpen)
	}
	if !strings.HasSuffix(got, wantClose) {
		t.Errorf("OSC8Link missing close sequence\ngot:  %q\nwant suffix: %q", got, wantClose)
	}
	// Visible text must appear between open and close
	if !strings.Contains(got, "test.jsonl") {
		t.Error("OSC8Link should contain the visible text")
	}
}

func TestOSC8LinkZeroWidth(t *testing.T) {
	text := "/tmp/transcript.jsonl"
	linked := OSC8Link("file:///tmp/transcript.jsonl", text)
	plainW := ansi.StringWidth(text)
	linkedW := ansi.StringWidth(linked)
	if plainW != linkedW {
		t.Errorf("OSC8Link should be zero-width: plain=%d, linked=%d", plainW, linkedW)
	}
}

func TestOSC8LinkUsesBEL(t *testing.T) {
	got := OSC8Link("file:///tmp/f.jsonl", "f.jsonl")
	// BEL (0x07) should appear, not ESC\ (0x1b 0x5c)
	if !strings.Contains(got, "\a") {
		t.Error("OSC8Link should use BEL (\\a) as string terminator")
	}
	if strings.Contains(got, "\033\\") {
		t.Error("OSC8Link should NOT use ESC\\\\ as string terminator")
	}
}
