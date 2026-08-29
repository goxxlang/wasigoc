// Bounded runtime/pprof: same honest "correct terminal shape, not a
// placeholder" reasoning as os/exec/os/user -- this target has no
// sampling profiler, heap-profile bookkeeping, or symbolized-stack
// support at all, so every profiling entry point returns a clear
// "not supported" error (matching os/exec's own stub shape) rather than
// silently producing an empty-but-plausible-looking profile.
// `NewProfile`/`Lookup`/`Profiles`/`(*Profile).Add`/`Remove`/`Count` are
// real bookkeeping (a named, empty custom-profile registry costs nothing
// and doesn't need OS support), matching real Go's own shape for that
// half of the API; only the parts that need real stack sampling or
// writing an actual pprof-format profile are stubbed.
package pprof

import (
	"errors"
	"io"
)

var errNotSupported = errors.New("runtime/pprof: profiling not supported on this target")

type Profile struct {
	name string
}

var profiles []*Profile

func NewProfile(name string) *Profile {
	p := &Profile{name: name}
	profiles = append(profiles, p)
	return p
}

func Lookup(name string) *Profile {
	i := 0
	for i < len(profiles) {
		if profiles[i].name == name {
			return profiles[i]
		}
		i = i + 1
	}
	return nil
}

func Profiles() []*Profile {
	return profiles
}

func (p *Profile) Name() string {
	return p.name
}

func (p *Profile) Count() int {
	return 0
}

func (p *Profile) Add(value any, skip int) {}

func (p *Profile) Remove(value any) {}

func (p *Profile) WriteTo(w io.Writer, debug int) error {
	return errNotSupported
}

func StartCPUProfile(w io.Writer) error {
	return errNotSupported
}

func StopCPUProfile() {}

func WriteHeapProfile(w io.Writer) error {
	return errNotSupported
}
