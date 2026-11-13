package collector

import (
	"strconv"
	"strings"
	"testing"
)

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
		"100 1 claude\n" +
			"200 100 node\n" +
			"300 100 vitest\n" +
			"400 200 grep\n" + // grandchild of claude via node
			"500 1 launchd\n", // unrelated process
	)
	pc := newProcCollector()
	sessions, flatProcs := pc.buildTrees(psFixture(ps))

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

	// Verify classification.
	pidMap := map[int]ProcessInfo{}
	for _, p := range s.Descendants {
		pidMap[p.PID] = p
	}
	node := pidMap[200]
	if node.TypeCode != "N" {
		t.Errorf("node TypeCode = %q, want N", node.TypeCode)
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
		"10 1 claude\n" +
			"20 10 node\n" +
			"30 1 claude-code\n" +
			"40 30 bash\n",
	)
	pc := newProcCollector()
	sessions, flatProcs := pc.buildTrees(psFixture(ps))

	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	if len(flatProcs) != 2 {
		t.Errorf("flatProcs = %d, want 2 (node + bash)", len(flatProcs))
	}
}

func TestBuildTreesNoClaude(t *testing.T) {
	ps := []byte(
		"1 0 launchd\n" +
			"100 1 node\n" +
			"200 1 bash\n",
	)
	pc := newProcCollector()
	sessions, flatProcs := pc.buildTrees(psFixture(ps))

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
		"7923 1 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"7982 7923 /Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper\n" +
			"8003 7923 /Applications/Claude.app/Contents/Frameworks/Claude Helper (Renderer).app/Contents/MacOS/Claude Helper (Renderer)\n" +
			"92690 4473 claude\n" + // real CLI session
			"200 92690 node\n", // CLI child
	)
	pc := newProcCollector()
	sessions, flatProcs := pc.buildTrees(psFixture(ps))

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
		"7923 1 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"7982 7923 /Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper\n" +
			"92690 4473 claude\n",
	)
	pc := newProcCollector()
	sessions, _ := pc.buildTrees(psFixture(ps))

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
		"77254 77137 /Applications/Claude.app/Contents/Helpers/chrome-native-host\n" +
			"92690 4473 claude\n",
	)
	pc := newProcCollector()
	pc.buildTrees(psFixture(ps))

	if pc.DesktopRunning {
		t.Error("DesktopRunning = true, want false (only chrome host, not Desktop)")
	}
	if !pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = false, want true")
	}
}

func TestBuildTreesBothDesktopAndChrome(t *testing.T) {
	ps := []byte(
		"7923 1 /Applications/Claude.app/Contents/MacOS/Claude\n" +
			"77254 77137 /Applications/Claude.app/Contents/Helpers/chrome-native-host\n" +
			"92690 4473 claude\n",
	)
	pc := newProcCollector()
	pc.buildTrees(psFixture(ps))

	if !pc.DesktopRunning {
		t.Error("DesktopRunning = false, want true")
	}
	if !pc.ChromeHostRunning {
		t.Error("ChromeHostRunning = false, want true")
	}
}

func TestBuildTreesNoDesktop(t *testing.T) {
	ps := []byte(
		"92690 4473 claude\n" +
			"200 92690 node\n",
	)
	pc := newProcCollector()
	pc.buildTrees(psFixture(ps))

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
		"30 1 claude-code\n" +
			"40 30 bash\n",
	)
	pc := newProcCollector()
	sessions, _ := pc.buildTrees(psFixture(ps))
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
}

func TestBuildTreesEmptyInput(t *testing.T) {
	pc := newProcCollector()
	sessions, flatProcs := pc.buildTrees(psFixture([]byte("")))
	if len(sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(sessions))
	}
	if len(flatProcs) != 0 {
		t.Errorf("flatProcs = %d, want 0", len(flatProcs))
	}
}

// psFixture turns "pid ppid comm" lines into the rawProc list
// buildTrees consumes, so tree fixtures stay readable. comm may
// contain spaces.
func psFixture(out []byte) []rawProc {
	var procs []rawProc
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		procs = append(procs, rawProc{pid: pid, ppid: ppid, comm: strings.Join(f[2:], " ")})
	}
	return procs
}
