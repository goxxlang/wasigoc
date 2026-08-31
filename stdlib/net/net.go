// Package net: WASI preview 1 has no socket syscalls. Dial/Listen still
// work as a userspace stack: TCP is net.Pipe (reliable duplex), UDP is
// length-prefixed datagrams over a Pipe, HTTP is HTTP/1.0 on those Conns.
//
//   Dial("tcp", addr)           // fails unless a matching Listen is waiting
//   Listen("tcp", addr)         // Accept receives the Pipe peer from Dial
//   ListenPacket("udp", addr)   // UDP listen; DialPacket attaches a Pipe
//   DialPacket("udp", addr)     // connected UDP to a ListenPacket
//   PacketPipe()                // UDP-shaped datagram pair (framed Pipe)
//
// Unknown hosts (example.com) still error — there is no host TCP/UDP.
package net

import (
	"errors"
	"strings"
)

var errNotSupported = errors.New("net: not supported on wasm32-wasip1 (WASI preview 1 has no socket syscalls)")
var errClosedPipe = errors.New("net: pipe closed")
var errRefused = errors.New("net: connection refused")
var errClosed = errors.New("net: listener closed")

type Conn struct {
	valid    bool
	closed   bool
	recv     chan []byte
	send     chan []byte
	leftover []byte
	laddr    string
	raddr    string
}

func (c *Conn) Read(p []byte) (int, error) {
	if !c.valid {
		return 0, errNotSupported
	}
	if c.closed {
		return 0, errClosedPipe
	}
	if len(c.leftover) == 0 {
		chunk, ok := <-c.recv
		if !ok {
			return 0, errors.New("EOF")
		}
		c.leftover = chunk
	}
	n := copy(p, c.leftover)
	c.leftover = c.leftover[n:]
	return n, nil
}

func (c *Conn) Write(p []byte) (int, error) {
	if !c.valid {
		return 0, errNotSupported
	}
	if c.closed {
		return 0, errClosedPipe
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	c.send <- cp
	return len(p), nil
}

func (c *Conn) Close() error {
	if !c.valid {
		return errNotSupported
	}
	if c.closed {
		return nil
	}
	c.closed = true
	close(c.send)
	return nil
}

func (c *Conn) LocalAddr() string  { return c.laddr }
func (c *Conn) RemoteAddr() string { return c.raddr }

func Pipe() (*Conn, *Conn) {
	ab := make(chan []byte, 16)
	ba := make(chan []byte, 16)
	a := &Conn{valid: true, recv: ba, send: ab}
	b := &Conn{valid: true, recv: ab, send: ba}
	return a, b
}

type Listener struct {
	valid  bool
	closed bool
	addr   string
	accept chan *Conn
}

func (l *Listener) Addr() string { return l.addr }

func (l *Listener) Accept() (*Conn, error) {
	if l == nil || !l.valid {
		return nil, errNotSupported
	}
	if l.closed {
		return nil, errClosed
	}
	c, ok := <-l.accept
	if !ok {
		return nil, errClosed
	}
	return c, nil
}

func (l *Listener) Close() error {
	if l == nil || !l.valid {
		return errNotSupported
	}
	if l.closed {
		return nil
	}
	l.closed = true
	close(l.accept)
	forgetTCP(l.addr)
	return nil
}

var tcpBound = map[string]*Listener{}

func forgetTCP(address string) {
	tcpBound[address] = nil
}

func Listen(network string, address string) (*Listener, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errNotSupported
	}
	ln := &Listener{valid: true, addr: address, accept: make(chan *Conn, 1)}
	tcpBound[address] = ln
	return ln, nil
}

func Dial(network string, address string) (*Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errNotSupported
	}
	ln, ok := tcpBound[address]
	if !ok || ln == nil || ln.closed {
		return nil, errRefused
	}
	a, b := Pipe()
	a.laddr = address
	a.raddr = "pipe"
	b.laddr = "pipe"
	b.raddr = address
	go enqueueTCP(ln, a)
	return b, nil
}

func enqueueTCP(ln *Listener, c *Conn) {
	if ln == nil || ln.closed {
		c.Close()
		return
	}
	ln.accept <- c
}

type PacketConn struct {
	valid  bool
	closed bool
	listen bool
	conn   *Conn
	laddr  string
	raddr  string
	attach chan *Conn
}

func (c *PacketConn) LocalAddr() string  { return c.laddr }
func (c *PacketConn) RemoteAddr() string { return c.raddr }

func (c *PacketConn) Close() error {
	if c == nil || !c.valid {
		return errNotSupported
	}
	if c.closed {
		return nil
	}
	c.closed = true
	if c.listen {
		forgetUDP(c.laddr)
		close(c.attach)
		if c.conn == nil {
			return nil
		}
	}
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func readFull(c *Conn, p []byte) error {
	off := 0
	for off < len(p) {
		n, err := c.Read(p[off:])
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("EOF")
		}
		off = off + n
	}
	return nil
}

func (c *PacketConn) WriteTo(p []byte, addr string) (int, error) {
	if c == nil || !c.valid || c.closed {
		return 0, errClosedPipe
	}
	if c.conn == nil {
		return 0, errClosedPipe
	}
	n := len(p)
	hdr := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	if _, err := c.conn.Write(hdr); err != nil {
		return 0, err
	}
	if n > 0 {
		if _, err := c.conn.Write(p); err != nil {
			return 0, err
		}
	}
	return n, nil
}

func (c *PacketConn) ReadFrom(p []byte) (int, string, error) {
	if c == nil || !c.valid || c.closed {
		return 0, "", errClosedPipe
	}
	if c.listen && c.conn == nil {
		peer, ok := <-c.attach
		if !ok || peer == nil {
			return 0, "", errClosed
		}
		c.conn = peer
	}
	if c.conn == nil {
		return 0, "", errClosedPipe
	}
	hdr := make([]byte, 4)
	if err := readFull(c.conn, hdr); err != nil {
		return 0, "", err
	}
	n := int(hdr[0])<<24 | int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
	if n < 0 {
		return 0, "", errClosedPipe
	}
	buf := make([]byte, n)
	if n > 0 {
		if err := readFull(c.conn, buf); err != nil {
			return 0, "", err
		}
	}
	copyN := copy(p, buf)
	return copyN, "pipe", nil
}

func PacketPipe() (*PacketConn, *PacketConn) {
	a, b := Pipe()
	pa := &PacketConn{valid: true, conn: a, laddr: "pipe", raddr: "pipe"}
	pb := &PacketConn{valid: true, conn: b, laddr: "pipe", raddr: "pipe"}
	return pa, pb
}

var udpBound = map[string]*PacketConn{}

func forgetUDP(address string) {
	udpBound[address] = nil
}

func ListenPacket(network string, address string) (*PacketConn, error) {
	if network != "udp" && network != "udp4" && network != "udp6" {
		return nil, errNotSupported
	}
	ln := &PacketConn{valid: true, listen: true, laddr: address, attach: make(chan *Conn, 1)}
	udpBound[address] = ln
	return ln, nil
}

func DialPacket(network string, address string) (*PacketConn, error) {
	if network != "udp" && network != "udp4" && network != "udp6" {
		return nil, errNotSupported
	}
	ln, ok := udpBound[address]
	if !ok || ln == nil || ln.closed {
		return nil, errRefused
	}
	a, b := Pipe()
	a.laddr = address
	a.raddr = "pipe"
	b.laddr = "pipe"
	b.raddr = address
	go enqueueUDP(ln, a)
	return &PacketConn{valid: true, conn: b, laddr: "pipe", raddr: address}, nil
}

func enqueueUDP(ln *PacketConn, c *Conn) {
	if ln == nil || ln.closed {
		c.Close()
		return
	}
	ln.attach <- c
}

func SplitHostPort(hostport string) (string, string, error) {
	if len(hostport) == 0 {
		return "", "", errors.New("missing port in address")
	}
	if hostport[0] == 91 {
		end := strings.Index(hostport, "]:")
		if end < 0 {
			return "", "", errors.New("missing port in address")
		}
		return hostport[1:end], hostport[end+2:], nil
	}
	i := strings.LastIndex(hostport, ":")
	if i < 0 {
		return "", "", errors.New("missing port in address")
	}
	return hostport[0:i], hostport[i+1:], nil
}

func JoinHostPort(host string, port string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}
