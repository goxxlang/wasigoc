package main

import (
	"fmt"
	"hash/maphash"
)

func main() {
	fixedSeed := maphash.Seed{}

	// Same seed + same input -> same hash, every time.
	h1 := maphash.String(fixedSeed, "hello")
	h2 := maphash.String(fixedSeed, "hello")
	fmt.Println(h1 == h2)

	// Different input -> (virtually certainly) different hash.
	h3 := maphash.String(fixedSeed, "world")
	fmt.Println(h1 != h3)

	// Bytes and String give the same result for the same content.
	h4 := maphash.Bytes(fixedSeed, []byte("hello"))
	fmt.Println(h1 == h4)

	// A random seed differs from the fixed zero seed (astronomically
	// unlikely to collide).
	randSeed := maphash.MakeSeed()
	h5 := maphash.String(randSeed, "hello")
	fmt.Println(h5 != h1)

	// Streaming Hash matches the one-shot function for the same content,
	// split across multiple Write calls.
	var hh maphash.Hash
	hh.SetSeed(fixedSeed)
	hh.WriteString("hel")
	hh.WriteString("lo")
	fmt.Println(hh.Sum64() == h1)

	// Reset returns to the same starting state.
	hh.Reset()
	hh.WriteString("hello")
	fmt.Println(hh.Sum64() == h1)

	// Sum() byte encoding round-trips the same 8 bytes as Sum64().
	sum := hh.Sum(nil)
	fmt.Println(len(sum))

	// A zero-value Hash lazily self-seeds and still produces a stable
	// hash for repeated Sum64 calls without further writes.
	var hz maphash.Hash
	hz.WriteString("zero")
	v1 := hz.Sum64()
	v2 := hz.Sum64()
	fmt.Println(v1 == v2)

	// SetSeed changes what Seed() reports back.
	hh.SetSeed(randSeed)
	fmt.Println(hh.Seed() == randSeed)
}
