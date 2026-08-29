package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"testing/quick"
	"testing/slogtest"
)

func main() {
	err := quick.Check(func(x int) bool { return x-x == 0 })
	fmt.Println(err == nil)
	err = quick.Check(func(x int) bool { return x > 1000000 })
	fmt.Println(err != nil)
	err = quick.CheckString(func(s string) bool { return len(s) >= 0 })
	fmt.Println(err == nil)

	var buf bytes.Buffer
	l := slog.New(&buf)
	fmt.Println(slogtest.Exercise(l, &buf) == nil)
}
