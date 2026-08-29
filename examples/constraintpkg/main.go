package main

import (
	"fmt"
	"go/build/constraint"
)

func tagIs(tags []string, tag string) bool {
	for i := 0; i < len(tags); i++ {
		if tags[i] == tag {
			return true
		}
	}
	return false
}

func main() {
	fmt.Println(constraint.IsGoBuild("//go:build linux && amd64"))
	fmt.Println(constraint.IsGoBuild("// +build linux"))
	fmt.Println(constraint.IsPlusBuild("// +build linux amd64"))

	e1, err1 := constraint.Parse("//go:build linux && amd64")
	fmt.Println(err1 == nil)
	fmt.Println(e1.String())

	linuxAmd64 := []string{"linux", "amd64"}
	darwinArm64 := []string{"darwin", "arm64"}
	fmt.Println(e1.Eval(func(tag string) bool { return tagIs(linuxAmd64, tag) }))
	fmt.Println(e1.Eval(func(tag string) bool { return tagIs(darwinArm64, tag) }))

	e2, err2 := constraint.Parse("//go:build (linux || darwin) && !cgo")
	fmt.Println(err2 == nil)
	fmt.Println(e2.String())
	fmt.Println(e2.Eval(func(tag string) bool { return tagIs([]string{"linux"}, tag) }))
	fmt.Println(e2.Eval(func(tag string) bool { return tagIs([]string{"linux", "cgo"}, tag) }))
	fmt.Println(e2.Eval(func(tag string) bool { return tagIs([]string{"windows"}, tag) }))

	// Old-style "+build": space-separated OR, comma-separated AND, "!" negates.
	e3, err3 := constraint.Parse("// +build linux,amd64 darwin,arm64")
	fmt.Println(err3 == nil)
	fmt.Println(e3.Eval(func(tag string) bool { return tagIs(linuxAmd64, tag) }))
	fmt.Println(e3.Eval(func(tag string) bool { return tagIs(darwinArm64, tag) }))
	fmt.Println(e3.Eval(func(tag string) bool { return tagIs([]string{"linux", "arm64"}, tag) }))

	e4, err4 := constraint.Parse("// +build !windows")
	fmt.Println(err4 == nil)
	fmt.Println(e4.Eval(func(tag string) bool { return tagIs([]string{"linux"}, tag) }))
	fmt.Println(e4.Eval(func(tag string) bool { return tagIs([]string{"windows"}, tag) }))

	// Not a constraint line at all.
	_, err5 := constraint.Parse("// just a comment")
	fmt.Println(err5 != nil)

	// Malformed //go:build expression.
	_, err6 := constraint.Parse("//go:build linux &&")
	fmt.Println(err6 != nil)

	_, err7 := constraint.Parse("//go:build (linux")
	fmt.Println(err7 != nil)
}
