// Package embed is a stub: real Go's embed.FS is filled by the compiler
// from `//go:embed` directives, and wasigoc does not yet generate that
// data (see README tracker: "compiler-facing; treat as generated later").
// Open/ReadFile return a clear "no files" error so source that imports
// embed still compiles. Implements io/fs.FS.
package embed

import (
	"errors"
	"io/fs"
)

var ErrNotExist = errors.New("embed: no files (wasigoc does not generate embed data)")

type FS struct{}

func (f FS) Open(name string) (fs.File, error) {
	return nil, ErrNotExist
}

func (f FS) ReadFile(name string) ([]byte, error) {
	return nil, ErrNotExist
}
