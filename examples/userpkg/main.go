package main

import (
	"fmt"
	"os/user"
)

func main() {
	_, err1 := user.Current()
	fmt.Println(err1 != nil)

	_, err2 := user.Lookup("root")
	fmt.Println(err2 != nil)

	_, err3 := user.LookupId("0")
	fmt.Println(err3 != nil)
}
