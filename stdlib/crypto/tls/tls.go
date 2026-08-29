// Package tls is a stub: a real handshake needs sockets (see net) plus
// x509 chain verification this project's crypto/x509 doesn't do. Same
// honest-terminal-shape as os/exec -- Dial/LoadX509KeyPair return a
// clear "not supported" error so source that imports crypto/tls still
// compiles.
package tls

import "errors"

var ErrNotSupported = errors.New(
	"tls: not supported on wasm32-wasip1 (needs sockets and x509 chain verification)")

type Config struct {
	ServerName string
}

type Conn struct {
	valid bool
}

func Dial(network string, addr string, config *Config) (*Conn, error) {
	return nil, ErrNotSupported
}

func (c *Conn) Handshake() error { return ErrNotSupported }

func (c *Conn) Close() error { return ErrNotSupported }

func LoadX509KeyPair(certFile string, keyFile string) error {
	return ErrNotSupported
}
