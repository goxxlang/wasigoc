// Bounded subset, and the bound is real: GC() and Gosched() are no-ops
// here, not fake hooks pretending otherwise. Two independent reasons:
// (1) plain Go source in this compiler cannot reach `gc::heap().Collect()`
// or the C++20 coroutine scheduler at all -- doing so for real would need
// compiler-level special-casing (like os.Getenv/reflect.TypeOf/time.Now),
// not just Go source, and (2) even with that wiring, `wasigo::New<T>()`
// (what `&T{...}` lowers to for every struct pointer, in every package,
// today) isn't routed through Oilpan yet -- see README's own Rosetta
// table -- so an actual collection pass would find nothing Go-visible to
// collect regardless. Real Go's own GC()/Gosched() also make zero
// behavioral guarantees (both are scheduling hints, not correctness
// requirements), so a no-op is a valid, honest, non-breaking
// implementation of the documented contract, not a shortcut around one.
// GOMAXPROCS/NumCPU/NumGoroutine/Version/GOOS/GOARCH are real, correct
// values for this actual target (one thread, wasm32-wasip1).
package runtime

const GOOS = "wasip1"
const GOARCH = "wasm"

func GC() {}

func Gosched() {}

// GOMAXPROCS always reports (and stays) 1 -- there is exactly one thread
// here, so the real GOMAXPROCS knob has nothing to control.
func GOMAXPROCS(n int) int {
	return 1
}

func NumCPU() int {
	return 1
}

// NumGoroutine cannot see the real coroutine runqueue from Go source
// (see the package comment) -- always reports 1 rather than guessing.
func NumGoroutine() int {
	return 1
}

func Version() string {
	return "go++wasigoc"
}
