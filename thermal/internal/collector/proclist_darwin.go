package collector

/*
#include <libproc.h>
#include <string.h>
#include <sys/sysctl.h>

static int coolant_bsdinfo(int pid, int *ppid, long long *start_us, char *comm, int commlen) {
	struct proc_bsdinfo bi;
	if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &bi, sizeof(bi)) != sizeof(bi)) return -1;
	*ppid = (int)bi.pbi_ppid;
	*start_us = (long long)bi.pbi_start_tvsec * 1000000LL + (long long)bi.pbi_start_tvusec;
	strncpy(comm, bi.pbi_comm, commlen - 1);
	comm[commlen - 1] = 0;
	return 0;
}

static int coolant_argmax(void) {
	int mib[2] = {CTL_KERN, KERN_ARGMAX};
	int argmax = 0;
	size_t sz = sizeof(argmax);
	if (sysctl(mib, 2, &argmax, &sz, NULL, 0) != 0) return -1;
	return argmax;
}

// coolant_argv0 reads KERN_PROCARGS2 into buf and reports where argv[0]
// sits in it. The layout is [int argc][exec path\0][\0 padding][argv0\0]...
static int coolant_argv0(int pid, char *buf, size_t cap, int *off, int *len) {
	int mib[3] = {CTL_KERN, KERN_PROCARGS2, pid};
	size_t sz = cap;
	if (sysctl(mib, 3, buf, &sz, NULL, 0) != 0 || sz <= sizeof(int)) return -1;
	char *p = buf + sizeof(int), *end = buf + sz;
	while (p < end && *p) p++;
	while (p < end && !*p) p++;
	if (p >= end) return -1;
	*off = (int)(p - buf);
	*len = (int)strnlen(p, (size_t)(end - p));
	return 0;
}

*/
import "C"

import (
	"time"
	"unsafe"
)

// sysProcs reads processes through libproc and sysctl. Its buffers are
// reused across scans, so one sysProcs serves one goroutine, matching
// procLister's contract; separate instances share nothing.
type sysProcs struct {
	cbuf []C.int
	pids []int
	args []byte // KERN_PROCARGS2 scratch, KERN_ARGMAX bytes
}

func (s *sysProcs) listPIDs() []int {
	for {
		if len(s.cbuf) == 0 {
			s.cbuf = make([]C.int, 4096)
		}
		size := C.int(len(s.cbuf) * int(unsafe.Sizeof(s.cbuf[0])))
		n := int(C.proc_listallpids(unsafe.Pointer(&s.cbuf[0]), size))
		if n <= 0 {
			return nil
		}
		if n < len(s.cbuf) {
			s.pids = s.pids[:0]
			for _, pid := range s.cbuf[:n] {
				s.pids = append(s.pids, int(pid))
			}
			return s.pids
		}
		s.cbuf = make([]C.int, 2*len(s.cbuf)) // full buffer may be truncated
	}
}

func (*sysProcs) info(pid int) (procInfo, bool) {
	var ppid C.int
	var startUs C.longlong
	var comm [C.MAXCOMLEN + 1]C.char
	if C.coolant_bsdinfo(C.int(pid), &ppid, &startUs, &comm[0], C.int(len(comm))) != 0 {
		return procInfo{}, false
	}
	return procInfo{
		ppid:  int(ppid),
		start: time.UnixMicro(int64(startUs)),
		short: C.GoString(&comm[0]),
	}, true
}

func (s *sysProcs) argv0(pid int) (string, bool) {
	if s.args == nil {
		n := int(C.coolant_argmax())
		if n <= 0 {
			return "", false
		}
		s.args = make([]byte, n)
	}
	var off, n C.int
	buf := (*C.char)(unsafe.Pointer(&s.args[0]))
	if C.coolant_argv0(C.int(pid), buf, C.size_t(len(s.args)), &off, &n) != 0 {
		return "", false
	}
	return string(s.args[off : off+n]), true
}
