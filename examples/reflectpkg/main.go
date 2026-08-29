package main

import (
	"fmt"
	"reflect"
)

type Point struct {
	X    int
	Y    int
	Name string
}

func main() {
	p := Point{X: 3, Y: 4, Name: "origin"}

	v := reflect.ValueOf(p)
	fmt.Println(v.Kind() == reflect.Struct)
	fmt.Println(v.NumField())
	fmt.Println(v.FieldName(0))
	fmt.Println(v.FieldName(1))
	fmt.Println(v.FieldName(2))

	f0 := v.Field(0)
	fmt.Println(f0.Kind() == reflect.Int64)
	fmt.Println(f0.Int())

	f2 := v.Field(2)
	fmt.Println(f2.Kind() == reflect.String)
	fmt.Println(f2.String())

	t := reflect.TypeOf(p)
	fmt.Println(t.Name())
	fmt.Println(t.Kind() == reflect.Struct)

	n := 42
	nv := reflect.ValueOf(n)
	fmt.Println(nv.Kind() == reflect.Int64)
	fmt.Println(nv.Int())

	s := "hello"
	sv := reflect.ValueOf(s)
	fmt.Println(sv.Kind() == reflect.String)
	fmt.Println(sv.String())

	b := true
	bv := reflect.ValueOf(b)
	fmt.Println(bv.Kind() == reflect.Bool)
	fmt.Println(bv.Bool())
}
