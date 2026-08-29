// Tiny subset of go/format: Source(src) parses then re-prints, the same
// idea as gofmt, bounded by exactly what go/parser and go/printer
// support (see their own package comments) -- not a general Go source
// formatter.
package format

import (
	"go/parser"
	"go/printer"
)

func Source(src string) (string, error) {
	f, err := parser.ParseFile(src)
	if err != nil {
		return "", err
	}
	return printer.Sprint(f), nil
}
