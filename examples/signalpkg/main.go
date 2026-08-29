package main

import (
	"fmt"
	"os/signal"
)

const sigint = 2
const sigterm = 15

func main() {
	c := make(chan int, 1)
	signal.Notify(c, sigint, sigterm)

	// A real no-op: nothing was ever sent, so a non-blocking select
	// falls through to default immediately.
	select {
	case v := <-c:
		fmt.Println("unexpectedly received", v)
	default:
		fmt.Println("no signal received, as expected")
	}

	signal.Ignore(sigint)
	signal.Reset(sigterm)
	signal.Stop(c)
	fmt.Println("all signal functions callable without crashing")
}
