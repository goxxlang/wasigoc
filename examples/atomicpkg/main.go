package main

import (
	"fmt"
	"sync/atomic"
)

func main() {
	var n int64
	atomic.AddInt64(&n, 5)
	atomic.AddInt64(&n, 3)
	fmt.Println(atomic.LoadInt64(&n))
	ok := atomic.CompareAndSwapInt64(&n, 8, 100)
	fmt.Println(ok, atomic.LoadInt64(&n))
	ok2 := atomic.CompareAndSwapInt64(&n, 8, 200)
	fmt.Println(ok2, atomic.LoadInt64(&n))

	var counter atomic.Int64
	counter.Add(10)
	counter.Add(5)
	fmt.Println(counter.Load())
	old := counter.Swap(0)
	fmt.Println(old, counter.Load())

	var b atomic.Bool
	b.Store(true)
	fmt.Println(b.Load())

	var v atomic.Value
	v.Store("hi")
	fmt.Println(v.Load())
}
