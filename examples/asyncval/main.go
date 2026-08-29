// Channel-using function that returns a value: emitted as wasigo::TaskT<T>.
package main

import "fmt"

func take(ch chan int) int {
	v := <-ch
	return v * 2
}

func main() {
	ch := make(chan int, 1)
	ch <- 21
	fmt.Println(take(ch))
}
