// Bounded subset of index/suffixarray: in-memory New/Lookup only. Real Go
// builds the suffix array with the linear-time DC3/skew algorithm and can
// persist/reload one via Read/Write (gob-encoded); this builds it with a
// plain O(n log^2 n) comparison sort (sort.Slice over suffix start
// indices, each comparison a byte-by-byte suffix compare) since there's no
// gob here to persist through anyway. Same asymptotic *query* cost as real
// Go (binary search over the sorted suffix array), just a slower one-time
// build. No Read/Write/serialization.
package suffixarray

import "sort"

type Index struct {
	data []byte
	sa   []int
}

func suffixLess(data []byte, i int, j int) bool {
	a := data[i:]
	b := data[j:]
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for k := 0; k < n; k++ {
		if a[k] != b[k] {
			return a[k] < b[k]
		}
	}
	return len(a) < len(b)
}

func New(data []byte) *Index {
	n := len(data)
	sa := make([]int, n)
	for i := 0; i < n; i++ {
		sa[i] = i
	}
	sort.Slice(sa, func(i int, j int) bool {
		return suffixLess(data, sa[i], sa[j])
	})
	return &Index{data: data, sa: sa}
}

func (x *Index) Bytes() []byte {
	return x.data
}

func hasPrefixAt(data []byte, at int, s []byte) bool {
	if at+len(s) > len(data) {
		return false
	}
	for k := 0; k < len(s); k++ {
		if data[at+k] != s[k] {
			return false
		}
	}
	return true
}

func compareSuffixToKey(data []byte, at int, s []byte) int {
	suf := data[at:]
	n := len(suf)
	if len(s) < n {
		n = len(s)
	}
	for k := 0; k < n; k++ {
		if suf[k] != s[k] {
			if suf[k] < s[k] {
				return -1
			}
			return 1
		}
	}
	if len(suf) < len(s) {
		return -1
	}
	if len(suf) > len(s) {
		return 1
	}
	return 0
}

func (x *Index) lowerBound(s []byte) int {
	lo, hi := 0, len(x.sa)
	for lo < hi {
		mid := (lo + hi) / 2
		if compareSuffixToKey(x.data, x.sa[mid], s) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (x *Index) upperBound(s []byte) int {
	lo, hi := 0, len(x.sa)
	for lo < hi {
		mid := (lo + hi) / 2
		if hasPrefixAt(x.data, x.sa[mid], s) || compareSuffixToKey(x.data, x.sa[mid], s) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (x *Index) Lookup(s []byte, n int) []int {
	if len(s) == 0 || len(x.sa) == 0 {
		return []int{}
	}
	lo := x.lowerBound(s)
	hi := x.upperBound(s)
	count := hi - lo
	if n >= 0 && n < count {
		count = n
	}
	out := make([]int, count)
	for i := 0; i < count; i++ {
		out[i] = x.sa[lo+i]
	}
	return out
}
