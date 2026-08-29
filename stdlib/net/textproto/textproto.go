// Tiny subset of net/textproto: MIME-style header canonicalization, a
// MIMEHeader map type, and a line/header Reader over an io.Reader. No
// dot-reader/continued-line-folding beyond the simple leading-whitespace
// case, no Writer/PrintfLine, no Conn/Pipeline (those are net-layer, out of
// scope here).
package textproto

import (
	"errors"
	"io"
)

// CanonicalMIMEHeaderKey returns the canonical form of a header key, e.g.
// "content-type" -> "Content-Type" (first letter of each hyphen-separated
// word upper-cased, the rest lower-cased).
func CanonicalMIMEHeaderKey(s string) string {
	b := []byte(s)
	upper := true
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 45 {
			upper = true
			continue
		}
		if upper {
			if c >= 97 && c <= 122 {
				b[i] = c - 32
			}
			upper = false
		} else {
			if c >= 65 && c <= 90 {
				b[i] = c + 32
			}
		}
	}
	return string(b)
}

// MIMEHeader is a plain map[string][]string, the same shape as real Go's --
// but since wasigoc only supports methods on struct receivers (not on a
// defined map/slice alias), its Get/Set/Add/Del/Values are free functions
// here instead of methods. Maps are reference types, so mutation through
// these functions is visible to the caller exactly as a method would be.
type MIMEHeader map[string][]string

func HeaderGet(h MIMEHeader, key string) string {
	vs := h[CanonicalMIMEHeaderKey(key)]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

func HeaderSet(h MIMEHeader, key string, value string) {
	h[CanonicalMIMEHeaderKey(key)] = []string{value}
}

func HeaderAdd(h MIMEHeader, key string, value string) {
	k := CanonicalMIMEHeaderKey(key)
	h[k] = append(h[k], value)
}

func HeaderDel(h MIMEHeader, key string) {
	delete(h, CanonicalMIMEHeaderKey(key))
}

func HeaderValues(h MIMEHeader, key string) []string {
	return h[CanonicalMIMEHeaderKey(key)]
}

type Reader struct {
	r    io.Reader
	buf  []byte
	done bool
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) fill() {
	tmp := make([]byte, 4096)
	n, err := r.r.Read(tmp)
	if n > 0 {
		r.buf = append(r.buf, tmp[0:n]...)
	}
	if err != nil {
		r.done = true
	}
}

func indexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// ReadLine reads a single line, stripping the trailing CRLF or LF. Returns
// io.EOF only once no more data and no partial line remain.
func (r *Reader) ReadLine() (string, error) {
	for {
		i := indexByte(r.buf, 10)
		if i >= 0 {
			line := r.buf[0:i]
			if len(line) > 0 && line[len(line)-1] == 13 {
				line = line[0 : len(line)-1]
			}
			s := string(line)
			r.buf = r.buf[i+1:]
			return s, nil
		}
		if r.done {
			if len(r.buf) > 0 {
				line := r.buf
				if len(line) > 0 && line[len(line)-1] == 13 {
					line = line[0 : len(line)-1]
				}
				s := string(line)
				r.buf = []byte{}
				return s, nil
			}
			return "", io.EOF
		}
		r.fill()
	}
}

func isSpaceByte(c byte) bool {
	return c == 32 || c == 9
}

func trimLeadingSpace(s string) string {
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	return s[i:]
}

func trimTrailingSpace(s string) string {
	i := len(s)
	for i > 0 && isSpaceByte(s[i-1]) {
		i--
	}
	return s[0:i]
}

// ReadMIMEHeader reads header lines ("Key: Value\r\n") up to and including
// the blank line that terminates them, folding a continuation line (one
// starting with a space or tab) into the previous header's value.
func (r *Reader) ReadMIMEHeader() (MIMEHeader, error) {
	h := MIMEHeader{}
	lastKey := ""
	for {
		line, err := r.ReadLine()
		if err != nil {
			if len(h) > 0 {
				return h, nil
			}
			return h, err
		}
		if line == "" {
			return h, nil
		}
		if isSpaceByte(line[0]) && lastKey != "" {
			h[lastKey][len(h[lastKey])-1] = h[lastKey][len(h[lastKey])-1] + " " + trimLeadingSpace(line)
			continue
		}
		i := indexByte([]byte(line), 58)
		if i < 0 {
			return h, errors.New("textproto: malformed MIME header line: " + line)
		}
		key := CanonicalMIMEHeaderKey(trimTrailingSpace(line[0:i]))
		value := trimLeadingSpace(trimTrailingSpace(line[i+1:]))
		h[key] = append(h[key], value)
		lastKey = key
	}
}
