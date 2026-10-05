//go:build !darwin && !linux

package diagnostics

// Runtime memory, GC, and goroutine metrics remain available on other systems.
// Missing OS counters are explicitly unavailable rather than measured zeros.
func processUsage() (float64, uint64, bool, bool) { return 0, 0, false, false }
