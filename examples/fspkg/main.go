package main

import (
	"fmt"
	"io"
	"io/fs"
)

type memFile struct {
	name string
	data []byte
	pos  int
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return &memInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f *memFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *memFile) Close() error {
	return nil
}

type memInfo struct {
	name string
	size int64
}

func (i *memInfo) Name() string      { return i.name }
func (i *memInfo) Size() int64       { return i.size }
func (i *memInfo) Mode() fs.FileMode { return 0 }
func (i *memInfo) IsDir() bool       { return false }

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
	m := &memFS{files: map[string][]byte{"a.txt": []byte("hello fs")}}

	data, err := fs.ReadFile(m, "a.txt")
	fmt.Println(string(data))
	fmt.Println(err == nil)

	fi, ferr := fs.Stat(m, "a.txt")
	fmt.Println(ferr == nil)
	fmt.Println(fi.Name())
	fmt.Println(fi.Size())
	fmt.Println(fi.IsDir())

	_, missErr := fs.ReadFile(m, "missing.txt")
	fmt.Println(missErr != nil)
	fmt.Println(missErr == fs.ErrNotExist)

	fmt.Println(fs.ValidPath("a/b/c"))
	fmt.Println(fs.ValidPath("."))
	fmt.Println(fs.ValidPath("/a"))
	fmt.Println(fs.ValidPath("a/"))
	fmt.Println(fs.ValidPath("a//b"))
	fmt.Println(fs.ValidPath("a/../b"))
	fmt.Println(fs.ValidPath(""))
}
