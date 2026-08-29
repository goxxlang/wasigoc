// Bounded image/gif: `Decode` only, first frame of a GIF87a/GIF89a stream
// (no `DecodeAll`/`GIF` animation struct, no `Encode` -- encoding needs
// color quantization from truecolor down to a <=256 color palette, a
// substantial separate algorithm this project doesn't have, an honest gap
// rather than an invented feature). Returns a concrete `*image.RGBA` (this
// project's `image` package has no `Paletted` concrete type -- see its own
// tracker line), resolving each pixel's palette index immediately rather
// than keeping an indexed buffer, the same "one concrete pixel-buffer type"
// bound `image/draw`/`image/png` already use. Supports: global AND local
// color tables, the Graphic Control Extension's transparent-color index
// (rendered as a zero `color.Rgba{}`, matching real Go's own
// `image/gif.Decode` behavior for a single frame), 4-pass row interlacing
// (GIF's interlacing is simple row reordering, unlike PNG's 7-pass Adam7
// which this project's `image/png` already docments as NOT supported), and
// skips over Comment/Plain Text/Application extension sub-blocks without
// interpreting them. Does NOT support: multiple image frames (returns as
// soon as the first `sImageDescriptor` block decodes, same early-return
// real Go's own `Decode` -- as opposed to `DecodeAll` -- already does),
// the lenient "one stray trailing sub-block" tolerance real Go's decoder
// has for malformed encoders (golang.org/issue/16146) -- an honest,
// narrower bound, not a silent behavior gap.
//
// GIF's LZW variant needs NO special-casing in this project's own
// `compress/lzw`: real Go's own package doc says explicitly "it implements
// LZW as used by the GIF and PDF file formats" (TIFF is the incompatible
// one, not GIF) -- verified directly against real Go's source
// (`compress/lzw/reader.go`'s doc comment and `image/gif/reader.go`, which
// itself calls the stock `lzw.NewReader(br, lzw.LSB, litWidth)` with no
// GIF-specific "early change" wrapper at all. This corrects an earlier,
// unverified assumption in this project's own tracker that GIF needed such
// a quirk -- it doesn't, real Go's own source settles it). Verified via:
// (1) real Go's `image/gif.Decode` decoding a Pillow-generated indexed GIF
// and a hand-built (this project's own frame layout, real Go's
// `compress/lzw.Writer` for the bitstream) interlaced GIF, both giving the
// pixel values embedded in this package's own golden test; (2) round-trip
// through this project's own `compress/lzw` package already proven
// wire-compatible with real Go's (see its own tracker line's bump-timing
// note) means a GIF written by any real encoder decodes correctly here too.
package gif

import (
	"compress/lzw"
	"errors"
	"image"
	"io"
)

var ErrFormat = errors.New("gif: invalid format")

func readN(r io.Reader, n int) ([]byte, error) {
	out := make([]byte, n)
	got := 0
	for got < n {
		buf := make([]byte, 1)
		k, err := r.Read(buf)
		if k == 0 {
			if err != nil {
				return nil, err
			}
			return nil, io.ErrUnexpectedEOF
		}
		out[got] = buf[0]
		got = got + 1
	}
	return out, nil
}

func readByte(r io.Reader) (byte, error) {
	b, err := readN(r, 1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func skipSubBlocks(r io.Reader) error {
	for {
		size, err := readByte(r)
		if err != nil {
			return err
		}
		if size == 0 {
			return nil
		}
		if _, err := readN(r, int(size)); err != nil {
			return err
		}
	}
}

// blockReader unwraps a GIF sub-block stream ((n, n bytes)*, 0) into a
// continuous byte stream, the shape this project's `compress/lzw.Reader`
// needs (it only ever calls plain `Read`, never a `ReadByte` fast path, so
// unlike real Go's own blockReader there's no need to implement one).
type blockReader struct {
	r    io.Reader
	buf  []byte
	pos  int
	done bool
}

func (b *blockReader) fill() error {
	size, err := readByte(b.r)
	if err != nil {
		return err
	}
	if size == 0 {
		b.done = true
		return io.EOF
	}
	data, err := readN(b.r, int(size))
	if err != nil {
		return err
	}
	b.buf = data
	b.pos = 0
	return nil
}

func (b *blockReader) Read(p []byte) (int, error) {
	if b.done {
		return 0, io.EOF
	}
	if b.pos >= len(b.buf) {
		if err := b.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(p, b.buf[b.pos:])
	b.pos = b.pos + n
	return n, nil
}

type gifColor struct {
	r byte
	g byte
	b byte
}

func readColorTable(r io.Reader, fields byte) ([]gifColor, error) {
	n := 1 << (1 + int(fields&7))
	data, err := readN(r, 3*n)
	if err != nil {
		return nil, err
	}
	table := make([]gifColor, n)
	i := 0
	for i < n {
		table[i] = gifColor{r: data[3*i], g: data[3*i+1], b: data[3*i+2]}
		i = i + 1
	}
	return table, nil
}

var interlaceSkip = []int{8, 8, 4, 2}
var interlaceStart = []int{0, 4, 2, 1}

// deinterlace rearranges GIF's 4-pass interlaced row order (scan lines
// arrive pass-by-pass: every 8th row from 0, every 8th from 4, every 4th
// from 2, every 2nd from 1) back into normal top-to-bottom order.
func deinterlace(pix []byte, width int, height int) []byte {
	out := make([]byte, len(pix))
	srcRow := 0
	pass := 0
	for pass < 4 {
		y := interlaceStart[pass]
		for y < height {
			copy(out[y*width:y*width+width], pix[srcRow*width:srcRow*width+width])
			srcRow = srcRow + 1
			y = y + interlaceSkip[pass]
		}
		pass = pass + 1
	}
	return out
}

func Decode(r io.Reader) (*image.RGBA, error) {
	header, err := readN(r, 6)
	if err != nil {
		return nil, err
	}
	vers := string(header)
	if vers != "GIF87a" && vers != "GIF89a" {
		return nil, ErrFormat
	}
	screen, err := readN(r, 7)
	if err != nil {
		return nil, err
	}
	fields := screen[4]
	var globalTable []gifColor
	if fields&0x80 != 0 {
		globalTable, err = readColorTable(r, fields)
		if err != nil {
			return nil, err
		}
	}

	hasTransparent := false
	var transparentIndex byte

	for {
		sec, err := readByte(r)
		if err != nil {
			return nil, err
		}
		if sec == 0x3B {
			return nil, errors.New("gif: missing image data")
		}
		if sec == 0x21 {
			ext, err := readByte(r)
			if err != nil {
				return nil, err
			}
			if ext == 0xF9 {
				block, err := readN(r, 6)
				if err != nil {
					return nil, err
				}
				if block[0] != 4 || block[5] != 0 {
					return nil, ErrFormat
				}
				if block[1]&0x01 != 0 {
					hasTransparent = true
					transparentIndex = block[4]
				} else {
					hasTransparent = false
				}
			} else {
				if err := skipSubBlocks(r); err != nil {
					return nil, err
				}
			}
			continue
		}
		if sec != 0x2C {
			return nil, errors.New("gif: unknown block type")
		}

		desc, err := readN(r, 9)
		if err != nil {
			return nil, err
		}
		left := int(desc[0]) + int(desc[1])<<8
		top := int(desc[2]) + int(desc[3])<<8
		fwidth := int(desc[4]) + int(desc[5])<<8
		fheight := int(desc[6]) + int(desc[7])<<8
		imgFields := desc[8]

		table := globalTable
		if imgFields&0x80 != 0 {
			table, err = readColorTable(r, imgFields)
			if err != nil {
				return nil, err
			}
		}
		if table == nil {
			return nil, errors.New("gif: no color table")
		}

		litWidth, err := readByte(r)
		if err != nil {
			return nil, err
		}
		if litWidth < 2 || litWidth > 8 {
			return nil, ErrFormat
		}

		br := &blockReader{r: r}
		lzwr := lzw.NewReader(br, lzw.LSB, int(litWidth))
		pix := make([]byte, fwidth*fheight)
		n := 0
		for n < len(pix) {
			k, rerr := lzwr.Read(pix[n:])
			n = n + k
			if rerr != nil {
				break
			}
			if k == 0 {
				break
			}
		}
		if n < len(pix) {
			return nil, errors.New("gif: not enough image data")
		}
		if err := skipSubBlocks(r); err != nil {
			return nil, err
		}

		if imgFields&0x40 != 0 {
			pix = deinterlace(pix, fwidth, fheight)
		}

		img := image.NewRGBA(image.Rect(left, top, left+fwidth, top+fheight))
		i := 0
		for i < len(pix) {
			idx := pix[i]
			if int(idx) >= len(table) {
				return nil, errors.New("gif: invalid pixel value")
			}
			off := i * 4
			if hasTransparent && idx == transparentIndex {
				img.Pix[off] = 0
				img.Pix[off+1] = 0
				img.Pix[off+2] = 0
				img.Pix[off+3] = 0
			} else {
				c := table[idx]
				img.Pix[off] = c.r
				img.Pix[off+1] = c.g
				img.Pix[off+2] = c.b
				img.Pix[off+3] = 255
			}
			i = i + 1
		}
		return img, nil
	}
}
