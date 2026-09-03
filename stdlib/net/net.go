// Package net: real TCP/UDP on a goclang++.bat --shim-sandbox build
// (gocvm.Call -- see runtime.hpp's wasigo::gocvm and shim_sandbox's
// src/sapi/real_win.cc), including a real Listener.Accept()/Conn that
// actually moves bytes, not just a reachability probe. Under plain
// wasm32-wasip1 (compile.bat), gocvm.Call itself reports no host
// bridge and Dial/Listen fall back to the original userspace stack
// below: TCP is net.Pipe (reliable duplex), UDP is length-prefixed
// datagrams over a Pipe, both entirely local to this process.
//
//   Dial("tcp", addr)           // real if a bridge is linked, else
//                                // only matches a local Listen via Pipe
//   Listen("tcp", addr)         // Accept receives real or local Conns
//   ListenPacket("udp", addr)   // UDP listen; DialPacket attaches a Pipe
//   DialPacket("udp", addr)     // connected UDP to a ListenPacket
//   PacketPipe()                // UDP-shaped datagram pair (framed Pipe)
package net

import (
	"errors"
	"gocvm"
	"strconv"
	"strings"
)

var errNotSupported = errors.New("net: not supported on wasm32-wasip1 (WASI preview 1 has no socket syscalls)")
var errClosedPipe = errors.New("net: pipe closed")
var errRefused = errors.New("net: connection refused")
var errClosed = errors.New("net: listener closed")

// gocvm.Call's (string, error): err is only non-nil when there is no
// real answer at all (no bridge registered, ABAC deny, unknown topic --
// see wasigo::gocvm::Call in runtime.hpp). A real bridge's own failure
// (a real connect() refused, a real accept() error, ...) still comes
// back as err == nil with the payload starting "error: " (real_win.cc's
// own convention throughout) -- that is a definitive real answer, not a
// signal to fall back to the local-only stub behavior below.
func isRealError(reply string) bool {
	return strings.HasPrefix(reply, "error:")
}

// isNoBridge distinguishes "this build has no bridge at all" (the only
// case that should fall back to the local-only stack below) from every
// other err != nil gocvm.Call can return on a real --shim-sandbox build
// (ABAC deny, a bridge-internal panic, a reentrant call) -- those are
// genuine operational failures on a build that DOES have a real bridge
// and must surface as-is, not silently downgrade to a local-only Conn/
// Listener that can never reach the address the caller actually asked
// for.
func isNoBridge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no host bridge registered")
}

// "handle=<id>" optionally followed by " remote=<addr>" -- the shape
// net.dial/listen/accept's real replies share (see
// shim_sandbox/docs/architecture.md's topics table).
func parseHandleReply(reply string) (handle string, remote string) {
	i := strings.Index(reply, "handle=")
	if i < 0 {
		return "", ""
	}
	rest := reply[i+len("handle="):]
	sp := strings.Index(rest, " ")
	if sp < 0 {
		return rest, ""
	}
	handle = rest[0:sp]
	if ri := strings.Index(rest[sp+1:], "remote="); ri >= 0 {
		remote = rest[sp+1+ri+len("remote="):]
	}
	return handle, remote
}

type Conn struct {
	valid    bool
	closed   bool
	recv     chan []byte
	send     chan []byte
	leftover []byte
	laddr    string
	raddr    string
	real     bool
	handle   string
}

func (c *Conn) Read(p []byte) (int, error) {
	if !c.valid {
		return 0, errNotSupported
	}
	if c.closed {
		return 0, errClosedPipe
	}
	if c.real {
		reply, err := gocvm.Call("net.io.read", c.handle+"\x1f"+strconv.Itoa(len(p)))
		if err != nil {
			return 0, err
		}
		if isRealError(reply) {
			return 0, errors.New(reply)
		}
		if reply == "" {
			return 0, errors.New("EOF")
		}
		n := copy(p, []byte(reply))
		return n, nil
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
	if c.real {
		reply, err := gocvm.Call("net.io.write", c.handle+"\x1f"+string(p))
		if err != nil {
			return 0, err
		}
		if isRealError(reply) {
			return 0, errors.New(reply)
		}
		return len(p), nil
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
	if c.real {
		gocvm.Call("net.io.close", c.handle)
		return nil
	}
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
	real   bool
	handle string
}

func (l *Listener) Addr() string { return l.addr }

func (l *Listener) Accept() (*Conn, error) {
	if l == nil || !l.valid {
		return nil, errNotSupported
	}
	if l.closed {
		return nil, errClosed
	}
	if l.real {
		reply, err := gocvm.Call("net.accept", l.handle)
		if err != nil {
			return nil, err
		}
		if isRealError(reply) {
			return nil, errors.New(reply)
		}
		h, remote := parseHandleReply(reply)
		return &Conn{valid: true, real: true, handle: h, laddr: l.addr, raddr: remote}, nil
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
	if l.real {
		gocvm.Call("net.io.close", l.handle)
		return nil
	}
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
	reply, err := gocvm.Call("net.listen", network+" "+address)
	if err == nil {
		if isRealError(reply) {
			return nil, errors.New(reply)
		}
		h, _ := parseHandleReply(reply)
		return &Listener{valid: true, real: true, handle: h, addr: address, accept: make(chan *Conn, 1)}, nil
	}
	if !isNoBridge(err) {
		return nil, err
	}
	ln := &Listener{valid: true, addr: address, accept: make(chan *Conn, 1)}
	tcpBound[address] = ln
	return ln, nil
}

func Dial(network string, address string) (*Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errNotSupported
	}
	reply, err := gocvm.Call("net.dial", network+" "+address)
	if err == nil {
		if isRealError(reply) {
			return nil, errors.New(reply)
		}
		h, _ := parseHandleReply(reply)
		return &Conn{valid: true, real: true, handle: h, laddr: "", raddr: address}, nil
	}
	if !isNoBridge(err) {
		return nil, err
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
	real   bool
	handle string
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
	if c.real {
		gocvm.Call("net.io.close", c.handle)
		return nil
	}
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
	if c.real {
		reply, err := gocvm.Call("net.io.writeto", c.handle+"\x1f"+addr+"\x1f"+string(p))
		if err != nil {
			return 0, err
		}
		if isRealError(reply) {
			return 0, errors.New(reply)
		}
		return len(p), nil
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
	if c.real {
		reply, err := gocvm.Call("net.io.readfrom", c.handle+"\x1f"+strconv.Itoa(len(p)))
		if err != nil {
			return 0, "", err
		}
		if isRealError(reply) {
			return 0, "", errors.New(reply)
		}
		i := strings.Index(reply, "\x1f")
		if i < 0 {
			return 0, "", errors.New("net: malformed readfrom reply")
		}
		from := reply[0:i]
		data := reply[i+1:]
		n := copy(p, []byte(data))
		return n, from, nil
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
	// net.listen does a real bind() and, for a UDP network, correctly
	// skips the (TCP-only) listen() syscall (see real_win.cc's
	// BindReal) -- a real UDP "listener" is just a bound socket, ready
	// for ReadFrom/WriteTo below.
	reply, err := gocvm.Call("net.listen", network+" "+address)
	if err == nil {
		if isRealError(reply) {
			return nil, errors.New(reply)
		}
		h, _ := parseHandleReply(reply)
		return &PacketConn{valid: true, real: true, handle: h, laddr: address}, nil
	}
	if !isNoBridge(err) {
		return nil, err
	}
	ln := &PacketConn{valid: true, listen: true, laddr: address, attach: make(chan *Conn, 1)}
	udpBound[address] = ln
	return ln, nil
}

func DialPacket(network string, address string) (*PacketConn, error) {
	if network != "udp" && network != "udp4" && network != "udp6" {
		return nil, errNotSupported
	}
	reply, err := gocvm.Call("net.dial", network+" "+address)
	if err == nil {
		if isRealError(reply) {
			return nil, errors.New(reply)
		}
		h, _ := parseHandleReply(reply)
		return &PacketConn{valid: true, real: true, handle: h, laddr: "", raddr: address}, nil
	}
	if !isNoBridge(err) {
		return nil, err
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
