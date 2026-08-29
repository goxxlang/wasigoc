package main

import (
	"fmt"
	"net/mail"
)

func main() {
	a, err := mail.ParseAddress("Alice Smith <alice@example.com>")
	fmt.Println(err == nil)
	fmt.Println(a.Name)
	fmt.Println(a.Address)
	fmt.Println(a.String())

	b, err2 := mail.ParseAddress("bob@example.com")
	fmt.Println(err2 == nil)
	fmt.Println(b.Name == "")
	fmt.Println(b.Address)
	fmt.Println(b.String())

	_, err3 := mail.ParseAddress("not-an-address")
	fmt.Println(err3 != nil)

	_, err4 := mail.ParseAddress("Missing Angle <bob@example.com")
	fmt.Println(err4 != nil)

	list, err5 := mail.ParseAddressList("Alice <alice@example.com>, bob@example.com, \"Carol X\" <carol@example.com>")
	fmt.Println(err5 == nil)
	fmt.Println(len(list))
	fmt.Println(list[0].String())
	fmt.Println(list[1].String())
	fmt.Println(list[2].Name)
	fmt.Println(list[2].Address)
}
