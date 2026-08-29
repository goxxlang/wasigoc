// Tiny subset of Go's strings package, compiled as a directory import.
package strings

import "errors"

func HasPrefix(s string, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[0:len(prefix)] == prefix
}

func HasSuffix(s string, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func Index(s string, substr string) int {
	n := len(s)
	m := len(substr)
	if m == 0 {
		return 0
	}
	if m > n {
		return -1
	}
	lim := n - m
	for i := 0; i <= lim; i++ {
		if s[i:i+m] == substr {
			return i
		}
	}
	return -1
}

func Contains(s string, substr string) bool {
	return Index(s, substr) >= 0
}

func Count(s string, substr string) int {
	if len(substr) == 0 {
		return len(s) + 1
	}
	n := 0
	i := 0
	for {
		j := Index(s[i:], substr)
		if j < 0 {
			return n
		}
		n++
		i = i + j + len(substr)
	}
}

func Repeat(s string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out = out + s
	}
	return out
}

func Join(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	out := elems[0]
	for i := 1; i < len(elems); i++ {
		out = out + sep + elems[i]
	}
	return out
}

func Split(s string, sep string) []string {
	var out []string
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

func Replace(s string, old string, new string, n int) string {
	if n == 0 {
		return s
	}
	if old == "" {
		return s
	}
	out := ""
	start := 0
	done := 0
	for {
		if n > 0 && done >= n {
			return out + s[start:]
		}
		j := Index(s[start:], old)
		if j < 0 {
			return out + s[start:]
		}
		out = out + s[start:start+j] + new
		start = start + j + len(old)
		done++
	}
}

func ToUpper(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		ci := int(s[i])
		if ci >= 97 && ci <= 122 {
			ci = ci - 32
		}
		b = append(b, byte(ci))
	}
	return string(b)
}

func ToLower(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		ci := int(s[i])
		if ci >= 65 && ci <= 90 {
			ci = ci + 32
		}
		b = append(b, byte(ci))
	}
	return string(b)
}

func TrimPrefix(s string, prefix string) string {
	if HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

func TrimSuffix(s string, suffix string) string {
	if HasSuffix(s, suffix) {
		return s[0 : len(s)-len(suffix)]
	}
	return s
}

type Builder struct {
	buf []byte
}

func (b *Builder) WriteString(s string) (int, error) {
	b.buf = append(b.buf, s...)
	return len(s), nil
}

func (b *Builder) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *Builder) String() string {
	return string(b.buf)
}

func (b *Builder) Len() int {
	return len(b.buf)
}

func (b *Builder) Reset() {
	b.buf = []byte{}
}

func isSpaceByte(c byte) bool {
	return c == 32 || c == 9 || c == 10 || c == 11 || c == 12 || c == 13
}

func TrimLeft(s string, cutset string) string {
	i := 0
	for i < len(s) && Contains(cutset, s[i:i+1]) {
		i++
	}
	return s[i:]
}

func TrimRight(s string, cutset string) string {
	n := len(s)
	for n > 0 && Contains(cutset, s[n-1:n]) {
		n--
	}
	return s[:n]
}

func Trim(s string, cutset string) string {
	return TrimLeft(TrimRight(s, cutset), cutset)
}

func TrimSpace(s string) string {
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

func TrimFunc(s string, f func(rune) bool) string {
	start := 0
	end := len(s)
	for start < end {
		r, size := DecodeRuneAt(s, start)
		if !f(r) {
			break
		}
		start = start + size
	}
	for end > start {
		i := end - 1
		for i > start && isUTF8Cont(s[i]) {
			i--
		}
		r, _ := DecodeRuneAt(s, i)
		if !f(r) {
			break
		}
		end = i
	}
	return s[start:end]
}

func isUTF8Cont(c byte) bool {
	return int(c)&192 == 128
}

func DecodeRuneAt(s string, i int) (rune, int) {
	for j, r := range s[i:] {
		if j == 0 {
			size := 1
			for k := i + 1; k < len(s) && isUTF8Cont(s[k]); k++ {
				size++
			}
			return r, size
		}
	}
	return 65533, 0
}

func Fields(s string) []string {
	var out []string
	i := 0
	n := len(s)
	for i < n {
		for i < n && isSpaceByte(s[i]) {
			i++
		}
		if i >= n {
			break
		}
		start := i
		for i < n && !isSpaceByte(s[i]) {
			i++
		}
		out = append(out, s[start:i])
	}
	return out
}

func Cut(s string, sep string) (string, string, bool) {
	i := Index(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func ReplaceAll(s string, old string, new string) string {
	return Replace(s, old, new, -1)
}

func Map(mapping func(rune) rune, s string) string {
	var b []byte
	for _, r := range s {
		mr := mapping(r)
		if mr < 0 {
			continue
		}
		b = append(b, string(mr)...)
	}
	return string(b)
}

func EqualFold(s string, t string) bool {
	if len(s) != len(t) {
		return false
	}
	return ToLower(s) == ToLower(t)
}

func LastIndex(s string, substr string) int {
	n := len(s)
	m := len(substr)
	if m == 0 {
		return n
	}
	for i := n - m; i >= 0; i-- {
		if s[i:i+m] == substr {
			return i
		}
	}
	return -1
}

func IndexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func ContainsAny(s string, chars string) bool {
	return IndexAny(s, chars) >= 0
}

func IndexAny(s string, chars string) int {
	for i := 0; i < len(s); i++ {
		if Contains(chars, s[i:i+1]) {
			return i
		}
	}
	return -1
}

func ContainsRune(s string, r rune) bool {
	for _, rr := range s {
		if rr == r {
			return true
		}
	}
	return false
}

func LastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// Reader is a read-only io.Reader over a string.
type Reader struct {
	s string
	i int
}

func NewReader(s string) *Reader {
	return &Reader{s: s}
}

func (r *Reader) Len() int {
	n := len(r.s) - r.i
	if n < 0 {
		return 0
	}
	return n
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
