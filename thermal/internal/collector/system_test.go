package collector

import "testing"

func TestParseSwapKnownOutput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTotal int64
		wantUsed  int64
	}{
		{
			name:      "typical macOS output",
			input:     "total = 2048.00M  used = 512.25M  free = 1535.75M  (encrypted)",
			wantTotal: 2048 * 1024 * 1024,
			wantUsed:  int64(512.25 * 1024 * 1024),
		},
		{
			name:      "zero swap",
			input:     "total = 0.00M  used = 0.00M  free = 0.00M  (encrypted)",
			wantTotal: 0,
			wantUsed:  0,
		},
		{
			name:      "large swap values",
			input:     "total = 16384.00M  used = 8500.50M  free = 7883.50M  (encrypted)",
			wantTotal: 16384 * 1024 * 1024,
			wantUsed:  int64(8500.50 * 1024 * 1024),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats SystemStats
			parseSwap(tt.input, &stats)
			if stats.SwapTotalBytes != tt.wantTotal {
				t.Errorf("SwapTotalBytes = %d, want %d", stats.SwapTotalBytes, tt.wantTotal)
			}
			if stats.SwapUsedBytes != tt.wantUsed {
				t.Errorf("SwapUsedBytes = %d, want %d", stats.SwapUsedBytes, tt.wantUsed)
			}
		})
	}
}

func TestParseSwapBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"garbage", "not swap output at all"},
		{"partial match", "total = 100M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats SystemStats
			parseSwap(tt.input, &stats)
			if stats.SwapTotalBytes != 0 {
				t.Errorf("SwapTotalBytes = %d, want 0", stats.SwapTotalBytes)
			}
			if stats.SwapUsedBytes != 0 {
				t.Errorf("SwapUsedBytes = %d, want 0", stats.SwapUsedBytes)
			}
		})
	}
}

func TestParseGPUKnownOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantPct float64
	}{
		{
			name:    "typical ioreg output",
			input:   `    "Device Utilization %"=42`,
			wantPct: 42,
		},
		{
			name:    "zero utilization",
			input:   `    "Device Utilization %"=0`,
			wantPct: 0,
		},
		{
			name:    "full utilization",
			input:   `    "Device Utilization %"=100`,
			wantPct: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats SystemStats
			parseGPU(tt.input, &stats)
			if stats.GPUPercent != tt.wantPct {
				t.Errorf("GPUPercent = %f, want %f", stats.GPUPercent, tt.wantPct)
			}
		})
	}
}

func TestParseGPUBadInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"missing field", `    "Performance Statistics" = {}`},
		{"no match", "some random ioreg output"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats SystemStats
			parseGPU(tt.input, &stats)
			if stats.GPUPercent != 0 {
				t.Errorf("GPUPercent = %f, want 0", stats.GPUPercent)
			}
		})
	}
}

func TestParseVMStatField(t *testing.T) {
	// Real vm_stat output (abbreviated)
	vmstat := `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               12345.
Pages active:                            456789.
Pages inactive:                           98765.
Pages speculative:                         1234.
Pages throttled:                              0.
Pages wired down:                        112233.
Pages purgeable:                           5678.
"Translation faults":                  99887766.
Pages copy-on-write:                    1234567.
Pages zero filled:                     87654321.
Pages reactivated:                       654321.
Pages purged:                            123456.
Decompressions:                         9876543.
Compressions:                           8765432.
Pages occupied by compressor:            334455.
Swapins:                                      0.
Swapouts:                                     0.`

	tests := []struct {
		label string
		want  int64
	}{
		{"Pages active", 456789},
		{"Pages wired down", 112233},
		{"Pages occupied by compressor", 334455},
		{"Decompressions", 9876543},
		{"Pages free", 12345},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := parseVMStatField(vmstat, tt.label)
			if got != tt.want {
				t.Errorf("parseVMStatField(%q) = %d, want %d", tt.label, got, tt.want)
			}
		})
	}
}

func TestParseVMStatFieldMissing(t *testing.T) {
	vmstat := `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               12345.`

	got := parseVMStatField(vmstat, "Nonexistent field")
	if got != 0 {
		t.Errorf("parseVMStatField for missing field = %d, want 0", got)
	}

	got = parseVMStatField("", "Pages active")
	if got != 0 {
		t.Errorf("parseVMStatField on empty input = %d, want 0", got)
	}
}
