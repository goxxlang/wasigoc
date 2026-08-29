// crypto/rand, bounded: Reader fills a buffer from a time-seeded
// xorshift (math/rand), NOT a cryptographic OS CSPRNG. WASI preview1
// does have random_get, but wiring that needs a compiler builtin the
// same way time.Now does, and this package is ordinary .go -- same
// honest "not cryptographic" caveat math/rand itself already carries.
// Use this for nonces in tests and for unblocking APIs that take an
// io.Reader; do not use it to generate keys.
package rand

import "time"

var state uint64

func next() uint64 {
	x := state
	if x == 0 {
		x = uint64(time.Now().UnixNano())
		if x == 0 {
			x = 1
		}
	}
	x = x ^ (x >> 12)
	x = x ^ (x << 25)
	x = x ^ (x >> 27)
	state = x
	return x * 2685821657736338717
}

func Read(b []byte) (int, error) {
	i := 0
	for i < len(b) {
		v := next()
		n := 0
		for n < 7 && i < len(b) {
			b[i] = byte(v)
			v = v >> 8
			i = i + 1
			n = n + 1
		}
	}
	return len(b), nil
}
