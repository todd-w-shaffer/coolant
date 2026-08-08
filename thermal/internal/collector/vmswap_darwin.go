package collector

/*
#include <stdlib.h>
#include <mach/mach.h>
#include <mach/host_info.h>
#include <mach/vm_statistics.h>
#include <sys/sysctl.h>
*/
import "C"
import "unsafe"

// vmCounts holds the page counts and lifetime decompression counter read from
// host_statistics64(HOST_VM_INFO64) — the in-process replacement for parsing
// `vm_stat` output. Field meanings mirror the vm_stat lines they replace:
// Active = "Pages active", Wired = "Pages wired down", Compressed = "Pages
// occupied by compressor", Decompressions = the lifetime "Decompressions"
// counter (sampled into a per-tick delta by sampleDecompressions, exactly as
// the subprocess path did).
type vmCounts struct {
	Active         uint64
	Wired          uint64
	Compressed     uint64
	Decompressions uint64
}

// readVMStats64 reads vm_statistics64 directly from the kernel via
// host_statistics64, replacing the per-tick `vm_stat` fork. It reuses the
// process-lifetime hostPort cached in cpu_darwin.go, so — like readTicks — it
// never leaks a mach send right. Returns ok=false on a non-success kernel
// return; CollectSwapVM then leaves the vm fields at zero for that tick,
// matching the prior subprocess path (which zeroed on an empty/failed read).
// A non-success return from this synchronous kernel call is effectively never
// observed in practice, so no last-good retention is layered on.
func readVMStats64() (vmCounts, bool) {
	var info C.vm_statistics64_data_t
	count := C.mach_msg_type_number_t(C.HOST_VM_INFO64_COUNT)
	kr := C.host_statistics64(
		hostPort,
		C.HOST_VM_INFO64,
		C.host_info64_t(unsafe.Pointer(&info)),
		&count,
	)
	if kr != C.KERN_SUCCESS {
		return vmCounts{}, false
	}
	return vmCounts{
		Active:         uint64(info.active_count),
		Wired:          uint64(info.wire_count),
		Compressed:     uint64(info.compressor_page_count),
		Decompressions: uint64(info.decompressions),
	}, true
}

// readSwapUsage reads vm.swapusage directly via sysctlbyname, replacing the
// `sysctl -n vm.swapusage` fork. The kernel reports total/used in bytes (the
// subprocess path parsed the "M" suffix back into bytes), so this returns bytes
// straight from the xsw_usage struct. Returns ok=false on a sysctl failure.
func readSwapUsage() (total, used uint64, ok bool) {
	var xsu C.struct_xsw_usage
	size := C.size_t(unsafe.Sizeof(xsu))
	name := C.CString("vm.swapusage")
	defer C.free(unsafe.Pointer(name))
	if C.sysctlbyname(name, unsafe.Pointer(&xsu), &size, nil, 0) != 0 {
		return 0, 0, false
	}
	return uint64(xsu.xsu_total), uint64(xsu.xsu_used), true
}
