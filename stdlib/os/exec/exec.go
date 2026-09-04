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

	started    bool
	handle     string
	stdoutDone chan bool
	stderrDone chan bool
	stdinDone  chan bool
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

type sliceWriter struct {
	buf *[]byte
}

func (w *sliceWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// Run/Output/CombinedOutput all route through Start+Wait, which gives
// stdout and stderr independent real pipes (unlike the older one-shot
// "os.exec" topic, still valid for direct gocvm.Call use but unused by
// this package now) -- Output can therefore actually isolate stdout the
// way real Go's does, and CombinedOutput's two pumps sharing one buffer
// gets the same non-deterministic interleaving real Go's own
// CombinedOutput has (it also copies two separate pipes concurrently).
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Cmd) Output() ([]byte, error) {
	var out []byte
	c.Stdout = &sliceWriter{&out}
	err := c.Run()
	return out, err
}

func (c *Cmd) CombinedOutput() ([]byte, error) {
	var out []byte
	w := &sliceWriter{&out}
	c.Stdout = w
	c.Stderr = w
	err := c.Run()
	return out, err
}

// pumpStream drains one of the child's real output pipes into w (a
// no-op sink, not a skipped goroutine, when w is nil -- Start always
// runs both stdout and stderr pumps so neither pipe can fill up and
// block the child just because the caller didn't ask for that stream)
// until EOF, then signals done.
func (c *Cmd) pumpStream(topic string, w io.Writer, done chan bool) {
	for {
		reply, err := gocvm.Call(topic, c.handle+"\x1f"+"4096")
		if err != nil || isRealError(reply) || reply == "" {
			break
		}
		if w != nil {
			w.Write([]byte(reply))
		}
	}
	done <- true
}

// writeStdin drains c.Stdin into the child's real stdin pipe until EOF,
// then closes the pipe so the child sees its own EOF, matching real
// Go's Cmd.Wait semantics of closing Stdin after copying finishes.
func (c *Cmd) writeStdin() {
	buf := make([]byte, 4096)
	for {
		n, rerr := c.Stdin.Read(buf)
		if n > 0 {
			reply, err := gocvm.Call("os.exec.stdin.write", c.handle+"\x1f"+string(buf[:n]))
			if err != nil || isRealError(reply) {
				break
			}
		}
		if rerr != nil {
			break
		}
	}
	gocvm.Call("os.exec.stdin.close", c.handle)
	c.stdinDone <- true
}

// Start launches the command for real (goclang++.bat --shim-sandbox)
// without waiting for it to exit. Stdout and stderr each get their own
// real pipe and their own pump goroutine, always -- not just when the
// caller set that field -- since an undrained pipe can block the child
// once its buffer fills. If Stdin is set, a background goroutine streams
// it into the child's real stdin pipe. Wait below joins all goroutines
// before returning so all I/O is flushed first, matching real Go's
// Cmd.Wait semantics.
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
	c.stdoutDone = make(chan bool, 1)
	go c.pumpStream("os.exec.stdout.read", c.Stdout, c.stdoutDone)
	c.stderrDone = make(chan bool, 1)
	go c.pumpStream("os.exec.stderr.read", c.Stderr, c.stderrDone)
	if c.Stdin != nil {
		c.stdinDone = make(chan bool, 1)
		go c.writeStdin()
	}
	return nil
}

func (c *Cmd) Wait() error {
	if !c.started {
		return errNotSupported
	}
	<-c.stdoutDone
	<-c.stderrDone
	if c.stdinDone != nil {
		<-c.stdinDone
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
	reply, err := gocvm.Call("os.exec.lookpath", file)
	if err != nil {
		if isNoBridge(err) {
			return "", errNotSupported
		}
		return "", err
	}
	if isRealError(reply) {
		return "", ErrNotFound
	}
	const p = "ok "
	if !strings.HasPrefix(reply, p) {
		return "", errors.New("exec: malformed lookpath reply")
	}
	return reply[len(p):], nil
}
