// A deliberate stub, same shape as os/exec: WASI preview1 has no user
// database (no getpwnam/getpwuid equivalent, no ambient identity
// syscalls at all), so Current/Lookup/LookupId return a clear
// "not supported" error rather than silently pretending to work.
package user

import "errors"

type User struct {
	Uid      string
	Gid      string
	Username string
	Name     string
	HomeDir  string
}

func Current() (*User, error) {
	return nil, errors.New("os/user: not supported on wasm32-wasip1 (no user database)")
}

func Lookup(username string) (*User, error) {
	return nil, errors.New("os/user: not supported on wasm32-wasip1 (no user database)")
}

func LookupId(uid string) (*User, error) {
	return nil, errors.New("os/user: not supported on wasm32-wasip1 (no user database)")
}
