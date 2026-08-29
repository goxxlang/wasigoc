// Tiny subset of bufio: line-oriented Scanner (the everyday case -- reading
// stdin/a file a line at a time) and a small buffered Writer. No
// Scanner.Split/custom SplitFunc (only the default ScanLines behavior), no
// bufio.Reader.
package bufio

import (
	"bytes"
	"errors"
	"io"
)

const bufSize = 4096

type Scanner struct {
	r    io.Reader
	buf  []byte
	text string
	err  error
	done bool
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{r: r}
}

func (s *Scanner) fill() {
	tmp := make([]byte, bufSize)
	n, err := s.r.Read(tmp)
	if n > 0 {
		s.buf = append(s.buf, tmp[0:n]...)
	}
	if err != nil {
		if !errors.Is(err, errors.New("EOF")) {
			s.err = err
		}
		s.done = true
	}
}

func (s *Scanner) takeLine(i int) string {
	line := s.buf[0:i]
	if len(line) > 0 && line[len(line)-1] == 13 {
		line = line[0 : len(line)-1]
	}
	s.buf = s.buf[i+1:]
	return string(line)
}

func (s *Scanner) Scan() bool {
	for {
		i := bytes.IndexByte(s.buf, 10)
		if i >= 0 {
			s.text = s.takeLine(i)
			return true
		}
		if s.done {
			if len(s.buf) > 0 {
				line := s.buf
				if len(line) > 0 && line[len(line)-1] == 13 {
					line = line[0 : len(line)-1]
				}
				s.text = string(line)
				s.buf = []byte{}
				return true
			}
			return false
		}
		s.fill()
	}
}

func (s *Scanner) Text() string {
	return s.text
}

func (s *Scanner) Bytes() []byte {
	return []byte(s.text)
}

func (s *Scanner) Err() error {
	return s.err
}

// Writer is a small buffered io.Writer -- Flush must be called (or deferred)
// to actually send buffered bytes.
type Writer struct {
	w   io.Writer
	buf []byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (b *Writer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) >= bufSize {
		err := b.Flush()
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (b *Writer) WriteString(s string) (int, error) {
	n, err := b.Write([]byte(s))
	return n, err
}

func (b *Writer) WriteByte(c byte) error {
	b.buf = append(b.buf, c)
	return nil
}

func (b *Writer) Flush() error {
	if len(b.buf) == 0 {
		return nil
	}
	_, err := b.w.Write(b.buf)
	b.buf = []byte{}
	return err
}
