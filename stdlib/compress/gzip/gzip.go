// Bounded compress/gzip: RFC 1952's container around compress/flate -- a
// 10-byte fixed header (magic 0x1f 0x8b, CM=8, FLG, MTIME, XFL, OS) then a
// raw DEFLATE stream then an 8-byte little-endian CRC-32 + ISIZE trailer.
// `Writer` always emits the minimal header (FLG=0: no FNAME/FEXTRA/
// FCOMMENT/FHCRC, MTIME=0, OS=255 "unknown") -- no `Header` metadata
// support (`Name`/`Comment`/`ModTime`/`Extra`), same shape as flate's
// `level`-ignored bound. `Reader` DOES parse and skip FEXTRA/FNAME/
// FCOMMENT/FHCRC if present, so it can read a real gzip file a real tool
// produced with any of those set, not just this package's own minimal
// output. Same eager-decode trailer-positioning trick as compress/zlib
// (see its own comment): by the time `flate.NewReader` returns, the
// underlying stream already sits at the trailer's first byte.
package gzip

import (
	"compress/flate"
	"errors"
	"hash/crc32"
	"io"
)

var ErrHeader = errors.New("gzip: invalid header")
var ErrChecksum = errors.New("gzip: checksum mismatch")

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

func skipCString(r io.Reader) error {
	for {
		b, err := readN(r, 1)
		if err != nil {
			return err
		}
		if b[0] == 0 {
			return nil
		}
	}
}

type Writer struct {
	w   io.Writer
	buf []byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (gw *Writer) Write(p []byte) (int, error) {
	gw.buf = append(gw.buf, p...)
	return len(p), nil
}

func (gw *Writer) Close() error {
	hdr := []byte{31, 139, 8, 0, 0, 0, 0, 0, 0, 255}
	if _, err := gw.w.Write(hdr); err != nil {
		return err
	}
	fw, _ := flate.NewWriter(gw.w, flate.DefaultCompression)
	fw.Write(gw.buf)
	if err := fw.Close(); err != nil {
		return err
	}
	crc := crc32.ChecksumIEEE(gw.buf)
	size := uint32(len(gw.buf))
	trailer := []byte{
		byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24),
		byte(size), byte(size >> 8), byte(size >> 16), byte(size >> 24),
	}
	_, err := gw.w.Write(trailer)
	return err
}

type Reader struct {
	fr          *flate.Reader
	digest      *crc32.Digest
	expectedCRC uint32
	haveTrailer bool
	checked     bool
}

func NewReader(r io.Reader) (*Reader, error) {
	hdr, err := readN(r, 10)
	if err != nil {
		return nil, err
	}
	if hdr[0] != 31 || hdr[1] != 139 {
		return nil, ErrHeader
	}
	if hdr[2] != 8 {
		return nil, ErrHeader
	}
	flg := hdr[3]
	if flg&4 != 0 {
		xlenB, xerr := readN(r, 2)
		if xerr != nil {
			return nil, xerr
		}
		xlen := int(xlenB[0]) | (int(xlenB[1]) << 8)
		if _, xerr2 := readN(r, xlen); xerr2 != nil {
			return nil, xerr2
		}
	}
	if flg&8 != 0 {
		if serr := skipCString(r); serr != nil {
			return nil, serr
		}
	}
	if flg&16 != 0 {
		if serr := skipCString(r); serr != nil {
			return nil, serr
		}
	}
	if flg&2 != 0 {
		if _, herr := readN(r, 2); herr != nil {
			return nil, herr
		}
	}
	fr := flate.NewReader(r)
	gr := &Reader{fr: fr, digest: crc32.NewIEEE()}
	trailer, terr := readN(r, 8)
	if terr == nil {
		gr.expectedCRC = uint32(trailer[0]) | (uint32(trailer[1]) << 8) | (uint32(trailer[2]) << 16) | (uint32(trailer[3]) << 24)
		gr.haveTrailer = true
	}
	return gr, nil
}

func (gr *Reader) Read(p []byte) (int, error) {
	n, err := gr.fr.Read(p)
	if n > 0 {
		gr.digest.Write(p[:n])
	}
	if err == io.EOF && !gr.checked {
		gr.checked = true
		if gr.haveTrailer && gr.digest.Sum32() != gr.expectedCRC {
			return n, ErrChecksum
		}
	}
	return n, err
}

func (gr *Reader) Close() error {
	return gr.fr.Close()
}
