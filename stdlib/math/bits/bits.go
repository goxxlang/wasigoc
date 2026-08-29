// Tiny subset of math/bits, implemented with shifts/loops (no hex literals
// in this compiler's lexer, so no magic-constant tricks).
package bits

const UintSize = 64

func OnesCount64(x uint64) int {
	n := 0
	for x != 0 {
		if x&1 == 1 {
			n++
		}
		x = x >> 1
	}
	return n
}

func OnesCount32(x uint32) int {
	return OnesCount64(uint64(x))
}

func OnesCount(x uint) int {
	return OnesCount64(uint64(x))
}

func LeadingZeros64(x uint64) int {
	if x == 0 {
		return 64
	}
	n := 0
	for i := 63; i >= 0; i-- {
		bit := (x >> uint(i)) & 1
		if bit == 1 {
			break
		}
		n++
	}
	return n
}

func LeadingZeros32(x uint32) int {
	if x == 0 {
		return 32
	}
	n := 0
	for i := 31; i >= 0; i-- {
		bit := (x >> uint(i)) & 1
		if bit == 1 {
			break
		}
		n++
	}
	return n
}

func TrailingZeros64(x uint64) int {
	if x == 0 {
		return 64
	}
	n := 0
	for (x>>uint(n))&1 == 0 {
		n++
	}
	return n
}

func TrailingZeros32(x uint32) int {
	if x == 0 {
		return 32
	}
	n := 0
	for (x>>uint(n))&1 == 0 {
		n++
	}
	return n
}

func Len64(x uint64) int {
	return 64 - LeadingZeros64(x)
}

func Len32(x uint32) int {
	return 32 - LeadingZeros32(x)
}

func Len(x uint) int {
	return Len64(uint64(x))
}

func Reverse64(x uint64) uint64 {
	var out uint64
	for i := 0; i < 64; i++ {
		out = out << 1
		out = out | (x & 1)
		x = x >> 1
	}
	return out
}

func Reverse32(x uint32) uint32 {
	var out uint32
	for i := 0; i < 32; i++ {
		out = out << 1
		out = out | (x & 1)
		x = x >> 1
	}
	return out
}

func RotateLeft64(x uint64, k int) uint64 {
	s := uint(((k % 64) + 64) % 64)
	if s == 0 {
		return x
	}
	return (x << s) | (x >> (64 - s))
}

func RotateLeft32(x uint32, k int) uint32 {
	s := uint(((k % 32) + 32) % 32)
	if s == 0 {
		return x
	}
	return (x << s) | (x >> (32 - s))
}
