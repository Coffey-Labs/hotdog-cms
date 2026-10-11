//go:build !linux

package isolate

// limitMemory relies on the Go runtime's soft limit elsewhere; the deadline
// still applies.
func limitMemory(uint64) {}
