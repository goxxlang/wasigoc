package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	err := os.WriteFile("wasigo_test_scratch.txt", []byte("hello file"), 0644)
	fmt.Println(err)

	data, err2 := os.ReadFile("wasigo_test_scratch.txt")
	fmt.Println(string(data), err2)

	f, err3 := os.Open("wasigo_test_scratch.txt")
	fmt.Println(err3)
	out, err4 := io.ReadAll(f)
	fmt.Println(string(out), err4)
	closeErr := f.Close()
	fmt.Println(closeErr)

	cf, err5 := os.Create("wasigo_test_scratch2.txt")
	fmt.Println(err5)
	n, err6 := cf.Write([]byte("written"))
	fmt.Println(n, err6)
	cf.Close()

	data2, _ := os.ReadFile("wasigo_test_scratch2.txt")
	fmt.Println(string(data2))

	_, errOpen := os.Open("does_not_exist_wasigo.txt")
	fmt.Println(errOpen != nil)
}
