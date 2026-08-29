// Tiny subset of sync/atomic. wasm32-wasip1 is one thread with cooperative
// goroutines (no preemption inside a task -- see README's Rosetta table),
// so every operation here is a plain load/store/compare, not a real CPU
// atomic; the API shape (and therefore the "no data races by construction"
// property real Go gets from actual atomics) is what's preserved.
package atomic

func AddInt32(addr *int32, delta int32) int32 {
	*addr = *addr + delta
	return *addr
}

func AddInt64(addr *int64, delta int64) int64 {
	*addr = *addr + delta
	return *addr
}

func AddUint32(addr *uint32, delta uint32) uint32 {
	*addr = *addr + delta
	return *addr
}

func AddUint64(addr *uint64, delta uint64) uint64 {
	*addr = *addr + delta
	return *addr
}

func LoadInt32(addr *int32) int32   { return *addr }
func LoadInt64(addr *int64) int64   { return *addr }
func LoadUint32(addr *uint32) uint32 { return *addr }
func LoadUint64(addr *uint64) uint64 { return *addr }

func StoreInt32(addr *int32, val int32)   { *addr = val }
func StoreInt64(addr *int64, val int64)   { *addr = val }
func StoreUint32(addr *uint32, val uint32) { *addr = val }
func StoreUint64(addr *uint64, val uint64) { *addr = val }

func SwapInt32(addr *int32, new int32) int32 {
	old := *addr
	*addr = new
	return old
}

func SwapInt64(addr *int64, new int64) int64 {
	old := *addr
	*addr = new
	return old
}

func CompareAndSwapInt32(addr *int32, old int32, new int32) bool {
	if *addr == old {
		*addr = new
		return true
	}
	return false
}

func CompareAndSwapInt64(addr *int64, old int64, new int64) bool {
	if *addr == old {
		*addr = new
		return true
	}
	return false
}

// Go 1.19+ typed API: a struct receiver, so (unlike time.Duration -- see
// README's stdlib tracker) these can be real methods.

type Int32 struct {
	v int32
}

func (x *Int32) Load() int32     { return x.v }
func (x *Int32) Store(val int32) { x.v = val }
func (x *Int32) Add(delta int32) int32 {
	x.v = x.v + delta
	return x.v
}
func (x *Int32) Swap(new int32) int32 {
	old := x.v
	x.v = new
	return old
}
func (x *Int32) CompareAndSwap(old int32, new int32) bool {
	if x.v == old {
		x.v = new
		return true
	}
	return false
}

type Int64 struct {
	v int64
}

func (x *Int64) Load() int64     { return x.v }
func (x *Int64) Store(val int64) { x.v = val }
func (x *Int64) Add(delta int64) int64 {
	x.v = x.v + delta
	return x.v
}
func (x *Int64) Swap(new int64) int64 {
	old := x.v
	x.v = new
	return old
}
func (x *Int64) CompareAndSwap(old int64, new int64) bool {
	if x.v == old {
		x.v = new
		return true
	}
	return false
}

type Uint32 struct {
	v uint32
}

func (x *Uint32) Load() uint32     { return x.v }
func (x *Uint32) Store(val uint32) { x.v = val }
func (x *Uint32) Add(delta uint32) uint32 {
	x.v = x.v + delta
	return x.v
}
func (x *Uint32) Swap(new uint32) uint32 {
	old := x.v
	x.v = new
	return old
}
func (x *Uint32) CompareAndSwap(old uint32, new uint32) bool {
	if x.v == old {
		x.v = new
		return true
	}
	return false
}

type Uint64 struct {
	v uint64
}

func (x *Uint64) Load() uint64     { return x.v }
func (x *Uint64) Store(val uint64) { x.v = val }
func (x *Uint64) Add(delta uint64) uint64 {
	x.v = x.v + delta
	return x.v
}
func (x *Uint64) Swap(new uint64) uint64 {
	old := x.v
	x.v = new
	return old
}
func (x *Uint64) CompareAndSwap(old uint64, new uint64) bool {
	if x.v == old {
		x.v = new
		return true
	}
	return false
}

type Bool struct {
	v bool
}

func (x *Bool) Load() bool     { return x.v }
func (x *Bool) Store(val bool) { x.v = val }
func (x *Bool) Swap(new bool) bool {
	old := x.v
	x.v = new
	return old
}
func (x *Bool) CompareAndSwap(old bool, new bool) bool {
	if x.v == old {
		x.v = new
		return true
	}
	return false
}

type Value struct {
	v any
}

func (x *Value) Load() any     { return x.v }
func (x *Value) Store(val any) { x.v = val }
func (x *Value) Swap(new any) any {
	old := x.v
	x.v = new
	return old
}
