// Bounded runtime/coverage: same "correct terminal shape" reasoning as
// runtime/pprof/runtime/trace -- this target has no coverage
// instrumentation at all (no `go build -cover`-equivalent here), so
// every entry point returns a clear "not supported" error. Bounded to
// the io.Writer-based half of real Go's API (`WriteMeta`/`WriteCounters`/
// `ClearCounters`) -- `WriteMetaDir`/`WriteCountersDir` take a directory
// path, which this project's `os` package has no directory support for
// anyway (see its own tracker line), so they're not attempted.
package coverage

import (
	"errors"
	"io"
)

var errNotSupported = errors.New("runtime/coverage: coverage instrumentation not supported on this target")

func WriteMeta(w io.Writer) error {
	return errNotSupported
}

func WriteCounters(w io.Writer) error {
	return errNotSupported
}

func ClearCounters() error {
	return errNotSupported
}
