package main

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func testAnimModel(t *testing.T) animModel {
	t.Helper()
	th, err := theme.Get("classic")
	if err != nil {
		t.Fatalf("theme: %v", err)
	}
	ap, err := anim.Get("default")
	if err != nil {
		t.Fatalf("anim: %v", err)
	}
	return newAnimModel(th, ap)
}

func TestAnimModel_InitialDotCounts(t *testing.T) {
	m := testAnimModel(t)

	tests := []struct {
		name string
		got  int
		want int
	}{
		{"active", m.sections[0].dots.Len(), animActiveDots},
		{"stale", m.sections[1].dots.Len(), animStaleDots},
		{"highscore", m.sections[2].dots.Len(), animHighscoreActive},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s dots = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestAnimModel_ViewContainsSections(t *testing.T) {
	m := testAnimModel(t)
	for i := 0; i < 60; i++ {
		m.tick()
	}
	v := m.view()

	sections := []string{"tidal wave", "kitt scanner", "highscore"}
	for _, label := range sections {
		if !strings.Contains(strings.ToLower(v), label) {
			t.Errorf("view missing section label %q", label)
		}
	}
}

func TestAnimModel_ViewNonEmpty(t *testing.T) {
	m := testAnimModel(t)
	for i := 0; i < 60; i++ {
		m.tick()
	}
	v := m.view()
	if !strings.ContainsAny(v, "⬡⏣⬢") {
		t.Errorf("view contains no agent glyphs")
	}
}
