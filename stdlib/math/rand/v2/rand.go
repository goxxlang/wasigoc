// Bounded subset of math/rand/v2. Real Go 1.22 math/rand/v2 changed the
// Source interface to Uint64() (not v1's Int63()) and, deliberately for
// security reasons, dropped the package-level Seed function entirely --
// top-level functions are auto-seeded from OS entropy and cannot be reset.
// That shape is kept here: NewPCG builds a Source, New wraps a Source in a
// Rand, and package-level functions use an unexported self-seeded default
// Rand with no way to reseed it.
//
// PCG here is NOT the real PCG algorithm (same "Rosetta not parity"
// precedent as v1's Source and hash/maphash: real Go's own contract for
// this package doesn't promise bit-for-bit reproducibility across
// implementations either) -- it's a seeded xorshift64* mixed with a second
// seed word, real math/rand/v2's actual PCG is a different construction.
// No N[T] generic entry point (real v2's is a constrained union of every
// Go integer type; this compiler's generics haven't been exercised against
// an unexported multi-type union constraint) -- concrete IntN/Int32N/
// Int64N/Uint64N cover the same ground.
package rand

import "time"

type Source struct {
	state uint64
	inc   uint64
}

func NewPCG(seed1 uint64, seed2 uint64) *Source {
	s := &Source{state: seed1, inc: seed2 | 1}
	return s
}

func (s *Source) Uint64() uint64 {
	s.state = s.state + s.inc
	x := s.state
	x = x ^ (x >> 12)
	x = x ^ (x << 25)
	x = x ^ (x >> 27)
	return x * 2685821657736338717
}

type Rand struct {
	src *Source
}

func New(src *Source) *Rand {
	return &Rand{src: src}
}

func (r *Rand) Uint64() uint64 {
	return r.src.Uint64()
}

func (r *Rand) Int64() int64 {
	return int64(r.src.Uint64() >> 1)
}

func (r *Rand) Int32() int32 {
	return int32(r.src.Uint64() >> 33)
}

func (r *Rand) Int() int {
	return int(r.Int64())
}

func (r *Rand) Uint64N(n uint64) uint64 {
	if n == 0 {
		panic("invalid argument to Uint64N")
	}
	return r.src.Uint64() % n
}

func (r *Rand) Int64N(n int64) int64 {
	if n <= 0 {
		panic("invalid argument to Int64N")
	}
	return int64(r.Uint64N(uint64(n)))
}

func (r *Rand) Int32N(n int32) int32 {
	return int32(r.Int64N(int64(n)))
}

func (r *Rand) IntN(n int) int {
	if n <= 0 {
		panic("invalid argument to IntN")
	}
	return int(r.Int64N(int64(n)))
}

func (r *Rand) Float64() float64 {
	return float64(r.src.Uint64()>>11) / 9007199254740992.0
}

func (r *Rand) Float32() float32 {
	return float32(r.Float64())
}

func (r *Rand) Shuffle(n int, swap func(i int, j int)) {
	for i := n - 1; i > 0; i-- {
		j := r.IntN(i + 1)
		swap(i, j)
	}
}

func (r *Rand) Perm(n int) []int {
	m := make([]int, n)
	for i := 1; i < n; i++ {
		j := r.IntN(i + 1)
		m[i] = m[j]
		m[j] = i
	}
	return m
}

func seedFromClock() *Source {
	now := uint64(time.Now().UnixNano())
	return NewPCG(now, now^0x9E3779B97F4A7C15)
}

var global = New(seedFromClock())

func Uint64() uint64                         { return global.Uint64() }
func Int64() int64                           { return global.Int64() }
func Int32() int32                           { return global.Int32() }
func Int() int                               { return global.Int() }
func Uint64N(n uint64) uint64                { return global.Uint64N(n) }
func Int64N(n int64) int64                   { return global.Int64N(n) }
func Int32N(n int32) int32                   { return global.Int32N(n) }
func IntN(n int) int                         { return global.IntN(n) }
func Float64() float64                       { return global.Float64() }
func Float32() float32                       { return global.Float32() }
func Shuffle(n int, swap func(i int, j int)) { global.Shuffle(n, swap) }
func Perm(n int) []int                       { return global.Perm(n) }
