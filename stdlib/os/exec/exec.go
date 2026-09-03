// Package exec: real on a goclang++.bat --shim-sandbox build (real
// CreateProcess via gocvm.Call -- see runtime.hpp's wasigo::gocvm and
// shim_sandbox's src/sapi/real_win.cc). Under plain wasm32-wasip1
// (compile.bat), gocvm.Call itself reports no host bridge and every
// operation returns the same honest "not supported" error as before.
package exec

import (
	"errors"
	"gocvm"
	"io"
	"strconv"
	"strings"
)

var ErrNotFound = errors.New("exec: executable file not found in $PATH")

var errNotSupported = errors.New(
	"exec: not supported on wasm32-wasip1 (WASI preview 1 has no subprocess support)")

// gocvm.Call's (string, error): err is only non-nil when there is no
// real answer at all (no bridge). A real bridge's own failure (a real
// CreateProcess error, ...) still comes back err == nil with the
// payload starting "error: " -- a definitive real answer, not a signal
// to fall back to errNotSupported.
func isRealError(reply string) bool {
	return strings.HasPrefix(reply, "error:")
}

// isNoBridge distinguishes "this build has no bridge at all" (the only
// case that should fall back to errNotSupported) from every other
// err != nil gocvm.Call can return on a real --shim-sandbox build (ABAC
// deny, a bridge-internal panic, a reentrant call) -- those are genuine
// operational failures and must surface as-is, not get misreported as a
// platform limitation.
func isNoBridge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no host bridge registered")
}

type Cmd struct {
	Path   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	started  bool
	handle   string
	pumpDone chan bool
}

func Command(name string, arg ...string) *Cmd {
	c := &Cmd{Path: name}
	c.Args = append(c.Args, name)
	c.Args = append(c.Args, arg...)
	return c
}

func (c *Cmd) argv() string {
	s := ""
	for i, a := range c.Args {
		if i > 0 {
			s = s + "\x1f"
		}
		s = s + a
	}
	return s
}

// exit=<n>\n<output> -- real_win.cc::Exec's reply shape.
func parseExecReply(reply string) (int, string) {
	if !strings.HasPrefix(reply, "exit=") {
		return -1, reply
	}
	i := strings.Index(reply, "\n")
	if i < 0 {
		return -1, reply[5:]
	}
	n, err := strconv.Atoi(reply[5:i])
	if err != nil {
		return -1, reply[i+1:]
	}
	return n, reply[i+1:]
}

// "exit=<n>" -- real_win.cc::ExecWait's reply shape (no trailing output).
func parseExitCode(reply string) int {
	if !strings.HasPrefix(reply, "exit=") {
		return -1
	}
	n, err := strconv.Atoi(reply[5:])
	if err != nil {
		return -1
	}
	return n
}

// "ok handle=<id>" -- real_win.cc::ExecStart's reply shape.
func parseStartHandle(reply string) string {
	const p = "handle="
	i := strings.Index(reply, p)
	if i < 0 {
		return ""
	}
	return reply[i+len(p):]
}

// CombinedOutput runs the command for real when a gocvm host bridge is
// registered. The real backend redirects stdout+stderr to the same pipe
// (see shim_sandbox's docs/architecture.md), so unlike real Go's
// os/exec, Output() below can't isolate stdout alone -- it returns the
// same combined bytes CombinedOutput() does.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	reply, err := gocvm.Call("os.exec", c.argv())
	if err != nil {
		if isNoBridge(err) {
			return nil, errNotSupported
		}
		return nil, err
	}
	if isRealError(reply) {
		return nil, errors.New(reply)
	}
	code, out := parseExecReply(reply)
	if code != 0 {
		return []byte(out), errors.New("exit status " + strconv.Itoa(code))
	}
	return []byte(out), nil
}

func (c *Cmd) Output() ([]byte, error) {
	return c.CombinedOutput()
}

func (c *Cmd) Run() error {
	_, err := c.CombinedOutput()
	return err
}

// pump drains the child's combined stdout+stderr into whichever of
// Stdout/Stderr is set (Stdout preferred -- the real backend can't
// separate the two streams, see the package doc above) until EOF, then
// signals pumpDone so Wait knows output has been fully flushed.
func (c *Cmd) pump() {
	w := c.Stdout
	if w == nil {
		w = c.Stderr
	}
	for {
		reply, err := gocvm.Call("os.exec.stdout.read", c.handle+"\x1f"+"4096")
		if err != nil || isRealError(reply) || reply == "" {
			break
		}
		if w != nil {
			w.Write([]byte(reply))
		}
	}
	c.pumpDone <- true
}

// Start launches the command for real (goclang++.bat --shim-sandbox)
// without waiting for it to exit. If Stdout or Stderr is set, a
// background goroutine streams the child's combined output into it as
// it arrives; Wait below joins that goroutine before returning so all
// output is flushed first, matching real Go's Cmd.Wait semantics.
func (c *Cmd) Start() error {
	reply, err := gocvm.Call("os.exec.start", c.argv())
	if err != nil {
		if isNoBridge(err) {
			return errNotSupported
		}
		return err
	}
	if isRealError(reply) {
		return errors.New(reply)
	}
	h := parseStartHandle(reply)
	if h == "" {
		return errors.New("exec: malformed start reply")
	}
	c.handle = h
	c.started = true
	if c.Stdout != nil || c.Stderr != nil {
		c.pumpDone = make(chan bool, 1)
		go c.pump()
	}
	return nil
}

func (c *Cmd) Wait() error {
	if !c.started {
		return errNotSupported
	}
	if c.pumpDone != nil {
		<-c.pumpDone
	}
	reply, err := gocvm.Call("os.exec.wait", c.handle)
	if err != nil {
		if isNoBridge(err) {
			return errNotSupported
		}
		return err
	}
	if isRealError(reply) {
		return errors.New(reply)
	}
	code := parseExitCode(reply)
	if code != 0 {
		return errors.New("exit status " + strconv.Itoa(code))
	}
	return nil
}

func LookPath(file string) (string, error) {
	return "", errNotSupported
}
