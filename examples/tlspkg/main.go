package main

import (
	"crypto/tls"
	"fmt"
)

func main() {
	c, err := tls.Dial("tcp", "example.com:443", &tls.Config{ServerName: "example.com"})
	fmt.Println(c == nil)
	fmt.Println(err != nil)
	fmt.Println(tls.LoadX509KeyPair("a", "b") != nil)
	var conn tls.Conn
	fmt.Println(conn.Handshake() != nil)
	fmt.Println(conn.Close() != nil)
}
