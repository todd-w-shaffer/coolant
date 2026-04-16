package layout

import (
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

var testTheme = theme.Classic()
var testAnim = anim.Default()

func seedActive(h *Horizontal) {
	h.SetSize(120, 10)
	snap := collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    25,
			MemUsedBytes:  8 << 30,
			MemTotalBytes: 16 << 30,
			NCPUs:         8,
		},
		Sessions: []collector.SessionTree{{RootPID: 1, RootComm: "claude"}},
		AllProcs: []collector.ProcessInfo{
			{PID: 2, PPID: 1, TypeCode: "S", Comm: "bash"},
		},
		Online:    true,
		Timestamp: time.Now(),
	}
	h.State().Update(snap)
	h.Update(h.State())
}

func TestViewEmptyWhenZeroSize(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	if got := h.View(); got != "" {
		t.Errorf("View with zero size = %q, want empty", got)
	}
}

func TestViewIdleWhenNoSessions(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)
	snap := collector.Snapshot{
		Online:    true,
		Timestamp: time.Now(),
	}
	h.State().Update(snap)
	h.Update(h.State())

	got := h.View()
	if !strings.Contains(got, "coolant") {
		t.Errorf("idle view should contain 'coolant', got %q", got)
	}
}

func TestViewActiveWithSessions(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActive(h)

	got := h.View()
	lines := strings.Split(got, "\n")
	if len(lines) != 10 {
		t.Errorf("active view lines = %d, want 10 (height)", len(lines))
	}
}

func TestViewLinecountMatchesHeight(t *testing.T) {
	for _, height := range []int{1, 3, 5, 8, 10, 15} {
		h := NewHorizontal(testTheme, testAnim, keys.Default())
		h.SetSize(120, height)
		snap := collector.Snapshot{
			System:    collector.SystemStats{MemTotalBytes: 16 << 30, NCPUs: 8},
			Sessions:  []collector.SessionTree{{RootPID: 1}},
			AllProcs:  []collector.ProcessInfo{{PID: 2, TypeCode: "S"}},
			Online:    true,
			Timestamp: time.Now(),
		}
		h.State().Update(snap)
		h.Update(h.State())

		got := h.View()
		lines := strings.Split(got, "\n")
		if len(lines) != height {
			t.Errorf("height=%d: got %d lines, want %d", height, len(lines), height)
		}
	}
}

func TestToggleCollapse(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	if h.IsCollapsed() {
		t.Error("should not be collapsed by default")
	}
	h.ToggleCollapse()
	if !h.IsCollapsed() {
		t.Error("should be collapsed after toggle")
	}
	h.ToggleCollapse()
	if h.IsCollapsed() {
		t.Error("should not be collapsed after second toggle")
	}
}

func TestToggleHelp(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActive(h)

	before := h.View()
	h.ToggleHelp()
	after := h.View()

	if before == after {
		t.Error("help toggle should change output")
	}
}

func TestNotificationBarHiddenWhenPluginActive(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.State().PluginActive = true
	got := h.notificationBar()
	if got != "" {
		t.Errorf("notification bar should be empty when plugin active, got %q", got)
	}
}

func TestNotificationBarShowsInstallHint(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.State().PluginActive = false
	got := h.notificationBar()
	if !strings.Contains(got, "install") {
		t.Errorf("notification bar should mention install, got %q", got)
	}
}

func TestNotificationBarShowsUpdateAvailable(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.State().PluginActive = true
	h.State().UpdateAvailable = true
	got := h.notificationBar()
	if !strings.Contains(got, "update available") {
		t.Errorf("notification bar should show update hint, got %q", got)
	}
}

func TestNotificationBarHiddenWhenNoHints(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.State().PluginActive = true
	h.State().UpdateAvailable = false
	got := h.notificationBar()
	if got != "" {
		t.Errorf("notification bar should be empty, got %q", got)
	}
}

func TestNotificationBarShowsBothHints(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.State().PluginActive = false
	h.State().UpdateAvailable = true
	got := h.notificationBar()
	if !strings.Contains(got, "install") {
		t.Errorf("should show install hint, got %q", got)
	}
	if !strings.Contains(got, "update available") {
		t.Errorf("should show update hint, got %q", got)
	}
}
