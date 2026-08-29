package main

import (
	"fmt"
	"go/version"
)

func main() {
	fmt.Println(version.IsValid("go1.21.3"))
	fmt.Println(version.IsValid("go1.21"))
	fmt.Println(version.IsValid("1.21"))
	fmt.Println(version.IsValid("bogus"))

	fmt.Println(version.Lang("go1.21.3"))
	fmt.Println(version.Lang("go1.21"))

	fmt.Println(version.Compare("go1.9", "go1.10"))
	fmt.Println(version.Compare("go1.21.0", "go1.21.0"))
	fmt.Println(version.Compare("go1.22", "go1.21"))
	fmt.Println(version.Compare("go1.21.1", "go1.21.2"))
}
