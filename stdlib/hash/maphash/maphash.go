// Real Go's hash/maphash is deliberately unspecified beyond "stable for a
// given process run, not across runs or implementations" -- its actual
// algorithm (runtime AES hashing) isn't part of its documented contract,
// so this doesn't need to match it bit-for-bit any more than this
// project's math/rand needs to match Go's PRNG bit-for-bit (same
// "Rosetta not parity" precedent, see that package's own header comment).
// Implemented as a real seeded FNV-1a-64 instead: deterministic for a
// given (seed, input) pair, different seeds really do produce different
// hashes, which is the actual property callers depend on (hash tables,
// dedup) -- just not Go's internal algorithm.
package maphash

import "time"

const fnvOffset64 = 14695981039346656037
const fnvPrime64 = 1099511628211

type Seed struct {
	s uint64
}

// MakeSeed returns a new random seed, self-seeded from the wall clock --
// same idea math/rand's self-seeding already uses here.
func MakeSeed() Seed {
	return Seed{s: uint64(time.Now().UnixNano())}
}

// Hash is the zero-value-usable streaming hasher: like real maphash, an
// unseeded Hash lazily picks a random seed on first use.
type Hash struct {
	seed        Seed
	h           uint64
	initialized bool
}

func (h *Hash) ensureInit() {
	if !h.initialized {
		h.seed = MakeSeed()
		h.h = fnvOffset64 ^ h.seed.s
		h.initialized = true
	}
}

func (h *Hash) SetSeed(seed Seed) {
	h.seed = seed
	h.h = fnvOffset64 ^ seed.s
	h.initialized = true
}

func (h *Hash) Seed() Seed {
	h.ensureInit()
	return h.seed
}

func (h *Hash) Reset() {
	h.ensureInit()
	h.h = fnvOffset64 ^ h.seed.s
}

func (h *Hash) Write(b []byte) (int, error) {
	h.ensureInit()
	for i := 0; i < len(b); i++ {
		h.h = h.h ^ uint64(b[i])
		h.h = h.h * fnvPrime64
	}
	return len(b), nil
}

func (h *Hash) WriteString(s string) (int, error) {
	return h.Write([]byte(s))
}

func (h *Hash) WriteByte(b byte) error {
	h.Write([]byte{b})
	return nil
}

func (h *Hash) Sum64() uint64 {
	h.ensureInit()
	return h.h
}

func (h *Hash) Sum(b []byte) []byte {
	v := h.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func Bytes(seed Seed, b []byte) uint64 {
	h := uint64(fnvOffset64) ^ seed.s
	for i := 0; i < len(b); i++ {
		h = h ^ uint64(b[i])
		h = h * fnvPrime64
	}
	return h
}

func String(seed Seed, s string) uint64 {
	return Bytes(seed, []byte(s))
}
