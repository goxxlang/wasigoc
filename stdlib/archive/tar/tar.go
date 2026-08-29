// USTAR format archives, bounded to the everyday case: regular files
// only, `Header{Name, Mode, Size, Typeflag}` (no uid/gid/mtime/uname/
// gname/devmajor/devminor/prefix -- all written as zero/empty, which
// is a valid, real USTAR header, just not a faithful metadata copy).
// Verified round-trip against Python's own `tarfile` module for real
// interop, not just self-consistency (see examples/tarpkg).
package tar

import (
	"errors"
	"io"
	"strconv"
	"strings"
)

const blockSize = 512

type Header struct {
	Name     string
	Mode     int64
	Size     int64
	Typeflag byte
}

func cstring(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n = n + 1
	}
	return string(b[0:n])
}

func parseOctal(b []byte) int64 {
	s := strings.TrimSpace(cstring(b))
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 8, 64)
	return v
}

func putString(buf []byte, off int, s string, width int) {
	b := []byte(s)
	n := len(b)
	if n > width {
		n = width
	}
	for i := 0; i < n; i++ {
		buf[off+i] = b[i]
	}
}

// putOctal writes v as zero-padded octal digits filling width-1 bytes,
// followed by a NUL, into buf[off:off+width].
func putOctal(buf []byte, off int, v int64, width int) {
	s := strconv.FormatInt(v, 8)
	for len(s) < width-1 {
		s = "0" + s
	}
	putString(buf, off, s, width-1)
	buf[off+width-1] = 0
}

func readFull(r io.Reader, buf []byte) error {
	got := 0
	for got < len(buf) {
		n, err := r.Read(buf[got:])
		got = got + n
		if err != nil {
			if got == len(buf) {
				return nil
			}
			return err
		}
		if n == 0 {
			return errors.New("archive/tar: unexpected EOF")
		}
	}
	return nil
}

type Writer struct {
	w       io.Writer
	written int64
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (tw *Writer) finishEntry() error {
	pad := (blockSize - (tw.written % blockSize)) % blockSize
	if pad > 0 {
		_, err := tw.w.Write(make([]byte, pad))
		if err != nil {
			return err
		}
	}
	tw.written = 0
	return nil
}

func (tw *Writer) WriteHeader(hdr *Header) error {
	err := tw.finishEntry()
	if err != nil {
		return err
	}
	buf := make([]byte, blockSize)
	putString(buf, 0, hdr.Name, 100)
	putOctal(buf, 100, hdr.Mode, 8)
	putOctal(buf, 108, 0, 8)
	putOctal(buf, 116, 0, 8)
	putOctal(buf, 124, hdr.Size, 12)
	putOctal(buf, 136, 0, 12)
	for i := 148; i < 156; i++ {
		buf[i] = byte(32)
	}
	tf := hdr.Typeflag
	if tf == 0 {
		tf = byte('0')
	}
	buf[156] = tf
	putString(buf, 257, "ustar", 6)
	buf[263] = byte('0')
	buf[264] = byte('0')

	var sum int64 = 0
	for i := 0; i < blockSize; i++ {
		sum = sum + int64(buf[i])
	}
	chkDigits := strconv.FormatInt(sum, 8)
	for len(chkDigits) < 6 {
		chkDigits = "0" + chkDigits
	}
	putString(buf, 148, chkDigits, 6)
	buf[154] = 0
	buf[155] = byte(32)

	_, err = tw.w.Write(buf)
	return err
}

func (tw *Writer) Write(b []byte) (int, error) {
	n, err := tw.w.Write(b)
	tw.written = tw.written + int64(n)
	return n, err
}

func (tw *Writer) Close() error {
	err := tw.finishEntry()
	if err != nil {
		return err
	}
	zeros := make([]byte, blockSize)
	_, err = tw.w.Write(zeros)
	if err != nil {
		return err
	}
	_, err = tw.w.Write(zeros)
	return err
}

type Reader struct {
	r       io.Reader
	remain  int64
	padLeft int64
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (tr *Reader) Next() (*Header, error) {
	if tr.remain > 0 || tr.padLeft > 0 {
		skip := make([]byte, tr.remain+tr.padLeft)
		err := readFull(tr.r, skip)
		if err != nil {
			return nil, err
		}
		tr.remain = 0
		tr.padLeft = 0
	}
	buf := make([]byte, blockSize)
	err := readFull(tr.r, buf)
	if err != nil {
		return nil, err
	}
	allZero := true
	for i := 0; i < blockSize; i++ {
		if buf[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil, io.EOF
	}
	name := cstring(buf[0:100])
	mode := parseOctal(buf[100:108])
	size := parseOctal(buf[124:136])
	typeflag := buf[156]
	tr.remain = size
	tr.padLeft = (blockSize - (size % blockSize)) % blockSize
	return &Header{Name: name, Mode: mode, Size: size, Typeflag: typeflag}, nil
}

func (tr *Reader) Read(b []byte) (int, error) {
	if tr.remain <= 0 {
		return 0, io.EOF
	}
	toRead := b
	if int64(len(toRead)) > tr.remain {
		toRead = toRead[0:tr.remain]
	}
	n, err := tr.r.Read(toRead)
	tr.remain = tr.remain - int64(n)
	return n, err
}
