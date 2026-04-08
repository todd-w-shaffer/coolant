package model

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

func TestEstimateHeadroomKnownTypes(t *testing.T) {
	typeCounts := map[string]int{
		"V": 2, // 2 * 1GB = 2GB
		"S": 3, // 3 * 20MB = 60MB
	}
	memTotal := testMemTotal
	memUsed := 8 * int64(GB)

	info := EstimateHeadroom(typeCounts, memUsed, memTotal)

	wantCommitted := 2*WeightClass["V"] + 3*WeightClass["S"]
	if info.EstCommitted != wantCommitted {
		t.Errorf("EstCommitted = %d, want %d", info.EstCommitted, wantCommitted)
	}
	if info.MemAvailBytes != memTotal-memUsed {
		t.Errorf("MemAvailBytes = %d, want %d", info.MemAvailBytes, memTotal-memUsed)
	}
	if info.HeavyProcs != 2 {
		t.Errorf("HeavyProcs = %d, want 2", info.HeavyProcs)
	}
	wantHeavy := 2 * WeightClass["V"]
	if info.HeavyEstimate != wantHeavy {
		t.Errorf("HeavyEstimate = %d, want %d", info.HeavyEstimate, wantHeavy)
	}
}

func TestEstimateHeadroomNegativeOverCommitted(t *testing.T) {
	typeCounts := map[string]int{
		"V": 10, // 10GB committed
		"N": 5,  // 5GB committed — total 15GB
	}
	memTotal := testMemTotal
	memUsed := 14 * int64(GB) // only 2GB available

	info := EstimateHeadroom(typeCounts, memUsed, memTotal)

	if info.HeadroomBytes >= 0 {
		t.Errorf("HeadroomBytes = %d, want negative (over-committed)", info.HeadroomBytes)
	}
	if !strings.Contains(info.Warning, "over-committed") {
		t.Errorf("Warning = %q, want 'over-committed' substring", info.Warning)
	}
}

func TestEstimateHeadroomTight(t *testing.T) {
	// Headroom between crit and warn: should produce a warning
	typeCounts := map[string]int{
		"V": 1, // 1GB committed
	}
	memTotal := testMemTotal
	// Set memUsed so available - committed is between HeadroomCritBytes and HeadroomWarnBytes
	wantHeadroom := int64(config.HeadroomCritBytes + GB/2) // 2.5GB — above crit, below warn
	memUsed := memTotal - (wantHeadroom + WeightClass["V"])

	info := EstimateHeadroom(typeCounts, memUsed, memTotal)

	if info.Warning == "" {
		t.Error("expected headroom warning for tight headroom, got empty")
	}
	if strings.Contains(info.Warning, "over-committed") {
		t.Errorf("Warning = %q, should not be over-committed", info.Warning)
	}
}

func TestEstimateHeadroomComfortable(t *testing.T) {
	typeCounts := map[string]int{
		"S": 1, // 20MB committed — negligible
	}
	memTotal := testMemTotal
	memUsed := 4 * int64(GB) // 12GB available, plenty of headroom

	info := EstimateHeadroom(typeCounts, memUsed, memTotal)

	if info.Warning != "" {
		t.Errorf("Warning = %q, want empty for comfortable headroom", info.Warning)
	}
}

func TestEstimateHeadroomUnknownTypeFallsBackToX(t *testing.T) {
	typeCounts := map[string]int{
		"Z": 3, // unknown type — should use WeightClass["X"]
	}
	memTotal := testMemTotal
	memUsed := int64(0)

	info := EstimateHeadroom(typeCounts, memUsed, memTotal)

	wantCommitted := 3 * WeightClass["X"]
	if info.EstCommitted != wantCommitted {
		t.Errorf("EstCommitted for unknown type = %d, want %d (3 * X weight)", info.EstCommitted, wantCommitted)
	}
}

func TestEstimateHeadroomNTypes(t *testing.T) {
	// N is also a heavy proc type
	typeCounts := map[string]int{
		"N": 3,
	}
	info := EstimateHeadroom(typeCounts, 0, testMemTotal)

	if info.HeavyProcs != 3 {
		t.Errorf("HeavyProcs = %d, want 3 for N type", info.HeavyProcs)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0KB"},
		{"1KB", 1024, "1KB"},
		{"512KB", 512 * 1024, "512KB"},
		{"just under 1MB", int64(MB - 1), "1023KB"},
		{"1MB", int64(1 * MB), "1MB"},
		{"512MB", int64(512 * MB), "512MB"},
		{"just under 1GB", int64(GB - 1), "1023MB"},
		{"1GB", int64(1 * GB), "1.0GB"},
		{"2.5GB", int64(2.5 * float64(GB)), "2.5GB"},
		{"10GB", int64(10 * GB), "10.0GB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBytes(tc.bytes); got != tc.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}
