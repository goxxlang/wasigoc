// Tiny subset of math/rand: a plain xorshift64* PRNG (deterministic given a
// seed -- not cryptographically secure, same caveat real math/rand always
// had). The package-level generator seeds itself from time.Now().UnixNano()
// at startup (no crypto/rand or WASI random_get wired up here yet -- see
// README's stdlib tracker) so repeated runs aren't always identical, but
// Seed(n) still makes it fully reproducible when that's wanted.
package rand

import "time"

type Source struct {
	state uint64
}

func NewSource(seed int64) *Source {
	s := &Source{state: uint64(seed)}
	if s.state == 0 {
		s.state = 1
	}
	return s
}

func (s *Source) Seed(seed int64) {
	s.state = uint64(seed)
	if s.state == 0 {
		s.state = 1
	}
}

func (s *Source) next() uint64 {
	x := s.state
	x = x ^ (x >> 12)
	x = x ^ (x << 25)
	x = x ^ (x >> 27)
	s.state = x
	return x * 2685821657736338717
}

func (s *Source) Int63() int64 {
	return int64(s.next() >> 1)
}

type Rand struct {
	src *Source
}

func New(src *Source) *Rand {
	return &Rand{src: src}
}

func (r *Rand) Seed(seed int64) {
	r.src.Seed(seed)
}

func (r *Rand) Int63() int64 {
	return r.src.Int63()
}

func (r *Rand) Int31() int32 {
	return int32(r.src.Int63() >> 32)
}

func (r *Rand) Int() int {
	return int(r.src.Int63())
}

func (r *Rand) Int63n(n int64) int64 {
	if n <= 0 {
		panic("invalid argument to Int63n")
	}
	return r.Int63() % n
}

func (r *Rand) Int31n(n int32) int32 {
	return int32(r.Int63n(int64(n)))
}

func (r *Rand) Intn(n int) int {
	if n <= 0 {
		panic("invalid argument to Intn")
	}
	return int(r.Int63n(int64(n)))
}

func (r *Rand) Float64() float64 {
	return float64(r.Int63()) / 9223372036854775808.0
}

func (r *Rand) Float32() float32 {
	return float32(r.Float64())
}

func (r *Rand) Shuffle(n int, swap func(i int, j int)) {
	for i := n - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		swap(i, j)
	}
}

func (r *Rand) Perm(n int) []int {
	m := make([]int, n)
	for i := 1; i < n; i++ {
		j := r.Intn(i + 1)
		m[i] = m[j]
		m[j] = i
	}
	return m
}

func seedFromClock() int64 {
	return time.Now().UnixNano()
}

var globalSrc = NewSource(seedFromClock())
var global = New(globalSrc)

func Seed(seed int64)              { global.Seed(seed) }
func Int63() int64                 { return global.Int63() }
func Int31() int32                 { return global.Int31() }
func Int() int                     { return global.Int() }
func Int63n(n int64) int64         { return global.Int63n(n) }
func Int31n(n int32) int32         { return global.Int31n(n) }
func Intn(n int) int               { return global.Intn(n) }
func Float64() float64             { return global.Float64() }
func Float32() float32             { return global.Float32() }
func Shuffle(n int, swap func(i int, j int)) { global.Shuffle(n, swap) }
func Perm(n int) []int             { return global.Perm(n) }
