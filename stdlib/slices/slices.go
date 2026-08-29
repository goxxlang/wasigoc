// Tiny subset of slices: generic helpers over []T.
package slices

func Contains[T comparable](s []T, v T) bool {
	return Index(s, v) >= 0
}

func Index[T comparable](s []T, v T) int {
	for i := 0; i < len(s); i++ {
		if s[i] == v {
			return i
		}
	}
	return -1
}

func Equal[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Reverse[T any](s []T) {
	i := 0
	j := len(s) - 1
	for i < j {
		s[i], s[j] = s[j], s[i]
		i++
		j--
	}
}

func Sort[T Ordered](s []T) {
	n := len(s)
	for i := 1; i < n; i++ {
		v := s[i]
		j := i
		for j > 0 && s[j-1] > v {
			s[j] = s[j-1]
			j--
		}
		s[j] = v
	}
}

func IsSorted[T Ordered](s []T) bool {
	for i := 1; i < len(s); i++ {
		if s[i] < s[i-1] {
			return false
		}
	}
	return true
}

func Max[T Ordered](s []T) T {
	m := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] > m {
			m = s[i]
		}
	}
	return m
}

func Min[T Ordered](s []T) T {
	m := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] < m {
			m = s[i]
		}
	}
	return m
}

func Clone[T any](s []T) []T {
	out := make([]T, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = s[i]
	}
	return out
}

func Insert[T any](s []T, i int, v T) []T {
	out := make([]T, 0, len(s)+1)
	for k := 0; k < i; k++ {
		out = append(out, s[k])
	}
	out = append(out, v)
	for k := i; k < len(s); k++ {
		out = append(out, s[k])
	}
	return out
}

func Delete[T any](s []T, i int, j int) []T {
	out := make([]T, 0, len(s)-(j-i))
	for k := 0; k < i; k++ {
		out = append(out, s[k])
	}
	for k := j; k < len(s); k++ {
		out = append(out, s[k])
	}
	return out
}

func Concat[T any](slices ...[]T) []T {
	var out []T
	for i := 0; i < len(slices); i++ {
		s := slices[i]
		for j := 0; j < len(s); j++ {
			out = append(out, s[j])
		}
	}
	return out
}

type Ordered any
