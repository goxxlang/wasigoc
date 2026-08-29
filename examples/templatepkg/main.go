package main

import (
	"bytes"
	"fmt"
	"text/template"
)

type Address struct {
	City string
}

type Person struct {
	Name    string
	Age     int
	Married bool
	Home    Address
}

func main() {
	t, err := template.New("p").Parse("Hello {{.Name}}, age {{.Age}}, city {{.Home.City}}.")
	fmt.Println(err == nil)
	var buf bytes.Buffer
	t.Execute(&buf, Person{Name: "Alice", Age: 30, Home: Address{City: "Springfield"}})
	fmt.Println(buf.String() == "Hello Alice, age 30, city Springfield.")

	t2raw, t2err := template.New("q").Parse("{{if .Married}}married{{else}}single{{end}}")
	t2 := template.Must(t2raw, t2err)
	var buf2 bytes.Buffer
	t2.Execute(&buf2, Person{Married: true})
	fmt.Println(buf2.String() == "married")

	var buf3 bytes.Buffer
	t2.Execute(&buf3, Person{Married: false})
	fmt.Println(buf3.String() == "single")

	t3, _ := template.New("r").Parse("{{if .Age}}has age {{.Age}}{{end}}, done")
	var buf4 bytes.Buffer
	t3.Execute(&buf4, Person{Age: 0})
	fmt.Println(buf4.String() == ", done")

	_, badErr := template.New("bad").Parse("{{unterminated")
	fmt.Println(badErr != nil)

	_, badErr2 := template.New("bad2").Parse("{{if .X}}no end")
	fmt.Println(badErr2 != nil)
}
