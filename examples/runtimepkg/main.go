package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println(runtime.GOOS)
	fmt.Println(runtime.GOARCH)
	fmt.Println(runtime.NumCPU())
	fmt.Println(runtime.GOMAXPROCS(4))
	fmt.Println(runtime.GOMAXPROCS(0))
	fmt.Println(runtime.NumGoroutine())
	fmt.Println(runtime.Version())
	runtime.GC()
	runtime.Gosched()
	fmt.Println("survived")
}
