package main

import (
	"fmt"
	"go/build"
	"go/importer"
)

func main() {
	ctx := build.Default()
	fmt.Println(ctx.GOOS == "wasip1")
	fmt.Println(ctx.GOARCH == "wasm")
	fmt.Println(build.IsLocalImport("./foo"))
	fmt.Println(build.IsLocalImport("fmt") == false)
	name, err := build.ParsePackageName("package hello\n")
	fmt.Println(err == nil)
	fmt.Println(name == "hello")
	_, err2 := build.Import("fmt", ".")
	fmt.Println(err2 != nil)
	_, err3 := importer.Default().Import("fmt")
	fmt.Println(err3 != nil)
}
