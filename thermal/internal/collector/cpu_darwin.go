package collector

/*
#include <mach/mach.h>
#include <mach/host_info.h>
*/
import "C"
import (
	"sync"
	"unsafe"
)

// cpuTicks holds cumulative CPU tick counts from the kernel.
type cpuTicks struct {
	User   uint64
	System uint64
	Idle   uint64
	Nice   uint64
}

func (t cpuTicks) total() uint64 {
	return t.User + t.System + t.Idle + t.Nice
}

func (t cpuTicks) busy() uint64 {
	return t.User + t.System + t.Nice
}

// cpuSampler computes delta-based CPU% between successive calls.
type cpuSampler struct {
	mu   sync.Mutex
	prev cpuTicks
}

var defaultCPUSampler cpuSampler

// readTicks calls host_statistics(HOST_CPU_LOAD_INFO) to get cumulative
// CPU ticks — the same API Activity Monitor uses.
func readTicks() cpuTicks {
	var info C.host_cpu_load_info_data_t
	count := C.mach_msg_type_number_t(C.HOST_CPU_LOAD_INFO_COUNT)

	C.host_statistics(
		C.mach_host_self(),
		C.HOST_CPU_LOAD_INFO,
		C.host_info_t(unsafe.Pointer(&info)),
		&count,
	)

	return cpuTicks{
		User:   uint64(info.cpu_ticks[C.CPU_STATE_USER]),
		System: uint64(info.cpu_ticks[C.CPU_STATE_SYSTEM]),
		Idle:   uint64(info.cpu_ticks[C.CPU_STATE_IDLE]),
		Nice:   uint64(info.cpu_ticks[C.CPU_STATE_NICE]),
	}
}

// SampleCPUPercent returns CPU utilization since the last call, as 0-100.
// First call returns 0 (no previous sample to diff against).
func SampleCPUPercent() float64 {
	cur := readTicks()

	defaultCPUSampler.mu.Lock()
	prev := defaultCPUSampler.prev
	defaultCPUSampler.prev = cur
	defaultCPUSampler.mu.Unlock()

	// First sample — no delta yet
	if prev.total() == 0 {
		return 0
	}

	totalDelta := cur.total() - prev.total()
	if totalDelta == 0 {
		return 0
	}

	busyDelta := cur.busy() - prev.busy()
	pct := float64(busyDelta) / float64(totalDelta) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}
