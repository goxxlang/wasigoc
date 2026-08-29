package main

import (
	"errors"
	"fmt"
)

func Fib(n int) int {
	a := 0
	b := 1
	for i := 0; i < n; i++ {
		a, b = b, a+b
	}
	return a
}

func Divide(a int, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

func main() {
	for i := 0; i < 10; i++ {
		fmt.Printf("fib(%d) = %d\n", i, Fib(i))
	}

	q, err := Divide(10, 2)
	if err != nil {
		fmt.Println("error:", err)
	} else {
		fmt.Println("10 / 2 =", q)
	}

	_, err = Divide(1, 0)
	if err != nil {
		fmt.Println("error:", err)
	}
}
