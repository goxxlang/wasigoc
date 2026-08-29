package main

import (
	"fmt"
	"net"
)

func pipeWriter(c *net.Conn, done chan bool) {
	c.Write([]byte("hello"))
	c.Write([]byte(" world"))
	c.Close()
	done <- true
}

func main() {
	conn, err := net.Dial("tcp", "example.com:80")
	fmt.Println(conn == nil)
	fmt.Println(err != nil)

	ln, err2 := net.Listen("tcp", ":8080")
	fmt.Println(ln == nil)
	fmt.Println(err2 != nil)

	c := &net.Conn{}
	_, err3 := c.Read(nil)
	fmt.Println(err3 != nil)
	_, err4 := c.Write(nil)
	fmt.Println(err4 != nil)
	err5 := c.Close()
	fmt.Println(err5 != nil)

	a, b := net.Pipe()
	done := make(chan bool)
	go pipeWriter(a, done)

	buf := make([]byte, 5)
	n, rerr := b.Read(buf)
	fmt.Println(n)
	fmt.Println(rerr == nil)
	fmt.Println(string(buf[0:n]))

	buf2 := make([]byte, 6)
	n2, rerr2 := b.Read(buf2)
	fmt.Println(n2)
	fmt.Println(rerr2 == nil)
	fmt.Println(string(buf2[0:n2]))

	<-done

	_, eofErr := b.Read(buf)
	fmt.Println(eofErr != nil)

	b.Close()
	_, writeErr := b.Write([]byte("x"))
	fmt.Println(writeErr != nil)
	_, readErr := b.Read(buf)
	fmt.Println(readErr != nil)
}
