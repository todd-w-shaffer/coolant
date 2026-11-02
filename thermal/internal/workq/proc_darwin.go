package workq

/*
#include <libproc.h>
#include <stdlib.h>
#include <sys/resource.h>

static unsigned long long workq_footprint(int pid) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) return 0;
	return ri.ri_phys_footprint;
}

static int workq_ppid(int pid) {
	struct proc_bsdinfo bi;
	if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &bi, sizeof(bi)) != sizeof(bi)) return -1;
	return (int)bi.pbi_ppid;
}

static double workq_load1(void) {
	double l[1];
	if (getloadavg(l, 1) != 1) return -1;
	return l[0];
}
*/
import "C"

import (
	"os"
	"unsafe"
)

// IsAncestor reports whether pid is a strict ancestor of this process,
// walking the parent chain through proc_pidinfo. It is how a nested
// `thermo run` proves the ticket it inherited belongs to its own
// process tree, so a ticket leaked into an unrelated process never
// passes.
func IsAncestor(pid int) bool {
	if pid <= 1 {
		return false
	}
	cur := os.Getppid()
	for i := 0; i < 256 && cur > 1; i++ {
		if cur == pid {
			return true
		}
		cur = int(C.workq_ppid(C.int(cur)))
	}
	return false
}

// TreeFootprint answers the summed phys_footprint (proc_pid_rusage, the
// figure Activity Monitor's Memory column shows) of root and its live
// descendants. Footprint, not RSS, so shared library pages are not
// counted once per process.
func TreeFootprint(root int) uint64 {
	buf := make([]C.int, 512)
	bufBytes := C.int(len(buf) * int(unsafe.Sizeof(buf[0])))
	total := uint64(C.workq_footprint(C.int(root)))
	queue := []int{root}
	for seen := 0; len(queue) > 0 && seen < 4096; seen++ {
		p := queue[0]
		queue = queue[1:]
		n := min(int(C.proc_listchildpids(C.int(p), unsafe.Pointer(&buf[0]), bufBytes)), len(buf))
		for _, c := range buf[:max(n, 0)] {
			if c > 0 {
				total += uint64(C.workq_footprint(c))
				queue = append(queue, int(c))
			}
		}
	}
	return total
}

// LoadAverage answers the 1-minute load average, or -1 when unreadable.
func LoadAverage() float64 {
	return float64(C.workq_load1())
}
