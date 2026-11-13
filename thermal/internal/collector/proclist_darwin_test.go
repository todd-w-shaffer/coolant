package collector

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSysProcLister_FindsSelf(t *testing.T) {
	l := newProcLister(&sysProcs{}, time.Now)
	self, ok := byPIDOf(l.list())[os.Getpid()]
	if !ok {
		t.Fatal("own pid not listed")
	}
	if self.ppid != os.Getppid() {
		t.Errorf("ppid = %d, want %d", self.ppid, os.Getppid())
	}
	if self.comm != os.Args[0] {
		t.Errorf("comm = %q, want argv0 %q", self.comm, os.Args[0])
	}
}

// TestSysProcLister_MatchesPS checks the libproc view agrees with ps
// on pid, ppid, and comm.
func TestSysProcLister_MatchesPS(t *testing.T) {
	l := newProcLister(&sysProcs{}, time.Now)
	ours := byPIDOf(l.list())
	out, err := exec.Command("ps", "-Ao", "pid=,ppid=,comm=").Output()
	if err != nil {
		t.Skipf("ps unavailable: %v", err)
	}

	compared, mismatched := 0, 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		comm := strings.Join(f[2:], " ")
		p, ok := ours[pid]
		if !ok {
			continue // raced: exited or spawned between the two scans
		}
		compared++
		if p.ppid != ppid || strings.Join(strings.Fields(p.comm), " ") != comm {
			mismatched++
			if mismatched <= 5 {
				t.Logf("pid %d: ps=(%d %q) libproc=(%d %q)", pid, ppid, comm, p.ppid, p.comm)
			}
		}
	}
	if compared < 50 {
		t.Fatalf("only %d processes in common with ps", compared)
	}
	if mismatched*100 > compared {
		t.Errorf("%d/%d processes disagree with ps (>1%%)", mismatched, compared)
	}
}

// BenchmarkProcCollector_Collect measures one warm fast-loop process
// scan; BenchmarkPS is the equivalent ps -Ao listing, for scale.
func BenchmarkProcCollector_Collect(b *testing.B) {
	pc := &ProcCollector{children: map[int][]int{}, byPID: map[int]rawProc{}}
	pc.Collect()
	b.ResetTimer()
	for b.Loop() {
		pc.Collect()
	}
}

func BenchmarkPS(b *testing.B) {
	for b.Loop() {
		if _, err := exec.Command("ps", "-Ao", "pid=,ppid=,pcpu=,rss=,comm=").Output(); err != nil {
			b.Skip(err)
		}
	}
}
