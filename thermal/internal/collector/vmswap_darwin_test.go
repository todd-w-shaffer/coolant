package collector

import (
	"context"
	"testing"
)

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

// closeEnough reports whether got is within max(absTol, relTol*want) of want.
// The two readings are taken microseconds apart against live kernel state, so
// page counts and swap-used shift between them; this is a "same field, same
// units, same ballpark" check, not an exactness assertion.
func closeEnough(got, want, absTol int64, relTol float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	limit := absTol
	if rel := int64(relTol * float64(want)); rel > limit {
		limit = rel
	}
	return d <= limit
}

// TestVMSwapCgoMatchesSubprocess is the Phase 7 smoke-test gate: the in-process
// host_statistics64 / sysctlbyname readings must agree with the vm_stat / sysctl
// subprocess output they replace. A wrong struct field or unit would show up as
// a wildly different number here. Tolerances are generous because the readings
// are not simultaneous.
func TestVMSwapCgoMatchesSubprocess(t *testing.T) {
	ctx := context.Background()

	vc, ok := readVMStats64()
	if !ok {
		t.Fatal("readVMStats64 returned ok=false")
	}
	out, err := execCmd(ctx, "vm_stat")
	if err != nil || out == "" {
		t.Skipf("vm_stat subprocess unavailable: %v", err)
	}
	subActive := parseVMStatField(out, "Pages active")
	subInactive := parseVMStatField(out, "Pages inactive")
	subWired := parseVMStatField(out, "Pages wired down")
	subCompressed := parseVMStatField(out, "Pages occupied by compressor")
	subDecomp := parseVMStatField(out, "Decompressions")

	// Page counts drift as the system runs; allow 25% or 50k pages.
	if !closeEnough(int64(vc.Active), subActive, 50_000, 0.25) {
		t.Errorf("active pages: cgo=%d subprocess=%d (out of tolerance)", vc.Active, subActive)
	}
	if !closeEnough(int64(vc.Wired), subWired, 50_000, 0.25) {
		t.Errorf("wired pages: cgo=%d subprocess=%d (out of tolerance)", vc.Wired, subWired)
	}
	if !closeEnough(int64(vc.Compressed), subCompressed, 50_000, 0.25) {
		t.Errorf("compressed pages: cgo=%d subprocess=%d (out of tolerance)", vc.Compressed, subCompressed)
	}

	// Distinctness: a struct-field transcription slip (e.g. reading
	// inactive_count into Active, or compressor into Wired) produces a value in
	// the same ballpark as the tolerance checks accept, so also assert each cgo
	// field is nearer its intended subprocess field than its likeliest
	// confusable. Back-to-back reads drift <1%, far less than the inter-field
	// gaps, so this is robust without flaking.
	nearer := func(name string, got, want, confusable int64, confName string) {
		t.Helper()
		if absDiff(got, want) > absDiff(got, confusable) {
			t.Errorf("%s: cgo=%d is nearer %s=%d than its intended %s=%d — wrong struct field?",
				name, got, confName, confusable, name, want)
		}
	}
	nearer("active", int64(vc.Active), subActive, subInactive, "inactive")
	nearer("wired", int64(vc.Wired), subWired, subCompressed, "compressed")
	nearer("compressed", int64(vc.Compressed), subCompressed, subWired, "wired")

	// Decompressions is a lifetime monotonic counter read just before the
	// vm_stat fork, so subDecomp >= cgo within the few-ms fork window. A wrong
	// field here would read a page count (~1M) where millions are expected, so
	// assert the gap is small relative to the value AND non-negative-ish.
	if d := subDecomp - int64(vc.Decompressions); d < -1000 || !closeEnough(int64(vc.Decompressions), subDecomp, 1_000_000, 0.30) {
		t.Errorf("decompressions: cgo=%d subprocess=%d (gap %d out of expected forward window)", vc.Decompressions, subDecomp, d)
	}

	total, used, ok := readSwapUsage()
	if !ok {
		t.Fatal("readSwapUsage returned ok=false")
	}
	sout, err := execCmd(ctx, "sysctl", "-n", "vm.swapusage")
	if err != nil || sout == "" {
		t.Skipf("sysctl vm.swapusage subprocess unavailable: %v", err)
	}
	var ss SystemStats
	parseSwap(sout, &ss)
	// Swap total is stable; the subprocess rounds to 0.1 MB, so allow ~2MB.
	if !closeEnough(int64(total), ss.SwapTotalBytes, 2<<20, 0.02) {
		t.Errorf("swap total: cgo=%d subprocess=%d (out of tolerance)", total, ss.SwapTotalBytes)
	}
	// Swap used can move between reads; allow 16MB or 10%.
	if !closeEnough(int64(used), ss.SwapUsedBytes, 16<<20, 0.10) {
		t.Errorf("swap used: cgo=%d subprocess=%d (out of tolerance)", used, ss.SwapUsedBytes)
	}
}
