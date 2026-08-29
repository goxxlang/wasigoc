// A deliberate no-op, same honest-boundary shape as os/exec: WASI
// preview1 delivers no signals to a wasm guest at all, so Notify never
// actually sends anything on its channel -- real, not a lie dressed up
// as a stub, since real Go's own contract for these functions never
// GUARANTEES a signal arrives either. Signals are a plain `int`
// (POSIX-numbered), not real Go's os.Signal interface -- os is a
// compiler builtin here (no loaded stdlib/os/*.go source), and adding
// a new exported type there is out of scope for what's fundamentally a
// no-op package on this target.
package signal

func Notify(c chan int, sig ...int) {}

func Stop(c chan int) {}

func Ignore(sig ...int) {}

func Reset(sig ...int) {}
