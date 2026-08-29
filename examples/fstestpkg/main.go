package main

import (
	"fmt"
	"io/fs"
	"testing/fstest"
)

type memFile struct {
	name string
	data []byte
	pos  int
}

func (f *memFile) Stat() (fs.FileInfo, error) { return nil, nil }
func (f *memFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}
func (f *memFile) Close() error { return nil }

type memFS struct {
	files map[string][]byte
}

func (m *memFS) Open(name string) (fs.File, error) {
	data, ok := m.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFile{name: name, data: data}, nil
}

func main() {
	m := &memFS{files: map[string][]byte{"a.txt": []byte("hi")}}
	err := fstest.TestFS(m, "a.txt")
	fmt.Println(err == nil)

	err2 := fstest.TestFS(m, "a.txt", "missing.txt")
	fmt.Println(err2 != nil)
}
