package main

import (
	"fmt"
	"syscall"
)

func main() {
	pid := syscall.Getpid()
	fmt.Println(pid)
}
