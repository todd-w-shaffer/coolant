package collector

import (
	"testing"
	"time"
)

// fakeProc is one process as the fake kernel reports it.
type fakeProc struct {
	ppid  int
	start time.Time
	short string // p_comm, the kernel's short name
	argv0 string // "" means the argv0 lookup fails
}

// fakeProcSys is a scriptable procSys that counts argv0 lookups.
type fakeProcSys struct {
	procs       map[int]*fakeProc
	argv0Calls  map[int]int
	infoMissing map[int]bool // pid listed but gone by the info call
}

func newFakeProcSys() *fakeProcSys {
	return &fakeProcSys{
		procs:       map[int]*fakeProc{},
		argv0Calls:  map[int]int{},
		infoMissing: map[int]bool{},
	}
}

func (f *fakeProcSys) listPIDs() []int {
	pids := make([]int, 0, len(f.procs))
	for pid := range f.procs {
		pids = append(pids, pid)
	}
	return pids
}

func (f *fakeProcSys) info(pid int) (procInfo, bool) {
	p, ok := f.procs[pid]
	if !ok || f.infoMissing[pid] {
		return procInfo{}, false
	}
	return procInfo{ppid: p.ppid, start: p.start, short: p.short}, true
}

func (f *fakeProcSys) argv0(pid int) (string, bool) {
	f.argv0Calls[pid]++
	p, ok := f.procs[pid]
	if !ok || p.argv0 == "" {
		return "", false
	}
	return p.argv0, true
}

// testClock is a manually advanced clock.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time                { return c.t }
func (c *testClock) advance(d time.Duration)       { c.t = c.t.Add(d) }
func newTestClock() *testClock                     { return &testClock{t: time.Unix(1_000_000, 0)} }
func (c *testClock) ago(d time.Duration) time.Time { return c.t.Add(-d) }

func byPIDOf(procs []rawProc) map[int]rawProc {
	m := make(map[int]rawProc, len(procs))
	for _, p := range procs {
		m[p.pid] = p
	}
	return m
}

func TestProcLister_ReportsTreeFieldsAndArgv0(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "2.1.323", argv0: "claude"}
	sys.procs[200] = &fakeProc{ppid: 100, start: clk.ago(time.Hour), short: "node", argv0: "/usr/local/bin/node"}

	l := newProcLister(sys, clk.now)
	got := byPIDOf(l.list())

	if len(got) != 2 {
		t.Fatalf("listed %d procs, want 2", len(got))
	}
	if got[100].comm != "claude" || got[100].ppid != 1 {
		t.Errorf("pid 100 = %+v, want comm claude ppid 1", got[100])
	}
	if got[200].comm != "/usr/local/bin/node" || got[200].ppid != 100 {
		t.Errorf("pid 200 = %+v, want comm /usr/local/bin/node ppid 100", got[200])
	}
}

func TestProcLister_CachesArgv0ForSettledProcess(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "zsh", argv0: "-zsh"}

	l := newProcLister(sys, clk.now)
	for range 5 {
		l.list()
		clk.advance(150 * time.Millisecond)
	}
	if n := sys.argv0Calls[100]; n != 1 {
		t.Errorf("argv0 lookups = %d, want 1", n)
	}
}

func TestProcLister_RelooksUpAfterExec(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "zsh", argv0: "zsh"}

	l := newProcLister(sys, clk.now)
	l.list()
	// exec keeps pid and start time but swaps the image.
	sys.procs[100].short = "node"
	sys.procs[100].argv0 = "node"
	got := byPIDOf(l.list())

	if got[100].comm != "node" {
		t.Errorf("comm after exec = %q, want node", got[100].comm)
	}
}

func TestProcLister_RelooksUpReusedPID(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "node", argv0: "node"}

	l := newProcLister(sys, clk.now)
	l.list()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Minute), short: "node", argv0: "vite"}
	got := byPIDOf(l.list())

	if got[100].comm != "vite" {
		t.Errorf("comm for reused pid = %q, want vite", got[100].comm)
	}
}

func TestProcLister_CatchesRetitleOfNewProcess(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	// The Claude CLI rewrites its title shortly after launch.
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.now(), short: "2.1.323", argv0: "/opt/claude/2.1.323"}

	l := newProcLister(sys, clk.now)
	l.list()
	clk.advance(500 * time.Millisecond)
	sys.procs[100].argv0 = "claude"
	clk.advance(150 * time.Millisecond)
	got := byPIDOf(l.list())
	if got[100].comm != "claude" {
		t.Errorf("comm after title rewrite = %q, want claude", got[100].comm)
	}
}

func TestProcLister_CatchesLateRetitleWithinRefreshCap(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "node", argv0: "node"}

	l := newProcLister(sys, clk.now)
	l.list()
	sys.procs[100].argv0 = "vite worker"
	var seenAt time.Duration
	for elapsed := time.Duration(0); elapsed <= argv0MaxRefresh+time.Second; elapsed += 150 * time.Millisecond {
		clk.advance(150 * time.Millisecond)
		if byPIDOf(l.list())[100].comm == "vite worker" {
			seenAt = elapsed
			break
		}
	}
	if seenAt == 0 {
		t.Fatalf("late retitle not seen within %v", argv0MaxRefresh+time.Second)
	}
}

func TestProcLister_BacksOffArgv0Reads(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.now(), short: "node", argv0: "node"}

	l := newProcLister(sys, clk.now)
	const scans = 400 // 60s at 150ms
	for range scans {
		l.list()
		clk.advance(150 * time.Millisecond)
	}
	// Back-off doubles the interval from launch up to the 10s cap:
	// roughly a dozen reads ramping up, then one per 10s.
	if n := sys.argv0Calls[100]; n > 25 {
		t.Errorf("argv0 read %d times over %d scans, want back-off (<= 25)", n, scans)
	}
}

func TestProcLister_FallsBackToShortNameWhenArgv0Fails(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "kernel_task"}

	l := newProcLister(sys, clk.now)
	got := byPIDOf(l.list())
	if got[100].comm != "kernel_task" {
		t.Errorf("comm = %q, want kernel_task", got[100].comm)
	}
}

func TestProcLister_SkipsProcessGoneBeforeInfo(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "sh", argv0: "sh"}
	sys.infoMissing[100] = true

	l := newProcLister(sys, clk.now)
	if got := l.list(); len(got) != 0 {
		t.Errorf("listed %d procs, want 0", len(got))
	}
}

func TestProcLister_EvictsExitedProcesses(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	for pid := 100; pid < 110; pid++ {
		sys.procs[pid] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "sh", argv0: "sh"}
	}
	l := newProcLister(sys, clk.now)
	l.list()
	for pid := 100; pid < 109; pid++ {
		delete(sys.procs, pid)
	}
	l.list()
	l.list() // the swapped-out map from the first scan is cleared too
	if n := len(l.cur) + len(l.prev); n != 2 {
		t.Errorf("cache holds %d entries across both scans after 9 exits, want 2", n)
	}
}

func TestProcLister_KeepsLastGoodArgv0WhenRefreshFails(t *testing.T) {
	clk := newTestClock()
	sys := newFakeProcSys()
	sys.procs[100] = &fakeProc{ppid: 1, start: clk.ago(time.Hour), short: "2.1.323", argv0: "claude"}

	l := newProcLister(sys, clk.now)
	l.list()
	sys.procs[100].argv0 = "" // refresh read fails
	clk.advance(argv0MaxRefresh)
	if got := byPIDOf(l.list())[100].comm; got != "claude" {
		t.Errorf("comm after failed refresh = %q, want last good %q", got, "claude")
	}
}
