package main

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

func lookup(k string) error {
	if k == "" {
		return fmt.Errorf("lookup %s: %w", k, ErrNotFound)
	}
	return nil
}

func main() {
	err := lookup("")
	fmt.Println(err)
	fmt.Println(errors.Is(err, ErrNotFound))
	fmt.Println(errors.Unwrap(err) == ErrNotFound)

	e1 := errors.New("e1")
	e2 := errors.New("e2")
	joined := errors.Join(e1, e2)
	fmt.Println(joined)

	var nilErr error
	fmt.Println(errors.Unwrap(nilErr) == nil)

	wrapped2 := fmt.Errorf("outer: %w", err)
	fmt.Println(errors.Is(wrapped2, ErrNotFound))
}
