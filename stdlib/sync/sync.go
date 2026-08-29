// Tiny subset of sync. wasm32-wasip1 is one thread with cooperative
// goroutines (see README's Rosetta table), so Mutex/RWMutex are no-ops --
// there is no preemption inside a task, only at explicit await points.
// WaitGroup/Once still track real state since callers depend on their
// counts, not on blocking a second OS thread.
package sync

type Mutex struct {
	locked bool
}

func (m *Mutex) Lock() {
	m.locked = true
}

func (m *Mutex) Unlock() {
	m.locked = false
}

func (m *Mutex) TryLock() bool {
	if m.locked {
		return false
	}
	m.locked = true
	return true
}

type RWMutex struct {
	locked bool
}

func (m *RWMutex) Lock() {
	m.locked = true
}

func (m *RWMutex) Unlock() {
	m.locked = false
}

func (m *RWMutex) RLock() {}

func (m *RWMutex) RUnlock() {}

func (m *RWMutex) TryLock() bool {
	if m.locked {
		return false
	}
	m.locked = true
	return true
}

type Once struct {
	done bool
}

func (o *Once) Do(f func()) {
	if o.done {
		return
	}
	o.done = true
	f()
}

type WaitGroup struct {
	n int
}

func (w *WaitGroup) Add(delta int) {
	w.n = w.n + delta
}

func (w *WaitGroup) Done() {
	w.n--
}

// Wait does not block: there is no Go-source-level way to yield to the
// cooperative scheduler from outside a channel/select coroutine (see
// README's Rosetta table), so a WaitGroup only helps when every Done() is
// already known to have happened before Wait() is called.
func (w *WaitGroup) Wait() {}

// sync.Map is not implemented: it needs map[any]any, and wasigo::Any (this
// compiler's interface{} boxing) has no value equality/hash -- only
// identity would be available, which would silently break Load-after-Store
// with a fresh key value. Use a concretely-typed map instead.
