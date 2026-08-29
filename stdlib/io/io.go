// Tiny subset of io. Writer.Write matches Go's (n int, err error).
package io

import "errors"

var EOF = errors.New("EOF")
var ErrUnexpectedEOF = errors.New("unexpected EOF")

type Writer interface {
	Write(p []byte) (n int, err error)
}

type Reader interface {
	Read(p []byte) (n int, err error)
}

type Closer interface {
	Close() error
}

type StringWriter interface {
	WriteString(s string) (n int, err error)
}

func WriteString(w Writer, s string) (int, error) {
	sw, ok := w.(StringWriter)
	if ok {
		n, err := sw.WriteString(s)
		return n, err
	}
	n, err := w.Write([]byte(s))
	return n, err
}

func ReadAll(r Reader) ([]byte, error) {
	var out []byte
	buf := make([]byte, 512)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			out = append(out, buf[0:n]...)
		}
		if err != nil {
			if err == EOF {
				return out, nil
			}
			return out, err
		}
		if n == 0 {
			return out, nil
		}
	}
}

func Copy(dst Writer, src Reader) (int64, error) {
	var total int64
	buf := make([]byte, 512)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[0:n])
			total = total + int64(wn)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err == EOF {
				return total, nil
			}
			return total, err
		}
		if n == 0 {
			return total, nil
		}
	}
}
