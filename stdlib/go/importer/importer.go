// Package importer is a stub: real Go's importer walks the filesystem
// for export data / source, and this project's os has no directory
// listing. Default().Import returns a clear "not supported" error so
// source that imports go/importer still compiles. Not go/types.Package
// -- this project's go/types has no package-level checker to hang one
// off.
package importer

import "errors"

var ErrNotSupported = errors.New(
	"go/importer: no package discovery (no directory listing)")

type Package struct {
	Path string
	Name string
}

type Importer struct{}

func Default() *Importer { return &Importer{} }

func (imp *Importer) Import(path string) (*Package, error) {
	return nil, ErrNotSupported
}
