package collector

import "time"

// argv0MaxRefresh caps how long a cached argv0 goes unread. A process
// can retitle itself at any point (the Claude CLI does so shortly after
// launch, node workers via setproctitle later), so cached names are
// re-read on a back-off: after an interval equal to the process's age
// at the last read, never longer than this cap. A new process is
// re-read nearly every scan; a long-lived one once per cap.
const argv0MaxRefresh = 10 * time.Second

// procInfo is the cheap per-process kernel record (proc_bsdinfo).
type procInfo struct {
	ppid  int
	start time.Time
	short string // p_comm: the kernel's short name, changes on exec
}

// procSys is the kernel surface the lister reads. The darwin
// implementation is sysProcs; tests inject a fake.
type procSys interface {
	listPIDs() []int
	info(pid int) (procInfo, bool)
	// argv0 is the process's first argument, the name root detection
	// matches on. It is the expensive call, which is why it is cached.
	argv0(pid int) (string, bool)
}

// procEntry is one process image's cached name. pid reuse changes
// start; exec changes short. Either invalidates comm.
type procEntry struct {
	start  time.Time
	short  string
	comm   string
	readAt time.Time // when comm was last read
}

// due reports whether comm should be re-read at now.
func (e *procEntry) due(now time.Time) bool {
	interval := min(e.readAt.Sub(e.start), argv0MaxRefresh)
	return now.Sub(e.readAt) >= interval
}

// procLister lists every process's pid, ppid, and comm (argv0, falling
// back to p_comm), reading the kernel directly and caching argv0 per
// process image (see argv0MaxRefresh). Not safe for concurrent use.
type procLister struct {
	sys       procSys
	now       func() time.Time
	cur, prev map[int]*procEntry // this scan's and the last scan's entries by pid
	out       []rawProc          // reused backing slice
}

func newProcLister(sys procSys, now func() time.Time) *procLister {
	return &procLister{
		sys:  sys,
		now:  now,
		cur:  make(map[int]*procEntry, 1024),
		prev: make(map[int]*procEntry, 1024),
	}
}

// list returns one rawProc per live process. The returned slice is
// reused by the next call.
func (l *procLister) list() []rawProc {
	now := l.now()
	l.prev, l.cur = l.cur, l.prev
	clear(l.cur)
	l.out = l.out[:0]

	for _, pid := range l.sys.listPIDs() {
		if pid <= 0 {
			continue
		}
		info, ok := l.sys.info(pid)
		if !ok {
			continue
		}
		e := l.prev[pid]
		if e == nil || e.start != info.start || e.short != info.short {
			e = &procEntry{start: info.start, short: info.short}
		}
		if e.readAt.IsZero() || e.due(now) {
			if a, ok := l.sys.argv0(pid); ok && a != "" {
				e.comm = a
			} else if e.comm == "" {
				e.comm = info.short // first read only; a refresh failure keeps the last good name
			}
			e.readAt = now
		}
		l.cur[pid] = e
		l.out = append(l.out, rawProc{pid: pid, ppid: info.ppid, comm: e.comm})
	}
	return l.out
}
