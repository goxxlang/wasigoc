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
	// External name: no DNS in the guest by default (wasigo-p2).
	conn, err := net.Dial("tcp", "example.com:80")
	fmt.Println(conn == nil)
	fmt.Println(err != nil)

	// Loopback-only bind (web-native). Explicit 127.0.0.1 avoids
	// leftover host listeners on bare ":port" from looking like success.
	ln, err2 := net.Listen("tcp", "127.0.0.1:34567")
	fmt.Println(ln == nil)
	fmt.Println(err2 != nil)
	if ln != nil {
		ln.Close()
	}

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

	ln2, lerr := net.Listen("tcp", "127.0.0.1:34568")
	if lerr != nil {
		fmt.Println(false)
		fmt.Println("")
	} else {
		done2 := make(chan bool)
		go acceptOnce(ln2, done2)
		c2, derr := net.Dial("tcp", "127.0.0.1:34568")
		fmt.Println(derr == nil)
		if derr == nil {
			c2.Write([]byte("hi"))
			c2.Close()
		}
		<-done2
		ln2.Close()
	}

	ua, ub := net.PacketPipe()
	go udpSend(ua)
	ubuf := make([]byte, 8)
	un, _, uerr := ub.ReadFrom(ubuf)
	fmt.Println(uerr == nil)
	fmt.Println(string(ubuf[0:un]))
	ua.Close()
	ub.Close()

	h, port, _ := net.SplitHostPort("127.0.0.1:80")
	fmt.Println(h)
	fmt.Println(port)
	fmt.Println(net.JoinHostPort("::1", "443"))

	// Writer first, then reader goroutine — avoids parking ReadFrom alone
	// on the cooperative poll bridge with nothing yet submitted to wake it.
	uln, uerr3 := net.ListenPacket("udp", "127.0.0.1:34569")
	uc, uerr2 := net.DialPacket("udp", "127.0.0.1:34569")
	fmt.Println(uerr2 == nil && uerr3 == nil)
	if uerr2 == nil && uerr3 == nil {
		done3 := make(chan bool)
		go udpListenOnce(uln, done3)
		uc.WriteTo([]byte("pong"), "127.0.0.1:34569")
		<-done3
		uc.Close()
	} else {
		fmt.Println("")
	}
}

func acceptOnce(ln *net.Listener, done chan bool) {
	c, err := ln.Accept()
	if err == nil {
		buf := make([]byte, 2)
		c.Read(buf)
		fmt.Println(string(buf))
		c.Close()
	}
	done <- true
}

func udpSend(c *net.PacketConn) {
	c.WriteTo([]byte("ping"), "pipe")
}

func udpListenOnce(c *net.PacketConn, done chan bool) {
	buf := make([]byte, 8)
	n, _, err := c.ReadFrom(buf)
	if err == nil {
		fmt.Println(string(buf[0:n]))
	}
	c.Close()
	done <- true
}
