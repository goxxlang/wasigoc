// Tiny subset of context: cooperative cancellation over wasigo::Chan
// (real WASI is single-threaded -- see README's Rosetta table -- so
// there's no OS-level cancellation to hook into either way). Context is a
// concrete struct here, not real Go's interface -- this compiler doesn't
// need multiple implementations, and callers just naming the type
// (`func f(ctx context.Context)`) work the same either way. Value keys are
// `string`, not real Go's `any`: wasigo::Any has no equality (see
// stdlib/sync's Map, which is unsupported for the same reason), so an
// `any`-keyed lookup couldn't compile a `==` at all.
package context

import "errors"

var Canceled = errors.New("context canceled")
var DeadlineExceeded = errors.New("context deadline exceeded")

type Context struct {
	done   chan bool
	err    error
	parent *Context
	key    string
	val    any
}

func Background() *Context {
	return &Context{}
}

func TODO() *Context {
	return &Context{}
}

func (c *Context) Done() chan bool {
	if c.done == nil {
		c.done = make(chan bool)
	}
	return c.done
}

func (c *Context) Err() error {
	if c.err != nil {
		return c.err
	}
	if c.parent != nil {
		return c.parent.Err()
	}
	return nil
}

func (c *Context) Value(key string) any {
	cur := c
	for cur != nil {
		if cur.key == key {
			return cur.val
		}
		cur = cur.parent
	}
	return nil
}

type CancelFunc func()

// cancel is a real method, not a `cancel := func() { ... }` literal
// returned alongside child, so that WithCancel can return the *bound
// method value* `child.cancel` as the CancelFunc: a method value's
// receiver is captured by value (a plain pointer copy -- see
// EmitBindMethodValue in cpp_generator.cc), safe to outlive the function
// that created it. A func literal capturing a local by reference (the
// obvious way to write this) is NOT safe to return that way: the C++
// closure this compiler emits for an ordinary (non-goroutine) func literal
// captures `[&]`, so it would hold a reference to WithCancel's *own stack
// frame* -- dangling the moment WithCancel returns, a real bug (confirmed
// by a crash) rather than a theoretical one. See README's stdlib tracker.
func (c *Context) cancel() {
	if c.err == nil {
		c.err = Canceled
		close(c.done)
	}
}

func WithCancel(parent *Context) (*Context, CancelFunc) {
	child := &Context{parent: parent}
	child.done = make(chan bool)
	return child, child.cancel
}

func WithValue(parent *Context, key string, val any) *Context {
	return &Context{parent: parent, key: key, val: val}
}
