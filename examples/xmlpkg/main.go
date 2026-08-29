package main

import (
	"encoding/xml"
	"fmt"
)

type Address struct {
	City string
	Zip  string
}

type Person struct {
	Name    string
	Age     int
	Married bool
	Home    Address
}

type Simple struct {
	X int
	Y int
}

func main() {
	p := Person{Name: "A & B <ok>", Age: 30, Married: true, Home: Address{City: "Springfield", Zip: "12345"}}
	out, err := xml.Marshal(p)
	fmt.Println(err == nil)
	expected := "<Person><Name>A &amp; B &lt;ok&gt;</Name><Age>30</Age><Married>true</Married><Home><City>Springfield</City><Zip>12345</Zip></Home></Person>"
	fmt.Println(string(out) == expected)

	s := Simple{X: 1, Y: 2}
	out2, _ := xml.Marshal(s)
	fmt.Println(string(out2) == "<Simple><X>1</X><Y>2</Y></Simple>")
}
