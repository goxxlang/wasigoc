package main

import (
	"fmt"
	"mime"
)

func main() {
	fmt.Println(mime.TypeByExtension(".html") == "text/html; charset=utf-8")
	fmt.Println(mime.TypeByExtension(".JSON") == "application/json")
	fmt.Println(mime.TypeByExtension(".wasm") == "application/wasm")
	fmt.Println(mime.TypeByExtension(".unknown-ext") == "")

	err := mime.AddExtensionType(".foo", "application/x-foo")
	fmt.Println(err == nil)
	fmt.Println(mime.TypeByExtension(".foo") == "application/x-foo")
	fmt.Println(mime.TypeByExtension(".FOO") == "application/x-foo")
}
