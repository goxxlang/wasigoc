// Bounded runtime/trace: same "correct terminal shape" reasoning as
// runtime/pprof -- this target has no execution tracer, so `Start`
// returns a clear "not supported" error and `Stop` is a real no-op
// (matching the shape a real `Stop` call after a failed `Start` should
// have anyway).
package trace

import (
	"errors"
	"io"
)

var errNotSupported = errors.New("runtime/trace: tracing not supported on this target")

func Start(w io.Writer) error {
	return errNotSupported
}

func Stop() {}
