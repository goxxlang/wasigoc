package main

import (
	"container/heap"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"unicode/utf16"
)

type IntHeap struct {
	s []int
}

func (h IntHeap) Len() int           { return len(h.s) }
func (h IntHeap) Less(i, j int) bool { return h.s[i] < h.s[j] }
func (h IntHeap) Swap(i, j int)      { h.s[i], h.s[j] = h.s[j], h.s[i] }
func (h *IntHeap) Push(x any)        { h.s = append(h.s, x.(int)) }
func (h *IntHeap) Pop() any {
	old := h.s
	n := len(old)
	v := old[n-1]
	h.s = old[0 : n-1]
	return v
}

func main() {
	fmt.Println(hex.EncodeToString([]byte("hi")))
	b, err := hex.DecodeString("6869")
	fmt.Println(string(b), err)

	fmt.Println(base64.StdEncoding.EncodeToString([]byte("hi")))
	d, err2 := base64.StdEncoding.DecodeString("aGVsbG8=")
	fmt.Println(string(d), err2)

	r16 := utf16.Encode([]rune("A"))
	fmt.Println(len(r16), r16[0])

	h := &IntHeap{s: []int{5, 2, 8, 1}}
	heap.Init(h)
	heap.Push(h, 0)
	fmt.Println(heap.Pop(h))
	fmt.Println(heap.Pop(h))

	log.Println("done", 1, true)
}
