// Deprecated in real Go too -- a thin shim over io/os. Only the direct
// pass-throughs that map onto something this project's os/io already
// support: ReadAll, ReadFile, WriteFile. Not implemented: ReadDir,
// TempFile, TempDir (this project's os has no directories or temp-file
// support at all -- see os/exec's stub note for the same "not a todo,
// a deliberate boundary" reasoning) and NopCloser (needs io.ReadCloser,
// not added here yet).
package ioutil

import (
	"io"
	"os"
)

func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

func ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func WriteFile(filename string, data []byte, perm int) error {
	return os.WriteFile(filename, data, perm)
}
