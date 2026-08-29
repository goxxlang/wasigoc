// Tiny subset of maps: generic helpers over map[K]V.
package maps

func Keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func Values[K comparable, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func Clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func Copy[K comparable, V any](dst map[K]V, src map[K]V) {
	for k, v := range src {
		dst[k] = v
	}
}

func Equal[K comparable, V comparable](m1 map[K]V, m2 map[K]V) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		v2, ok := m2[k]
		if !ok || v2 != v {
			return false
		}
	}
	return true
}

func DeleteFunc[K comparable, V any](m map[K]V, del func(K, V) bool) {
	var dead []K
	for k, v := range m {
		if del(k, v) {
			dead = append(dead, k)
		}
	}
	for i := 0; i < len(dead); i++ {
		delete(m, dead[i])
	}
}
