// Bounded go/build: Context{GOOS,GOARCH} and helpers that do not need
// a filesystem (IsLocalImport, Default). Import/ImportDir return a
// clear "no directory listing" error -- this project's os has no
// ReadDir (see its tracker line). ParsePackageName uses go/parser to
// pull the package clause out of source text the caller already has.
package build

import (
	"errors"
	"go/parser"
	"strings"
)

var ErrNoFS = errors.New("go/build: no directory listing on wasm32-wasip1")

type Context struct {
	GOOS   string
	GOARCH string
}

func Default() Context {
	return Context{GOOS: "wasip1", GOARCH: "wasm"}
}

type Package struct {
	Name       string
	ImportPath string
	Dir        string
	GoFiles    []string
	Imports    []string
}

func Import(path string, srcDir string) (*Package, error) {
	return nil, ErrNoFS
}

func ImportDir(dir string) (*Package, error) {
	return nil, ErrNoFS
}

func IsLocalImport(path string) bool {
	return strings.HasPrefix(path, ".")
}

func ParsePackageName(src string) (string, error) {
	f, err := parser.ParseFile(src)
	if err != nil {
		return "", err
	}
	return f.Name, nil
}
