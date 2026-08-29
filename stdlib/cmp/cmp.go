// Tiny subset of cmp: ordering helpers over comparable/ordered types.
package cmp

type Ordered any

func Compare[T Ordered](a T, b T) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func Less[T Ordered](a T, b T) bool {
	return a < b
}

func Or[T comparable](vals ...T) T {
	var zero T
	for i := 0; i < len(vals); i++ {
		if vals[i] != zero {
			return vals[i]
		}
	}
	return zero
}
