// Package syscall: Getpid/Getppid/Environ/Getenv/Chdir/Kill are real on
// a goclang++.bat --shim-sandbox build (gocvm.Call -- see runtime.hpp's
// wasigo::gocvm and shim_sandbox's src/sapi/real_win.cc). Under plain
// wasm32-wasip1 (compile.bat), gocvm.Call itself reports no host bridge
// and these fall back to the same canned values/errors as before
// (Getpid 1, Getppid 0, Getenv/Environ empty, Chdir/Kill "not supported").
package syscall

import (
	"errors"
	"gocvm"
	"strconv"
	"strings"
)

var ErrNotSupported = errors.New(
	"syscall: not supported on wasm32-wasip1 (use WASI / Wasi2G++ shim, not Linux syscalls)")

// gocvm.Call's (string, error): err is only non-nil when there is no
// real answer at all (no bridge). A real bridge's own failure still
// comes back err == nil with the payload starting "error: " -- a
// definitive real answer, not a signal to fall back to a canned value.
func isRealError(reply string) bool {
	return strings.HasPrefix(reply, "error:")
}

// isNoBridge distinguishes "this build has no bridge at all" (the only
// case that should fall back to ErrNotSupported) from every other
// err != nil gocvm.Call can return on a real --shim-sandbox build (ABAC
// deny, a bridge-internal panic, a reentrant call) -- those are genuine
// operational failures and must surface as-is, not get misreported as a
// platform limitation. Only Chdir/Kill below can express that (Getpid/
// Getppid/Environ/Getenv match real Go's own infallible signatures --
// no error return to surface anything through, same bound as real Go's
// Getpid, which also cannot fail).
func isNoBridge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no host bridge registered")
}

func Getpid() int {
	reply, err := gocvm.Call("syscall", "getpid")
	if err != nil {
		return 1
	}
	n, perr := strconv.Atoi(reply)
	if perr != nil {
		return 1
	}
	return n
}

func Getppid() int {
	reply, err := gocvm.Call("syscall", "getppid")
	if err != nil {
		return 0
	}
	n, perr := strconv.Atoi(reply)
	if perr != nil {
		return 0
	}
	return n
}

func Getwd() (string, error) { return ".", nil }

func Chdir(dir string) error {
	reply, err := gocvm.Call("syscall", "chdir "+dir)
	if err != nil {
		if isNoBridge(err) {
			return ErrNotSupported
		}
		return err
	}
	if isRealError(reply) {
		return errors.New(reply)
	}
	return nil
}

func Kill(pid int, sig int) error {
	reply, err := gocvm.Call("syscall", "kill "+strconv.Itoa(pid)+" "+strconv.Itoa(sig))
	if err != nil {
		if isNoBridge(err) {
			return ErrNotSupported
		}
		return err
	}
	if isRealError(reply) {
		return errors.New(reply)
	}
	return nil
}

// reply is \x1f-joined "KEY=VALUE" entries (real_win.cc::Syscall's
// "environ" reply shape, straight from GetEnvironmentStringsW).
func Environ() []string {
	reply, err := gocvm.Call("syscall", "environ")
	if err != nil || reply == "" {
		return nil
	}
	return strings.Split(reply, "\x1f")
}

// reply is "1|<value>" (found) or "0|" (not found) -- real_win.cc::
// Syscall's "getenv" reply shape.
func Getenv(key string) (string, bool) {
	reply, err := gocvm.Call("syscall", "getenv "+key)
	if err != nil || len(reply) == 0 {
		return "", false
	}
	if reply[0] == '1' {
		return reply[2:], true
	}
	return "", false
}

func ByteSliceFromString(s string) ([]byte, error) {
	return append([]byte(s), 0), nil
}
