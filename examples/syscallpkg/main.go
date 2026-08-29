package main

import (
	"fmt"
	"syscall"
)

func main() {
	fmt.Println(syscall.Getpid() == 1)
	fmt.Println(syscall.Getppid() == 0)
	wd, err := syscall.Getwd()
	fmt.Println(err == nil)
	fmt.Println(wd == ".")
	fmt.Println(syscall.Chdir("/") != nil)
	fmt.Println(syscall.Kill(1, 9) != nil)
	fmt.Println(len(syscall.Environ()) == 0)
	_, ok := syscall.Getenv("PATH")
	fmt.Println(ok == false)
}
