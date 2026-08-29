package main

import (
	"context"
	"fmt"
)

func worker(ctx *context.Context, done chan bool) {
	select {
	case <-ctx.Done():
		fmt.Println("cancelled:", ctx.Err())
	}
	done <- true
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan bool)
	go worker(ctx, finished)
	cancel()
	<-finished

	ctx2 := context.WithValue(context.Background(), "user", "alice")
	v := ctx2.Value("user")
	fmt.Println(v)
	fmt.Println(ctx2.Value("missing") == nil)

	bg := context.Background()
	fmt.Println(bg.Err() == nil)
}
