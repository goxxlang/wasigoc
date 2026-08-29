// Tiny subset of container/heap. Interface lists Len/Less/Swap/Push/Pop
// flatly (not embedding sort.Interface) -- this compiler's interface
// parser doesn't accept a package-qualified embedded interface name.
//
// A Interface implementation must be a struct wrapping a slice (`type
// IntHeap struct { s []int }`), not a method on a bare defined-slice type
// (`type IntHeap []int`) the way the real container/heap godoc example
// does it -- wasigoc only attaches methods to a real `struct`; `type X []T`
// is a C++ `using` alias with nothing to hang a method on (see README's
// "type Name T" row), so such methods are silently never emitted.
package heap

type Interface interface {
	Len() int
	Less(i int, j int) bool
	Swap(i int, j int)
	Push(x any)
	Pop() any
}

func up(h Interface, j int) {
	for j > 0 {
		i := (j - 1) / 2
		if i == j || !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		j = i
	}
}

func down(h Interface, i0 int, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 {
			break
		}
		j := j1
		j2 := j1 + 1
		if j2 < n && h.Less(j2, j1) {
			j = j2
		}
		if !h.Less(j, i) {
			break
		}
		h.Swap(i, j)
		i = j
	}
	return i > i0
}

func Init(h Interface) {
	n := h.Len()
	for i := n/2 - 1; i >= 0; i-- {
		down(h, i, n)
	}
}

func Push(h Interface, x any) {
	h.Push(x)
	up(h, h.Len()-1)
}

func Pop(h Interface) any {
	n := h.Len() - 1
	h.Swap(0, n)
	down(h, 0, n)
	return h.Pop()
}

func Remove(h Interface, i int) any {
	n := h.Len() - 1
	if n != i {
		h.Swap(i, n)
		if !down(h, i, n) {
			up(h, i)
		}
	}
	return h.Pop()
}

func Fix(h Interface, i int) {
	if !down(h, i, h.Len()) {
		up(h, i)
	}
}
