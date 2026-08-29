// Package syscall is a stub: wasm32-wasip1 has WASI, not Linux/BSD
// syscalls, and this project talks to the host through wasi-libc /
// Wasi2G++'s ABAC shim rather than a raw syscall table. Getpid returns
// 1 (one process); Getwd returns "."; everything that would actually
// issue a syscall returns a clear "not supported" error. Same honest
// terminal shape as os/exec.
package syscall

import "errors"

var ErrNotSupported = errors.New(
	"syscall: not supported on wasm32-wasip1 (use WASI / Wasi2G++ shim, not Linux syscalls)")

func Getpid() int  { return 1 }
func Getppid() int { return 0 }

func Getwd() (string, error) { return ".", nil }

func Chdir(dir string) error { return ErrNotSupported }

func Kill(pid int, sig int) error { return ErrNotSupported }

func Environ() []string { return nil }

func Getenv(key string) (string, bool) { return "", false }

func ByteSliceFromString(s string) ([]byte, error) {
	return append([]byte(s), 0), nil
}
