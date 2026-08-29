package main

import (
	"fmt"
	"os/exec"
)

func main() {
	cmd := exec.Command("ls", "-la", "/tmp")
	fmt.Println(len(cmd.Args))
	err := cmd.Run()
	fmt.Println(err != nil)
	fmt.Println(err)

	out, err2 := cmd.Output()
	fmt.Println(out == nil, err2 != nil)

	_, err3 := exec.LookPath("ls")
	fmt.Println(err3 != nil)
}
