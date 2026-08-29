// Tiny subset of Go's bytes package.
package bytes

import "errors"

func Equal(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func IndexByte(s []byte, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func Contains(s []byte, sub []byte) bool {
	return Index(s, sub) >= 0
}

func Index(s []byte, sep []byte) int {
	n := len(s)
	m := len(sep)
	if m == 0 {
		return 0
	}
	if m > n {
		return -1
	}
	lim := n - m
	for i := 0; i <= lim; i++ {
		ok := true
		for j := 0; j < m; j++ {
			if s[i+j] != sep[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func Repeat(b []byte, count int) []byte {
	var out []byte
	for i := 0; i < count; i++ {
		for j := 0; j < len(b); j++ {
			out = append(out, b[j])
		}
	}
	return out
}

type Buffer struct {
	buf []byte
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *Buffer) WriteString(s string) (int, error) {
	b.buf = append(b.buf, s...)
	return len(s), nil
}

func (b *Buffer) WriteByte(c byte) error {
	b.buf = append(b.buf, c)
	return nil
}

func (b *Buffer) String() string {
	return string(b.buf)
}

func (b *Buffer) Bytes() []byte {
	return b.buf
}

func (b *Buffer) Len() int {
	return len(b.buf)
}

func (b *Buffer) Reset() {
	b.buf = []byte{}
}

func (b *Buffer) Read(p []byte) (int, error) {
	if len(b.buf) == 0 && len(p) > 0 {
		return 0, errors.New("EOF")
	}
	n := copy(p, b.buf)
	b.buf = b.buf[n:]
	return n, nil
}

func isSpaceByte(c byte) bool {
	return c == 32 || c == 9 || c == 10 || c == 11 || c == 12 || c == 13
}

func TrimLeft(s []byte, cutset string) []byte {
	i := 0
	for i < len(s) && Contains([]byte(cutset), s[i:i+1]) {
		i++
	}
	return s[i:]
}

func TrimRight(s []byte, cutset string) []byte {
	n := len(s)
	for n > 0 && Contains([]byte(cutset), s[n-1:n]) {
		n--
	}
	return s[:n]
}

func Trim(s []byte, cutset string) []byte {
	return TrimLeft(TrimRight(s, cutset), cutset)
}

func TrimSpace(s []byte) []byte {
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	n := len(s)
	for n > i && isSpaceByte(s[n-1]) {
		n--
	}
	return s[i:n]
}

func Clone(s []byte) []byte {
	if s == nil {
		return nil
	}
	out := make([]byte, len(s))
	copy(out, s)
	return out
}

func Cut(s []byte, sep []byte) ([]byte, []byte, bool) {
	i := Index(s, sep)
	if i < 0 {
		return s, nil, false
	}
	return s[:i], s[i+len(sep):], true
}

func Replace(s []byte, old []byte, new []byte, n int) []byte {
	if n == 0 || len(old) == 0 {
		return s
	}
	var out []byte
	start := 0
	done := 0
	for {
		if n > 0 && done >= n {
			out = append(out, s[start:]...)
			return out
		}
		j := Index(s[start:], old)
		if j < 0 {
			out = append(out, s[start:]...)
			return out
		}
		out = append(out, s[start:start+j]...)
		out = append(out, new...)
		start = start + j + len(old)
		done++
	}
}

func ReplaceAll(s []byte, old []byte, new []byte) []byte {
	return Replace(s, old, new, -1)
}

func Split(s []byte, sep []byte) [][]byte {
	var out [][]byte
	if len(sep) == 0 {
		for i := 0; i < len(s); i++ {
			out = append(out, s[i:i+1])
		}
		return out
	}
	start := 0
	for {
		j := Index(s[start:], sep)
		if j < 0 {
			out = append(out, s[start:])
			return out
		}
		out = append(out, s[start:start+j])
		start = start + j + len(sep)
	}
}

func Join(s [][]byte, sep []byte) []byte {
	if len(s) == 0 {
		return []byte{}
	}
	out := Clone(s[0])
	for i := 1; i < len(s); i++ {
		out = append(out, sep...)
		out = append(out, s[i]...)
	}
	return out
}

func ToUpper(s []byte) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		ci := int(s[i])
		if ci >= 97 && ci <= 122 {
			ci = ci - 32
		}
		b = append(b, byte(ci))
	}
	return b
}

func ToLower(s []byte) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		ci := int(s[i])
		if ci >= 65 && ci <= 90 {
			ci = ci + 32
		}
		b = append(b, byte(ci))
	}
	return b
}

func HasPrefix(s []byte, prefix []byte) bool {
	if len(prefix) > len(s) {
		return false
	}
	return Equal(s[0:len(prefix)], prefix)
}

func HasSuffix(s []byte, suffix []byte) bool {
	if len(suffix) > len(s) {
		return false
	}
	return Equal(s[len(s)-len(suffix):], suffix)
}

// Reader is a read-only io.Reader over a []byte, position tracked with a
// plain int (real Go uses int64 -- see README, this compiler's int already
// is 64-bit).
type Reader struct {
	s []byte
	i int
}

func NewReader(b []byte) *Reader {
	return &Reader{s: b}
}

func (r *Reader) Len() int {
	n := len(r.s) - r.i
	if n < 0 {
		return 0
	}
	return n
}

func (r *Reader) Size() int {
	return len(r.s)
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		if len(p) == 0 {
			return 0, nil
		}
		return 0, errors.New("EOF")
	}
	n := copy(p, r.s[r.i:])
	r.i = r.i + n
	return n, nil
}

func (r *Reader) ReadByte() (byte, error) {
	if r.i >= len(r.s) {
		return 0, errors.New("EOF")
	}
	b := r.s[r.i]
	r.i++
	return b, nil
}
