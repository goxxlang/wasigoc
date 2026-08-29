// Same bitwise-trick algorithms real Go's crypto/subtle uses (no
// branches on secret data), but with an honest caveat real Go doesn't
// have to make: this project compiles through an optimizing C++
// compiler (-O2 on the host, wasi-sdk clang++ for wasm), and there's no
// way to verify from here that a branch never gets reintroduced by
// instruction selection or vectorization the way Go's own compiler
// team specifically tunes against. Use for real security-sensitive
// comparisons with that caveat in mind, not as an unconditional
// guarantee.
package subtle

func ConstantTimeCompare(x []byte, y []byte) int {
	if len(x) != len(y) {
		return 0
	}
	var v byte = 0
	for i := 0; i < len(x); i++ {
		v = v | (x[i] ^ y[i])
	}
	return ConstantTimeByteEq(v, 0)
}

func ConstantTimeByteEq(x byte, y byte) int {
	return int((uint32(x^y) - 1) >> 31)
}

func ConstantTimeEq(x int32, y int32) int {
	return int((uint64(uint32(x^y)) - 1) >> 63)
}

// ConstantTimeSelect returns x if v == 1, y if v == 0; undefined for
// any other v (matches real Go's own documented contract).
func ConstantTimeSelect(v int, x int, y int) int {
	return v*x + (1-v)*y
}

// ConstantTimeLessOrEq returns 1 if x <= y, 0 otherwise; undefined if x
// or y are negative or > 2^31 (matches real Go's own documented
// contract).
func ConstantTimeLessOrEq(x int, y int) int {
	x32 := int32(x)
	y32 := int32(y)
	return int(((x32 - y32 - 1) >> 31) & 1)
}

// ConstantTimeCopy copies y into x if v == 1; leaves x unchanged if
// v == 0. Panics if x and y have different lengths.
func ConstantTimeCopy(v int, x []byte, y []byte) {
	if len(x) != len(y) {
		panic("subtle: slices have different lengths")
	}
	xmask := byte(v * -1)
	for i := 0; i < len(x); i++ {
		x[i] = x[i]&^xmask | y[i]&xmask
	}
}
