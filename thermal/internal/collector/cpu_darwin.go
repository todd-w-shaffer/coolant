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
	mu      sync.Mutex
	prev    cpuTicks
	lastPct float64 // held when ticks haven't updated between reads
}

var defaultCPUSampler cpuSampler

// hostPort caches the mach host port to avoid leaking one send right per
// call to mach_host_self(). Initialized once, reused for the process lifetime.
var hostPort = C.mach_host_self()

// readTicks calls host_statistics(HOST_CPU_LOAD_INFO) to get cumulative
// CPU ticks — the same API Activity Monitor uses. ok is false when the mach
// call fails (e.g. transiently across sleep/wake); the caller must then hold
// its last reading rather than treat the zero-valued struct as real data.
func readTicks() (cpuTicks, bool) {
	var info C.host_cpu_load_info_data_t
	count := C.mach_msg_type_number_t(C.HOST_CPU_LOAD_INFO_COUNT)

	if C.host_statistics(
		hostPort,
		C.HOST_CPU_LOAD_INFO,
		C.host_info_t(unsafe.Pointer(&info)),
		&count,
	) != C.KERN_SUCCESS {
		return cpuTicks{}, false
	}

	return cpuTicks{
		User:   uint64(info.cpu_ticks[C.CPU_STATE_USER]),
		System: uint64(info.cpu_ticks[C.CPU_STATE_SYSTEM]),
		Idle:   uint64(info.cpu_ticks[C.CPU_STATE_IDLE]),
		Nice:   uint64(info.cpu_ticks[C.CPU_STATE_NICE]),
	}, true
}

// computePercent derives CPU utilization (0–100) from two cumulative tick
// snapshots and the last held value. ok=false means there's no usable delta and
// the caller should keep its previous reading. Deltas are computed in SIGNED
// space: a non-monotonic counter (sleep/wake, CPU offline/online) reads as "no
// usable delta" and holds, instead of wrapping uint64 subtraction into garbage.
func computePercent(prev, cur cpuTicks, lastPct float64) (float64, bool) {
	if prev.total() == 0 {
		return 0, true // first sample — no delta yet, 0 is the real result
	}
	totalDelta := int64(cur.total()) - int64(prev.total())
	busyDelta := int64(cur.busy()) - int64(prev.busy())
	if totalDelta <= 0 || busyDelta < 0 {
		// Ticks unchanged (common at 150ms) or counter went backwards — hold.
		return lastPct, false
	}
	pct := float64(busyDelta) / float64(totalDelta) * 100
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// sample folds one tick reading into the sampler state and returns CPU%. A
// failed read (ok=false) holds the last value WITHOUT storing the bad ticks as
// prev — otherwise the next read would diff against a zero snapshot and emit a
// false 0%. The cgo-free core, so it's testable (see cpu_darwin_test.go).
func (s *cpuSampler) sample(cur cpuTicks, ok bool) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok {
		return s.lastPct
	}
	prev := s.prev
	s.prev = cur
	pct, valid := computePercent(prev, cur, s.lastPct)
	if valid {
		s.lastPct = pct
	}
	return pct
}

// SampleCPUPercent returns CPU utilization since the last call, as 0-100.
// First call returns 0 (no previous sample to diff against). When mach ticks
// haven't updated between reads (common at 150ms sampling) or a read fails,
// holds the last computed value instead of returning a false 0%.
func SampleCPUPercent() float64 {
	cur, ok := readTicks()
	return defaultCPUSampler.sample(cur, ok)
}
