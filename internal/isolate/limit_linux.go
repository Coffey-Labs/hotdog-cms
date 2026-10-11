package isolate

import "syscall"

// limitMemory caps the child's data segment, which on Linux counts the
// memory the Go runtime maps for its heap: past it, allocation fails and the
// child dies, not the service that started it.
func limitMemory(n uint64) {
	_ = syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: n, Max: n})
}
