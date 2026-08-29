// Tiny subset of sort: in-place insertion sort of []int and []string.
package sort

func Ints(x []int) {
	n := len(x)
	for i := 1; i < n; i++ {
		v := x[i]
		j := i
		for j > 0 && x[j-1] > v {
			x[j] = x[j-1]
			j--
		}
		x[j] = v
	}
}

func Strings(x []string) {
	n := len(x)
	for i := 1; i < n; i++ {
		v := x[i]
		j := i
		for j > 0 && x[j-1] > v {
			x[j] = x[j-1]
			j--
		}
		x[j] = v
	}
}

func IntsAreSorted(x []int) bool {
	for i := 1; i < len(x); i++ {
		if x[i] < x[i-1] {
			return false
		}
	}
	return true
}

func StringsAreSorted(x []string) bool {
	for i := 1; i < len(x); i++ {
		if x[i] < x[i-1] {
			return false
		}
	}
	return true
}

func Float64s(x []float64) {
	n := len(x)
	for i := 1; i < n; i++ {
		v := x[i]
		j := i
		for j > 0 && x[j-1] > v {
			x[j] = x[j-1]
			j--
		}
		x[j] = v
	}
}

func Float64sAreSorted(x []float64) bool {
	for i := 1; i < len(x); i++ {
		if x[i] < x[i-1] {
			return false
		}
	}
	return true
}

type Interface interface {
	Len() int
	Less(i int, j int) bool
	Swap(i int, j int)
}

func Sort(data Interface) {
	n := data.Len()
	for i := 1; i < n; i++ {
		for j := i; j > 0 && data.Less(j, j-1); j-- {
			data.Swap(j, j-1)
		}
	}
}

func IsSorted(data Interface) bool {
	n := data.Len()
	for i := 1; i < n; i++ {
		if data.Less(i, i-1) {
			return false
		}
	}
	return true
}

// Slice/SliceStable are generic (`[]T`), not reflection-based like real
// Go's `sort.Slice(x any, ...)` -- this compiler has no reflection to swap
// through an `any`. The common `sort.Slice(xs, func(i, j int) bool {...})`
// idiom still works: `xs` and `x` share one backing array (see README's
// Rosetta table on wasigo::Slice), so sorting `x` in place reorders the
// closure's `xs` too.
func Slice[T any](x []T, less func(i int, j int) bool) {
	n := len(x)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			x[j], x[j-1] = x[j-1], x[j]
		}
	}
}

func SliceStable[T any](x []T, less func(i int, j int) bool) {
	Slice(x, less)
}

func SliceIsSorted[T any](x []T, less func(i int, j int) bool) bool {
	for i := 1; i < len(x); i++ {
		if less(i, i-1) {
			return false
		}
	}
	return true
}

func Search(n int, f func(int) bool) int {
	lo := 0
	hi := n
	for lo < hi {
		mid := (lo + hi) / 2
		if !f(mid) {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func SearchInts(x []int, target int) int {
	return Search(len(x), func(i int) bool { return x[i] >= target })
}

func SearchStrings(x []string, target string) int {
	return Search(len(x), func(i int) bool { return x[i] >= target })
}

func SearchFloat64s(x []float64, target float64) int {
	return Search(len(x), func(i int) bool { return x[i] >= target })
}
