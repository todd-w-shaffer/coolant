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
			got := parseProcessLine([]byte(tt.line))
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
			got := parseProcessLine([]byte(tt.line))
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

		// Go runtime
		{"go", "GO"},

		// Rust runtime
		{"cargo", "RS"},
		{"rustc", "RS"},

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

// newProcCollector creates a ProcCollector with pre-allocated maps for testing.
func newProcCollector() *ProcCollector {
	return &ProcCollector{
		children: make(map[int][]int),
		byPID:    make(map[int]rawProc),
	}
}

func TestBuildTreesSingleSession(t *testing.T) {
	// Synthetic ps output: claude root (PID 100) with two children.
	ps := []byte(
		"  100  1   0.5  4096 claude\n" +
			"  200  100   5.0  8192 node\n" +
			"  300  100   2.0  1024 vitest\n" +
			"  400  200   0.0  512 grep\n" + // grandchild of claude via node
			"  500  1   0.0  256 launchd\n", // unrelated process
	)
	pc := newProcCollector()
	sessions, flatProcs, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.RootPID != 100 {
		t.Errorf("RootPID = %d, want 100", s.RootPID)
	}
	if s.RootComm != "claude" {
		t.Errorf("RootComm = %q, want %q", s.RootComm, "claude")
	}
	// Descendants: node(200), vitest(300), grep(400) — not claude itself, not launchd.
	if len(s.Descendants) != 3 {
		t.Fatalf("descendants = %d, want 3", len(s.Descendants))
	}
	if len(flatProcs) != 3 {
		t.Errorf("flatProcs = %d, want 3", len(flatProcs))
	}

	// Verify classification and RSS conversion (KB → bytes).
	pidMap := map[int]ProcessInfo{}
	for _, p := range s.Descendants {
		pidMap[p.PID] = p
	}
	node := pidMap[200]
	if node.TypeCode != "N" {
		t.Errorf("node TypeCode = %q, want N", node.TypeCode)
	}
	if node.RSSBytes != 8192*1024 {
		t.Errorf("node RSSBytes = %d, want %d", node.RSSBytes, 8192*1024)
	}
	vt := pidMap[300]
	if vt.TypeCode != "V" {
		t.Errorf("vitest TypeCode = %q, want V", vt.TypeCode)
	}
	gr := pidMap[400]
	if gr.TypeCode != "G" {
		t.Errorf("grep TypeCode = %q, want G", gr.TypeCode)
	}
}

func TestBuildTreesMultipleSessions(t *testing.T) {
	ps := []byte(
		"  10  1   0.0  1024 claude\n" +
			"  20  10   1.0  512 node\n" +
			"  30  1   0.0  1024 claude-code\n" +
			"  40  30   2.0  256 bash\n",
	)
	pc := newProcCollector()
	sessions, flatProcs, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	if len(flatProcs) != 2 {
		t.Errorf("flatProcs = %d, want 2 (node + bash)", len(flatProcs))
	}
}

func TestBuildTreesNoClaude(t *testing.T) {
	ps := []byte(
		"  1  0   0.0  0 launchd\n" +
			"  100  1   5.0  8192 node\n" +
			"  200  1   2.0  4096 bash\n",
	)
	pc := newProcCollector()
	sessions, flatProcs, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0 (no claude)", len(sessions))
	}
	if len(flatProcs) != 0 {
		t.Errorf("flatProcs = %d, want 0", len(flatProcs))
	}
}

func TestBuildTreesExcludesDesktopApp(t *testing.T) {
	// Claude Desktop Electron processes should NOT be treated as CLI sessions
	ps := []byte(
		"  7923  1   0.5  104496 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"  7982  7923   0.1  51392 /Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper\n" +
			"  8003  7923   0.2  40544 /Applications/Claude.app/Contents/Frameworks/Claude Helper (Renderer).app/Contents/MacOS/Claude Helper (Renderer)\n" +
			"  92690  4473   1.0  273616 claude\n" + // real CLI session
			"  200  92690   5.0  8192 node\n", // CLI child
	)
	pc := newProcCollector()
	sessions, flatProcs, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}

	// Should only find 1 session (CLI), not 4 (Desktop + helpers + CLI)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 (CLI only, not Desktop)", len(sessions))
	}
	if sessions[0].RootPID != 92690 {
		t.Errorf("RootPID = %d, want 92690 (CLI)", sessions[0].RootPID)
	}
	if len(flatProcs) != 1 {
		t.Errorf("flatProcs = %d, want 1 (node)", len(flatProcs))
	}
}

func TestBuildTreesDesktopDetected(t *testing.T) {
	ps := []byte(
		"  7923  1   0.5  104496 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"  7982  7923   0.1  51392 /Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper\n" +
			"  92690  4473   1.0  273616 claude\n",
	)
	pc := newProcCollector()
	sessions, _, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}

	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	if !pc.DesktopRunning {
		t.Error("DesktopRunning = false, want true")
	}
	if pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = true, want false (no native host)")
	}
}

func TestBuildTreesChromeHostDetected(t *testing.T) {
	ps := []byte(
		"  77254  77137   0.0  4096 /Applications/Claude.app/Contents/Helpers/chrome-native-host\n" +
			"  92690  4473   1.0  273616 claude\n",
	)
	pc := newProcCollector()
	pc.buildTrees(ps)

	if pc.DesktopRunning {
		t.Error("DesktopRunning = true, want false (only chrome host, not Desktop)")
	}
	if !pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = false, want true")
	}
}

func TestBuildTreesBothDesktopAndChrome(t *testing.T) {
	ps := []byte(
		"  7923  1   0.5  104496 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"  77254  77137   0.0  4096 /Applications/Claude.app/Contents/Helpers/chrome-native-host\n" +
			"  92690  4473   1.0  273616 claude\n",
	)
	pc := newProcCollector()
	pc.buildTrees(ps)

	if !pc.DesktopRunning {
		t.Error("DesktopRunning = false, want true")
	}
	if !pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = false, want true")
	}
}

func TestBuildTreesNoDesktop(t *testing.T) {
	ps := []byte(
		"  92690  4473   1.0  273616 claude\n" +
			"  200  92690   5.0  8192 node\n",
	)
	pc := newProcCollector()
	pc.buildTrees(ps)

	if pc.DesktopRunning {
		t.Error("DesktopRunning = true, want false")
	}
	if pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = true, want false")
	}
}

func TestBuildTreesClaudeCodeStillMatches(t *testing.T) {
	// "claude-code" should still match as a CLI session
	ps := []byte(
		"  30  1   0.0  1024 claude-code\n" +
			"  40  30   2.0  256 bash\n",
	)
	pc := newProcCollector()
	sessions, _, err := pc.buildTrees(ps)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
}

func TestBuildTreesEmptyInput(t *testing.T) {
	pc := newProcCollector()
	sessions, flatProcs, err := pc.buildTrees([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(sessions))
	}
	if len(flatProcs) != 0 {
		t.Errorf("flatProcs = %d, want 0", len(flatProcs))
	}
}
