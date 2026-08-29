package main

import (
	"fmt"
	"io/ioutil"
	"strings"
)

func main() {
	r := strings.NewReader("hello ioutil")
	data, err := ioutil.ReadAll(r)
	fmt.Println(err == nil)
	fmt.Println(string(data))

	werr := ioutil.WriteFile("ioutil_test.txt", []byte("round trip"), 420)
	fmt.Println(werr == nil)

	rdata, rerr := ioutil.ReadFile("ioutil_test.txt")
	fmt.Println(rerr == nil)
	fmt.Println(string(rdata))
}
