// Package tls: real on a goclang++.bat --shim-sandbox build (a real
// Schannel/SSPI handshake, automatic certificate chain + hostname
// validation always on -- see shim_sandbox's src/sapi/tls_win.cc --
// via gocvm.Call, runtime.hpp's wasigo::gocvm). Under plain
// wasm32-wasip1 (compile.bat), gocvm.Call itself reports no host
// bridge and every operation returns the same honest "not supported"
// error as before. LoadX509KeyPair (client certificates) stays
// stubbed -- out of scope, no ordinary HTTPS client needs it.
package tls

import (
	"errors"
	"gocvm"
	"strconv"
	"strings"
)

var ErrNotSupported = errors.New(
	"tls: not supported on wasm32-wasip1 (needs sockets and x509 chain verification)")

// gocvm.Call's (string, error): err is only non-nil when there is no
// real answer at all (no bridge). A real bridge's own failure (a real
// handshake or certificate validation failure, a real connect error)
// still comes back err == nil with the payload starting "error: " -- a
// definitive real answer, not a signal to fall back to ErrNotSupported.
func isRealError(reply string) bool {
	return strings.HasPrefix(reply, "error:")
}

// isNoBridge distinguishes "this build has no bridge at all" (the only
// case that should fall back to ErrNotSupported) from every other
// err != nil gocvm.Call can return on a real --shim-sandbox build (ABAC
// deny, a bridge-internal panic, a reentrant call) -- those are genuine
// operational failures and must surface as-is, not get misreported as a
// platform limitation.
func isNoBridge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no host bridge registered")
}

type Config struct {
	ServerName string
}

type Conn struct {
	valid  bool
	closed bool
	real   bool
	handle string
}

// "ok handle=<id>" -- real_win.cc's TlsDial reply shape.
func parseDialHandle(reply string) string {
	const p = "handle="
	i := strings.Index(reply, p)
	if i < 0 {
		return ""
	}
	return reply[i+len(p):]
}

// Dial connects and completes the full TLS handshake in one call (same
// as real Go's tls.Dial) -- Handshake() below is a no-op once Dial has
// succeeded.
func Dial(network string, addr string, config *Config) (*Conn, error) {
	reply, err := gocvm.Call("tls.dial", addr)
	if err != nil {
		if isNoBridge(err) {
			return nil, ErrNotSupported
		}
		return nil, err
	}
	if isRealError(reply) {
		return nil, errors.New(reply)
	}
	h := parseDialHandle(reply)
	if h == "" {
		return nil, errors.New("tls: malformed dial reply")
	}
	return &Conn{valid: true, real: true, handle: h}, nil
}

func (c *Conn) Read(p []byte) (int, error) {
	if c == nil || !c.valid || !c.real {
		return 0, ErrNotSupported
	}
	if c.closed {
		return 0, errors.New("tls: connection closed")
	}
	reply, err := gocvm.Call("tls.io.read", c.handle+"\x1f"+strconv.Itoa(len(p)))
	if err != nil {
		if isNoBridge(err) {
			return 0, ErrNotSupported
		}
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

func (c *Conn) Write(p []byte) (int, error) {
	if c == nil || !c.valid || !c.real {
		return 0, ErrNotSupported
	}
	if c.closed {
		return 0, errors.New("tls: connection closed")
	}
	reply, err := gocvm.Call("tls.io.write", c.handle+"\x1f"+string(p))
	if err != nil {
		if isNoBridge(err) {
			return 0, ErrNotSupported
		}
		return 0, err
	}
	if isRealError(reply) {
		return 0, errors.New(reply)
	}
	return len(p), nil
}

// Handshake is a no-op returning nil once Dial has already completed
// it (matches real Go: tls.Dial already returns a handshaken Conn).
func (c *Conn) Handshake() error {
	if c == nil || !c.valid || !c.real {
		return ErrNotSupported
	}
	return nil
}

func (c *Conn) Close() error {
	if c == nil || !c.valid {
		return ErrNotSupported
	}
	if c.closed {
		return nil
	}
	c.closed = true
	if c.real {
		gocvm.Call("tls.io.close", c.handle)
	}
	return nil
}

func LoadX509KeyPair(certFile string, keyFile string) error {
	return ErrNotSupported
}
