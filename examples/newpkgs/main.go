// Exercises the generics-based additions: cmp, slices, maps, unicode,
// math/bits, container/list, container/ring, sync.
package main

import (
	"cmp"
	"container/list"
	"container/ring"
	"fmt"
	"maps"
	"math/bits"
	"slices"
	"sync"
	"unicode"
)

func main() {
	fmt.Println(cmp.Compare(1, 2))
	fmt.Println(cmp.Compare(2, 2))
	fmt.Println(cmp.Compare(3, 2))

	xs := []int{3, 1, 2}
	slices.Sort(xs)
	fmt.Println(xs[0], xs[1], xs[2])
	fmt.Println(slices.Contains(xs, 2))
	fmt.Println(slices.Contains(xs, 9))
	fmt.Println(slices.Index(xs, 2))
	fmt.Println(slices.Max(xs))
	fmt.Println(slices.Min(xs))
	ys := slices.Clone(xs)
	slices.Reverse(ys)
	fmt.Println(ys[0], ys[1], ys[2])
	fmt.Println(slices.Equal(xs, xs))
	fmt.Println(slices.Equal(xs, ys))

	m := map[string]int{"a": 1}
	ks := maps.Keys(m)
	fmt.Println(len(ks))
	vs := maps.Values(m)
	fmt.Println(len(vs))
	m2 := maps.Clone(m)
	fmt.Println(maps.Equal(m, m2))

	fmt.Println(unicode.IsDigit('5'))
	fmt.Println(unicode.IsLetter('5'))
	fmt.Println(unicode.IsUpper('A'))
	fmt.Println(string(unicode.ToUpper('a')))

	fmt.Println(bits.OnesCount64(7))
	fmt.Println(bits.LeadingZeros64(1))
	fmt.Println(bits.Len64(255))
	fmt.Println(bits.RotateLeft64(1, 4))

	l := list.New()
	l.PushBack(1)
	l.PushBack(2)
	l.PushFront(0)
	fmt.Println(l.Len())
	fmt.Println(l.Front().Value)
	fmt.Println(l.Back().Value)

	r := ring.New(3)
	fmt.Println(r.Len())
	sum := 0
	r.Do(func(v any) {
		sum++
	})
	fmt.Println(sum)

	var mu sync.Mutex
	mu.Lock()
	mu.Unlock()
	var once sync.Once
	calls := 0
	once.Do(func() { calls++ })
	once.Do(func() { calls++ })
	fmt.Println(calls)
}
