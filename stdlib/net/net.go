// Package net is a stub: WASI preview 1 (this project's actual compile
// target, wasm32-wasip1) has no sockets at all -- wasi-libc's own
// sys/socket.h compiles socket()/connect()/bind()/listen()/accept() out
// entirely whenever __wasip1__ is defined:
//
//	#if (defined __wasilibc_unmodified_upstream) || !(defined __wasip1__)
//	int socket (int, int, int);
//	#endif
//
// verified by reading that header directly, not assumed. This is a real,
// permanent platform limitation for this target, not a missing wasmtime
// flag or a todo -- see README's "net/net/http status" note for the full
// story. A working net/http would need the wasm32-wasip2 Component Model
// target instead (which has a real wasi:sockets interface), a different
// ABI this whole project isn't built around; not attempted here.
//
// Pipe() is the one part of this package that doesn't need sockets at
// all -- an in-memory, synchronous, full-duplex connection between two
// goroutines (real Go's own net.Pipe is the same idea, just backed by a
// mutex/cond pair there instead of channels). Dial/Listen -- anything
// that would need to reach an actual address -- stay stubbed for the
// reason above.
package net

import "errors"

var errNotSupported = errors.New("net: not supported on wasm32-wasip1 (WASI preview 1 has no socket syscalls)")
var errClosedPipe = errors.New("net: pipe closed")

type Conn struct {
	valid    bool
	closed   bool
	recv     chan []byte
	send     chan []byte
	leftover []byte
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

// Pipe returns two Conns connected to each other: a write on one side is
// delivered whole to a Read on the other (unbuffered -- Write blocks
// until the peer reads it, matching real net.Pipe's synchronous
// behavior). Closing either end delivers EOF to the peer's next Read
// once its buffered data is drained, and makes both Read and Write on
// the closed end itself return errClosedPipe from then on.
func Pipe() (*Conn, *Conn) {
	ab := make(chan []byte)
	ba := make(chan []byte)
	a := &Conn{valid: true, recv: ba, send: ab}
	b := &Conn{valid: true, recv: ab, send: ba}
	return a, b
}

type Listener struct{}

func (l *Listener) Accept() (*Conn, error) { return nil, errNotSupported }
func (l *Listener) Close() error           { return errNotSupported }

func Dial(network string, address string) (*Conn, error) {
	return nil, errNotSupported
}

func Listen(network string, address string) (*Listener, error) {
	return nil, errNotSupported
}
