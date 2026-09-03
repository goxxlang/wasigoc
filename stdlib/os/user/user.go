// Package user: real on a goclang++.bat --shim-sandbox build (real
// GetUserNameW / NetUserGetInfo / LookupAccountNameW+SID via gocvm.Call
// -- see runtime.hpp's wasigo::gocvm and shim_sandbox's src/sapi/
// real_win.cc). Under plain wasm32-wasip1 (compile.bat), gocvm.Call
// itself reports no host bridge and every operation returns the same
// honest "not supported" error as before.
package user

import (
	"errors"
	"gocvm"
	"strings"
)

var errNotSupported = errors.New("os/user: not supported on wasm32-wasip1 (no user database)")

// gocvm.Call's (string, error): err is only non-nil when there is no
// real answer at all (no bridge). A real bridge's own failure (e.g. a
// real "no such user") still comes back err == nil with the payload
// starting "error: " -- a definitive real answer, not a signal to fall
// back to errNotSupported.
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

type User struct {
	Uid      string
	Gid      string
	Username string
	Name     string
	HomeDir  string
}

// "<uid>\x1f<username>\x1f<name>\x1f<homedir>" -- real_win.cc::User's
// reply shape.
func parseUser(reply string) (*User, error) {
	f := strings.Split(reply, "\x1f")
	if len(f) != 4 {
		return nil, errors.New("os/user: malformed reply")
	}
	return &User{Uid: f[0], Username: f[1], Name: f[2], HomeDir: f[3]}, nil
}

func call(op string) (*User, error) {
	reply, err := gocvm.Call("os.user", op)
	if err != nil {
		if isNoBridge(err) {
			return nil, errNotSupported
		}
		return nil, err
	}
	if isRealError(reply) {
		return nil, errors.New(reply)
	}
	return parseUser(reply)
}

func Current() (*User, error) {
	return call("")
}

func Lookup(username string) (*User, error) {
	return call("lookup " + username)
}

func LookupId(uid string) (*User, error) {
	return call("lookupid " + uid)
}
