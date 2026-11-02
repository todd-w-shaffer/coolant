//go:build !darwin

package workq

// IsAncestor is unsupported off darwin: a nested run never passes
// through, it takes its own ticket (safe, merely slower).
func IsAncestor(pid int) bool { return false }

// TreeFootprint is unsupported off darwin.
func TreeFootprint(root int) uint64 { return 0 }

// LoadAverage is unsupported off darwin.
func LoadAverage() float64 { return -1 }
