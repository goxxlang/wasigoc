// Package exec is a stub: wasm32-wasip1 (WASI preview 1) has no fork/exec
// -- there is no way to actually run a subprocess on this target at all,
// full stop, not a "not implemented yet" gap. This package exists purely
// so Go source that imports os/exec still *compiles*: every operation
// returns a clear error at runtime instead of the import itself being a
// hard compile failure, so a ported program can at least build and report
// "subprocess not supported here" through its own normal error handling.
package exec

import (
	"errors"
	"io"
)

var ErrNotFound = errors.New("exec: executable file not found in $PATH")

var errNotSupported = errors.New(
	"exec: not supported on wasm32-wasip1 (WASI preview 1 has no subprocess support)")

type Cmd struct {
	Path   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

func Command(name string, arg ...string) *Cmd {
	c := &Cmd{Path: name}
	c.Args = append(c.Args, name)
	c.Args = append(c.Args, arg...)
	return c
}

func (c *Cmd) Run() error {
	return errNotSupported
}

func (c *Cmd) Start() error {
	return errNotSupported
}

func (c *Cmd) Wait() error {
	return errNotSupported
}

func (c *Cmd) Output() ([]byte, error) {
	return nil, errNotSupported
}

func (c *Cmd) CombinedOutput() ([]byte, error) {
	return nil, errNotSupported
}

func LookPath(file string) (string, error) {
	return "", errNotSupported
}
