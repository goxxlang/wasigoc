// Bounded compress/zlib: RFC 1950's thin wrapper around compress/flate --
// a 2-byte header (CMF/FLG, mod-31 checksummed) then a raw DEFLATE stream
// then a big-endian Adler-32 trailer over the UNCOMPRESSED bytes. No
// preset-dictionary support (`FDICT`), no `NewWriterLevel`/`NewReaderDict`.
// `Reader` can read the 4-byte trailer immediately after building the
// underlying `flate.Reader`, not lazily at EOF, because `flate.Reader`
// already decodes its entire body eagerly at construction (see flate's own
// "slurp all" bound) -- by the time `flate.NewReader` returns, the
// underlying `io.Reader`'s cursor already sits exactly at the trailer's
// first byte, with any leftover sub-byte padding bits from the last
// deflate block correctly discarded (flate's bitReader never reads a byte
// from the underlying stream until it actually needs another bit from it).
package zlib

import (
	"compress/flate"
	"errors"
	"hash/adler32"
	"io"
)

var ErrHeader = errors.New("zlib: invalid header")
var ErrChecksum = errors.New("zlib: checksum mismatch")
var ErrDictionary = errors.New("zlib: preset dictionary not supported")

func readN(r io.Reader, n int) ([]byte, error) {
	out := make([]byte, n)
	got := 0
	for got < n {
		buf := make([]byte, 1)
		k, err := r.Read(buf)
		if k == 0 {
			return nil, err
		}
		out[got] = buf[0]
		got = got + 1
	}
	return out, nil
}

func header() []byte {
	cmf := byte(120)
	flg := 0
	rem := (int(cmf)*256 + flg) % 31
	fcheck := 0
	if rem != 0 {
		fcheck = 31 - rem
	}
	return []byte{cmf, byte(flg + fcheck)}
}

type Writer struct {
	w   io.Writer
	buf []byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (zw *Writer) Write(p []byte) (int, error) {
	zw.buf = append(zw.buf, p...)
	return len(p), nil
}

func (zw *Writer) Close() error {
	if _, err := zw.w.Write(header()); err != nil {
		return err
	}
	fw, _ := flate.NewWriter(zw.w, flate.DefaultCompression)
	fw.Write(zw.buf)
	if err := fw.Close(); err != nil {
		return err
	}
	sum := adler32.Checksum(zw.buf)
	trailer := []byte{byte(sum >> 24), byte(sum >> 16), byte(sum >> 8), byte(sum)}
	_, err := zw.w.Write(trailer)
	return err
}

type Reader struct {
	fr       *flate.Reader
	digest   *adler32.Digest
	expected uint32
	haveExp  bool
	checked  bool
}

func NewReader(r io.Reader) (*Reader, error) {
	hdr, err := readN(r, 2)
	if err != nil {
		return nil, err
	}
	cmf := hdr[0]
	flg := hdr[1]
	if (int(cmf)*256+int(flg))%31 != 0 {
		return nil, ErrHeader
	}
	if cmf&15 != 8 {
		return nil, ErrHeader
	}
	if flg&32 != 0 {
		return nil, ErrDictionary
	}
	fr := flate.NewReader(r)
	zr := &Reader{fr: fr, digest: adler32.New()}
	trailer, terr := readN(r, 4)
	if terr == nil {
		zr.expected = (uint32(trailer[0]) << 24) | (uint32(trailer[1]) << 16) | (uint32(trailer[2]) << 8) | uint32(trailer[3])
		zr.haveExp = true
	}
	return zr, nil
}

func (zr *Reader) Read(p []byte) (int, error) {
	n, err := zr.fr.Read(p)
	if n > 0 {
		zr.digest.Write(p[:n])
	}
	if err == io.EOF && !zr.checked {
		zr.checked = true
		if zr.haveExp && zr.digest.Sum32() != zr.expected {
			return n, ErrChecksum
		}
	}
	return n, err
}

func (zr *Reader) Close() error {
	return zr.fr.Close()
}
