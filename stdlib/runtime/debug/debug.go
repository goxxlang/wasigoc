// Same honest-no-op reasoning as the runtime package itself (see its
// own header comment): GC tuning knobs have nothing real to tune here
// (Oilpan isn't wired into allocation at all yet), and real Go's own
// contract for these functions doesn't guarantee any particular effect
// either. Stack() returns an empty slice -- a real, documented gap
// (no stack-walking support here), not a fabricated trace.
package debug

func SetGCPercent(percent int) int {
	return 100
}

func FreeOSMemory() {}

func Stack() []byte {
	return []byte{}
}

func SetMaxStack(bytes int) int {
	return 0
}

func SetMaxThreads(threads int) int {
	return 1
}

func SetPanicOnFault(enabled bool) bool {
	return false
}
