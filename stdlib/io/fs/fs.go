// io/fs: the FS/File/FileInfo interfaces plus ValidPath and the generic
// ReadFile helper. Bounded like net.Pipe/hash.Hash: no os-backed FS at all
// (this project's os has no directory listing -- see os's own tracker line),
// so this package only ever operates on a caller-provided FS implementation
// (an in-memory map-backed FS, same idea as embed.FS would be). No ReadDir/
// Glob/WalkDir (need a ReadDirFS a caller could implement, but no directory-
// bearing FS exists here to exercise it against, and no Sub (wraps another
// FS, adds nothing new to verify). FileMode is a plain uint32 (matching real
// Go's own underlying type) -- no methods (String/Perm/IsDir), since methods
// here need a real struct receiver, same bound as time.Duration -- IsDir
// lives on FileInfo instead, matching real Go.
package fs

import (
	"errors"
	"unicode/utf8"
)

type FileMode uint32

const ModeDir FileMode = 1 << 31

type FileInfo interface {
	Name() string
	Size() int64
	Mode() FileMode
	IsDir() bool
}

type File interface {
	Stat() (FileInfo, error)
	Read(p []byte) (int, error)
	Close() error
}

type FS interface {
	Open(name string) (File, error)
}

var ErrNotExist = errors.New("file does not exist")
var ErrExist = errors.New("file already exists")
var ErrInvalid = errors.New("invalid argument")
var ErrPermission = errors.New("permission denied")
var ErrClosed = errors.New("file already closed")

// ValidPath matches real Go's io/fs.ValidPath doc contract: UTF-8, unrooted,
// slash-separated, no "."/".."/empty elements, no leading/trailing slash,
// except the special case "." alone names the root.
func ValidPath(name string) bool {
	if !utf8.ValidString(name) {
		return false
	}
	if name == "." {
		return true
	}
	if len(name) == 0 {
		return false
	}
	if name[0] == '/' || name[len(name)-1] == '/' {
		return false
	}
	start := 0
	for i := 0; i <= len(name); i++ {
		if i == len(name) || name[i] == '/' {
			elem := name[start:i]
			if elem == "" || elem == "." || elem == ".." {
				return false
			}
			start = i + 1
		}
	}
	return true
}

// ReadFile reads the named file from fsys via Open/Read/Close -- the
// FS-generic equivalent of os.ReadFile.
func ReadFile(fsys FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	var out []byte
	buf := make([]byte, 512)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}
	f.Close()
	return out, nil
}

// Stat opens the named file via fsys and returns its FileInfo.
func Stat(fsys FS, name string) (FileInfo, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	fi, serr := f.Stat()
	f.Close()
	return fi, serr
}
