package main

import (
	"crypto/rand"
	"fmt"
)

func main() {
	b := make([]byte, 16)
	n, err := rand.Read(b)
	fmt.Println(err == nil)
	fmt.Println(n == 16)
	b2 := make([]byte, 8)
	n2, err2 := rand.Read(b2)
	fmt.Println(err2 == nil)
	fmt.Println(n2 == 8)
}
