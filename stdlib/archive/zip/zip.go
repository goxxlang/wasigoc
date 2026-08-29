// Bounded archive/zip: the ZIP file format's local file header + central
// directory + end-of-central-directory (EOCD) records, entries always
// DEFLATE-compressed via compress/flate (method 8) -- no Store method, no
// `CreateHeader`, no directories, no ZIP64 (fine for anything under 4GiB /
// 65535 entries, real Go's own original format's limits too). `Writer`
// buffers each entry's content until the NEXT `Create` or `Close` (same
// "buffer, compress once" bound as flate/zlib/gzip's Writer), matching
// real Go's actual `Create`/`Close` call shape even though the
// implementation isn't real streaming. `NewReader` takes the whole zip
// file as a `[]byte` rather than real Go's `io.ReaderAt` + size -- this
// project has no ReaderAt interface -- and scans backward from the end for
// the EOCD signature exactly like a real zip parser does (comments make
// its exact offset unknowable otherwise). `File.Open` verifies the
// entry's CRC-32 at EOF, same "checked, not just returned" bound as
// gzip.Reader/zlib.Reader.
package zip

import (
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"io"
)

var ErrFormat = errors.New("zip: not a valid zip file")
var ErrChecksum = errors.New("zip: checksum mismatch")
var ErrAlgorithm = errors.New("zip: unsupported compression method")

func appendU16(b []byte, v uint16) []byte {
	return append(b, byte(v), byte(v>>8))
}

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func readU16(b []byte, off int) int {
	return int(b[off]) | (int(b[off+1]) << 8)
}

func readU32(b []byte, off int) uint32 {
	return uint32(b[off]) | (uint32(b[off+1]) << 8) | (uint32(b[off+2]) << 16) | (uint32(b[off+3]) << 24)
}

type zipEntry struct {
	name       string
	buf        []byte
	crc        uint32
	compSize   uint32
	uncompSize uint32
	offset     uint32
}

type entryWriter struct {
	e *zipEntry
}

func (ew *entryWriter) Write(p []byte) (int, error) {
	ew.e.buf = append(ew.e.buf, p...)
	return len(p), nil
}

type Writer struct {
	w       io.Writer
	entries []*zipEntry
	current *zipEntry
	written int64
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (zw *Writer) Create(name string) (io.Writer, error) {
	if err := zw.finishCurrent(); err != nil {
		return nil, err
	}
	e := &zipEntry{name: name}
	zw.current = e
	zw.entries = append(zw.entries, e)
	return &entryWriter{e: e}, nil
}

func localHeader(e *zipEntry) []byte {
	var b []byte
	b = appendU32(b, 67324752)
	b = appendU16(b, 20)
	b = appendU16(b, 0)
	b = appendU16(b, 8)
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU32(b, e.crc)
	b = appendU32(b, e.compSize)
	b = appendU32(b, e.uncompSize)
	b = appendU16(b, uint16(len(e.name)))
	b = appendU16(b, 0)
	return b
}

func centralHeader(e *zipEntry) []byte {
	var b []byte
	b = appendU32(b, 33639248)
	b = appendU16(b, 20)
	b = appendU16(b, 20)
	b = appendU16(b, 0)
	b = appendU16(b, 8)
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU32(b, e.crc)
	b = appendU32(b, e.compSize)
	b = appendU32(b, e.uncompSize)
	b = appendU16(b, uint16(len(e.name)))
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, e.offset)
	return b
}

func (zw *Writer) finishCurrent() error {
	if zw.current == nil {
		return nil
	}
	e := zw.current
	e.crc = crc32.ChecksumIEEE(e.buf)
	e.uncompSize = uint32(len(e.buf))

	var cbuf bytes.Buffer
	fw, _ := flate.NewWriter(&cbuf, flate.DefaultCompression)
	fw.Write(e.buf)
	fw.Close()
	compData := cbuf.Bytes()
	e.compSize = uint32(len(compData))
	e.offset = uint32(zw.written)

	hdr := localHeader(e)
	if _, err := zw.w.Write(hdr); err != nil {
		return err
	}
	zw.written = zw.written + int64(len(hdr))
	if _, err := zw.w.Write([]byte(e.name)); err != nil {
		return err
	}
	zw.written = zw.written + int64(len(e.name))
	if _, err := zw.w.Write(compData); err != nil {
		return err
	}
	zw.written = zw.written + int64(len(compData))
	zw.current = nil
	return nil
}

func (zw *Writer) Close() error {
	if err := zw.finishCurrent(); err != nil {
		return err
	}
	cdStart := zw.written
	i := 0
	for i < len(zw.entries) {
		e := zw.entries[i]
		ch := centralHeader(e)
		if _, err := zw.w.Write(ch); err != nil {
			return err
		}
		zw.written = zw.written + int64(len(ch))
		if _, err := zw.w.Write([]byte(e.name)); err != nil {
			return err
		}
		zw.written = zw.written + int64(len(e.name))
		i = i + 1
	}
	cdSize := zw.written - cdStart

	var eocd []byte
	eocd = appendU32(eocd, 101010256)
	eocd = appendU16(eocd, 0)
	eocd = appendU16(eocd, 0)
	eocd = appendU16(eocd, uint16(len(zw.entries)))
	eocd = appendU16(eocd, uint16(len(zw.entries)))
	eocd = appendU32(eocd, uint32(cdSize))
	eocd = appendU32(eocd, uint32(cdStart))
	eocd = appendU16(eocd, 0)
	_, err := zw.w.Write(eocd)
	return err
}

type fileReader struct {
	r        io.Reader
	digest   *crc32.Digest
	expected uint32
	checked  bool
}

func (fr *fileReader) Read(p []byte) (int, error) {
	n, err := fr.r.Read(p)
	if n > 0 {
		fr.digest.Write(p[:n])
	}
	if err == io.EOF && !fr.checked {
		fr.checked = true
		if fr.digest.Sum32() != fr.expected {
			return n, ErrChecksum
		}
	}
	return n, err
}

type File struct {
	Name             string
	Method           uint16
	CRC32            uint32
	CompressedSize   uint32
	UncompressedSize uint32
	data             []byte
}

func (f *File) Open() (io.Reader, error) {
	var base io.Reader
	if f.Method == 0 {
		base = bytes.NewReader(f.data)
	} else if f.Method == 8 {
		base = flate.NewReader(bytes.NewReader(f.data))
	} else {
		return nil, ErrAlgorithm
	}
	return &fileReader{r: base, digest: crc32.NewIEEE(), expected: f.CRC32}, nil
}

type Reader struct {
	File []*File
}

func NewReader(data []byte) (*Reader, error) {
	n := len(data)
	minEOCD := 22
	end := n - minEOCD
	if end < 0 {
		return nil, ErrFormat
	}
	eocdOff := -1
	i := end
	for i >= 0 {
		if data[i] == 80 && data[i+1] == 75 && data[i+2] == 5 && data[i+3] == 6 {
			eocdOff = i
			break
		}
		i = i - 1
	}
	if eocdOff < 0 {
		return nil, ErrFormat
	}
	cdCount := readU16(data, eocdOff+10)
	cdOffset := readU32(data, eocdOff+16)

	zr := &Reader{}
	pos := int(cdOffset)
	idx := 0
	for idx < cdCount {
		if pos+46 > n {
			return nil, ErrFormat
		}
		sig := readU32(data, pos)
		if sig != 33639248 {
			return nil, ErrFormat
		}
		method := uint16(readU16(data, pos+10))
		crc := readU32(data, pos+16)
		compSize := readU32(data, pos+20)
		uncompSize := readU32(data, pos+24)
		nameLen := readU16(data, pos+28)
		extraLen := readU16(data, pos+30)
		commentLen := readU16(data, pos+32)
		localOffset := readU32(data, pos+42)
		name := string(data[pos+46 : pos+46+nameLen])
		pos = pos + 46 + nameLen + extraLen + commentLen

		lpos := int(localOffset)
		lNameLen := readU16(data, lpos+26)
		lExtraLen := readU16(data, lpos+28)
		dataStart := lpos + 30 + lNameLen + lExtraLen
		fdata := data[dataStart : dataStart+int(compSize)]

		zr.File = append(zr.File, &File{
			Name:             name,
			Method:           method,
			CRC32:            crc,
			CompressedSize:   compSize,
			UncompressedSize: uncompSize,
			data:             fdata,
		})
		idx = idx + 1
	}
	return zr, nil
}
