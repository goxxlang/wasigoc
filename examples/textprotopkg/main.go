package main

import (
	"fmt"
	"net/textproto"
	"strings"
)

func main() {
	fmt.Println(textproto.CanonicalMIMEHeaderKey("content-type"))
	fmt.Println(textproto.CanonicalMIMEHeaderKey("X-CUSTOM-HEADER"))

	h := textproto.MIMEHeader{}
	textproto.HeaderSet(h, "content-type", "text/plain")
	textproto.HeaderAdd(h, "X-Custom", "one")
	textproto.HeaderAdd(h, "X-Custom", "two")
	fmt.Println(textproto.HeaderGet(h, "Content-Type"))
	fmt.Println(len(textproto.HeaderValues(h, "x-custom")))
	fmt.Println(textproto.HeaderValues(h, "x-custom")[0])
	fmt.Println(textproto.HeaderValues(h, "x-custom")[1])
	textproto.HeaderDel(h, "Content-Type")
	fmt.Println(textproto.HeaderGet(h, "content-type") == "")

	raw := "Host: example.com\r\nContent-Type: text/plain\r\nX-Long: hello\r\n  world\r\n\r\nbody follows"
	r := textproto.NewReader(strings.NewReader(raw))
	hdr, err := r.ReadMIMEHeader()
	fmt.Println(err == nil)
	fmt.Println(textproto.HeaderGet(hdr, "Host"))
	fmt.Println(textproto.HeaderGet(hdr, "Content-Type"))
	fmt.Println(textproto.HeaderGet(hdr, "X-Long"))

	rest, err2 := r.ReadLine()
	fmt.Println(rest)
	fmt.Println(err2 == nil)

	_, err3 := r.ReadLine()
	fmt.Println(err3 != nil)
}
