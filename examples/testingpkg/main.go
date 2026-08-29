package main

import (
	"fmt"
	"testing"
)

func testAdd(t *testing.T) {
	got := 2 + 2
	if got != 4 {
		t.Error("expected 4 got", got)
	}
}

func testFail(t *testing.T) {
	t.Error("intentional failure")
}

func main() {
	ok1 := testing.Run("Add", testAdd)
	fmt.Println(ok1)
	ok2 := testing.Run("Fail", testFail)
	fmt.Println(ok2)
}
