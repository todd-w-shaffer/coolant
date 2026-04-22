package keys

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestDefaultKeyMapBindings(t *testing.T) {
	km := Default()

	tests := []struct {
		name     string
		binding  key.Binding
		wantKeys []string
		wantKey  string
		wantDesc string
	}{
		{"Help", km.Help, []string{"h", "?"}, "h/?", "help"},
		{"Quit", km.Quit, []string{"q", "ctrl+c"}, "q", "quit"},
		{"Collapse", km.Collapse, []string{"c"}, "c", "collapse sessions"},
		{"PurgeStale", km.PurgeStale, []string{"x"}, "x", "purge stale agents"},
		{"CategoryPrev", km.CategoryPrev, []string{"["}, "[", "prev category"},
		{"CategoryNext", km.CategoryNext, []string{"]"}, "]", "next category"},
		{"CategoryClear", km.CategoryClear, []string{"\\"}, "\\", "clear filter"},
		{"MouseToggle", km.MouseToggle, []string{"m"}, "m", "toggle mouse"},
		{"Intel", km.Intel, []string{"i"}, "i", "session intel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.binding.Keys(); !reflect.DeepEqual(got, tt.wantKeys) {
				t.Errorf("%s.Keys() = %v, want %v", tt.name, got, tt.wantKeys)
			}
			h := tt.binding.Help()
			if h.Key != tt.wantKey {
				t.Errorf("%s.Help().Key = %q, want %q", tt.name, h.Key, tt.wantKey)
			}
			if h.Desc != tt.wantDesc {
				t.Errorf("%s.Help().Desc = %q, want %q", tt.name, h.Desc, tt.wantDesc)
			}
		})
	}
}

func TestShortHelpOrder(t *testing.T) {
	km := Default()
	short := km.ShortHelp()
	want := []key.Binding{km.Help, km.Quit, km.Collapse, km.PurgeStale, km.CategoryPrev, km.CategoryNext, km.CategoryClear, km.MouseToggle, km.Intel}
	if len(short) != len(want) {
		t.Fatalf("ShortHelp() length = %d, want %d", len(short), len(want))
	}
	for i := range want {
		if short[i].Help() != want[i].Help() {
			t.Errorf("ShortHelp()[%d] = %+v, want %+v", i, short[i].Help(), want[i].Help())
		}
	}
}

func TestFullHelpGrouping(t *testing.T) {
	km := Default()
	full := km.FullHelp()
	if len(full) != 3 {
		t.Fatalf("FullHelp() rows = %d, want 3", len(full))
	}
	want := [][]key.Binding{
		{km.Help, km.Quit},
		{km.Collapse, km.PurgeStale, km.Intel},
		{km.CategoryPrev, km.CategoryNext, km.CategoryClear, km.MouseToggle},
	}
	for r := range want {
		if len(full[r]) != len(want[r]) {
			t.Fatalf("FullHelp()[%d] length = %d, want %d", r, len(full[r]), len(want[r]))
		}
		for c := range want[r] {
			if full[r][c].Help() != want[r][c].Help() {
				t.Errorf("FullHelp()[%d][%d] = %+v, want %+v", r, c, full[r][c].Help(), want[r][c].Help())
			}
		}
	}
}
