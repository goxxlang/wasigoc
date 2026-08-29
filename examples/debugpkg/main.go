package main

import (
	"fmt"
	"runtime/debug"
)

func main() {
	fmt.Println(debug.SetGCPercent(50) == 100)
	debug.FreeOSMemory()
	fmt.Println(len(debug.Stack()) == 0)
	fmt.Println(debug.SetMaxStack(1000000) == 0)
	fmt.Println(debug.SetMaxThreads(4) == 1)
	fmt.Println(debug.SetPanicOnFault(true) == false)
	fmt.Println("survived")
}
