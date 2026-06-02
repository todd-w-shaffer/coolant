package layout

import (
	"testing"
	"time"

	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
)

// seedActiveWide builds a realistic active dashboard at full bottom-strip width.
func seedActiveWide(h *Horizontal) {
	h.SetSize(240, 10)
	snap := collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:     47,
			MemUsedBytes:   10 << 30,
			MemTotalBytes:  16 << 30,
			NCPUs:          10,
			Decompressions: 1200,
			GPUPercent:     33,
			SwapUsedBytes:  2 << 30,
		},
		Sessions: []collector.SessionTree{
			{RootPID: 1, RootComm: "claude", Descendants: []collector.ProcessInfo{
				{PID: 2, PPID: 1, TypeCode: "S"},
				{PID: 3, PPID: 1, TypeCode: "N"},
				{PID: 4, PPID: 1, TypeCode: "G"},
			}},
			{RootPID: 5, RootComm: "claude", Descendants: []collector.ProcessInfo{
				{PID: 6, PPID: 5, TypeCode: "V"},
			}},
		},
		AllProcs: []collector.ProcessInfo{
			{PID: 2, PPID: 1, TypeCode: "S"},
			{PID: 3, PPID: 1, TypeCode: "N"},
			{PID: 4, PPID: 1, TypeCode: "G"},
			{PID: 6, PPID: 5, TypeCode: "V"},
		},
		Tokens:    collector.TokenStats{IOTokensPerSec: 500, PrettyTokensPerSec: 480, InputTotal: 100000, OutputTotal: 50000},
		Online:    true,
		Timestamp: time.Now(),
	}
	h.State().Update(snap)
	h.Update(h.State())
	// Warm up render history so sparklines have data to draw.
	for i := 0; i < 60; i++ {
		h.AnimTick()
	}
}

func BenchmarkViewActive(b *testing.B) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.AnimTick()
		_ = h.View()
	}
}

func BenchmarkViewActiveWithZoneScan(b *testing.B) {
	zone.NewGlobal()
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.AnimTick()
		_ = zone.Scan(h.View())
	}
}

func BenchmarkViewIdle(b *testing.B) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(240, 10)
	snap := collector.Snapshot{
		System:    collector.SystemStats{CPUPercent: 5, MemUsedBytes: 8 << 30, MemTotalBytes: 16 << 30, NCPUs: 10},
		Online:    true,
		Timestamp: time.Now(),
	}
	h.State().Update(snap)
	h.Update(h.State())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.AnimTick()
		_ = h.View()
	}
}

func BenchmarkHeadlineViewLines(b *testing.B) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.headline.ViewLines()
	}
}

func BenchmarkGaugesView(b *testing.B) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.gauges.View()
	}
}

func BenchmarkRatesView(b *testing.B) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	seedActiveWide(h)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.rates.View()
	}
}
