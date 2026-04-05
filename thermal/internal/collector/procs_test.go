package collector

import "testing"

func TestParseProcessLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantPID  int
		wantPPID int
		wantCPU  float64
		wantRSS  int64
		wantComm string
	}{
		{
			name:     "standard ps output",
			line:     "  1234  5678   3.2  8192 node",
			wantPID:  1234,
			wantPPID: 5678,
			wantCPU:  3.2,
			wantRSS:  8192,
			wantComm: "node",
		},
		{
			name:     "comm with path",
			line:     "  999  100   0.0  1024 /usr/local/bin/vitest",
			wantPID:  999,
			wantPPID: 100,
			wantCPU:  0.0,
			wantRSS:  1024,
			wantComm: "/usr/local/bin/vitest",
		},
		{
			name:     "comm with spaces in path",
			line:     "  42  1   12.5  65536 /Applications/Some App.app/Contents/MacOS/helper",
			wantPID:  42,
			wantPPID: 1,
			wantCPU:  12.5,
			wantRSS:  65536,
			wantComm: "/Applications/Some App.app/Contents/MacOS/helper",
		},
		{
			name:     "zero CPU and RSS",
			line:     "  1  0   0.0  0 launchd",
			wantPID:  1,
			wantPPID: 0,
			wantCPU:  0.0,
			wantRSS:  0,
			wantComm: "launchd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProcessLine(tt.line)
			if got.pid != tt.wantPID {
				t.Errorf("pid = %d, want %d", got.pid, tt.wantPID)
			}
			if got.ppid != tt.wantPPID {
				t.Errorf("ppid = %d, want %d", got.ppid, tt.wantPPID)
			}
			if got.cpu != tt.wantCPU {
				t.Errorf("cpu = %f, want %f", got.cpu, tt.wantCPU)
			}
			if got.rss != tt.wantRSS {
				t.Errorf("rss = %d, want %d", got.rss, tt.wantRSS)
			}
			if got.comm != tt.wantComm {
				t.Errorf("comm = %q, want %q", got.comm, tt.wantComm)
			}
		})
	}
}

func TestParseProcessLineShortInput(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty string", ""},
		{"too few fields", "1234 5678 0.0"},
		{"four fields", "1234 5678 0.0 1024"},
		{"whitespace only", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProcessLine(tt.line)
			if got.pid != 0 {
				t.Errorf("pid = %d, want 0 for short input", got.pid)
			}
			if got.comm != "" {
				t.Errorf("comm = %q, want empty for short input", got.comm)
			}
		})
	}
}

func TestClassifyComm(t *testing.T) {
	tests := []struct {
		comm string
		want string
	}{
		// Test runners
		{"vitest", "V"},
		{"jest", "V"},
		{"pytest", "V"},

		// Compilers
		{"tsc", "T"},

		// Build tools
		{"webpack", "B"},
		{"eslint", "B"},
		{"cargo", "B"},
		{"go", "B"},

		// Runtimes
		{"node", "N"},
		{"python3", "P"},

		// Search
		{"rg", "R"},
		{"grep", "G"},
		{"fd", "F"},

		// Shell
		{"bash", "S"},
		{"git", "C"},

		// With path prefix (basename extracted)
		{"/usr/local/bin/node", "N"},
		{"/opt/homebrew/bin/vitest", "V"},

		// Case insensitive
		{"Node", "N"},
		{"VITEST", "V"},
	}

	for _, tt := range tests {
		t.Run(tt.comm, func(t *testing.T) {
			got := classifyComm(tt.comm)
			if got != tt.want {
				t.Errorf("classifyComm(%q) = %q, want %q", tt.comm, got, tt.want)
			}
		})
	}
}

func TestClassifyCommUnknown(t *testing.T) {
	unknowns := []string{"firefox", "spotify", "some-random-binary", ""}
	for _, comm := range unknowns {
		t.Run(comm, func(t *testing.T) {
			got := classifyComm(comm)
			if got != "X" {
				t.Errorf("classifyComm(%q) = %q, want %q", comm, got, "X")
			}
		})
	}
}

func TestBasename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/usr/local/bin/node", "node"},
		{"/a/b/c/d/vitest", "vitest"},
		{"node", "node"},
		{"", ""},
		{"/trailing/slash/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := basename(tt.input)
			if got != tt.want {
				t.Errorf("basename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
