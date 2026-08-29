package main

import (
	"embed"
	"fmt"
)

func main() {
	var f embed.FS
	_, err := f.ReadFile("x.txt")
	fmt.Println(err != nil)
	_, err2 := f.Open(".")
	fmt.Println(err2 != nil)
}
