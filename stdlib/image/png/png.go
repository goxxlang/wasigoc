// Bounded image/png: a real PNG encoder/decoder built on compress/zlib and
// hash/crc32. `Encode` takes a concrete `*image.RGBA` (not the generic
// `image.Image` interface real Go's `Encode` accepts) since this project's
// `image` package only has one concrete pixel-buffer type anyway (see its
// own tracker line) -- always writes color type 6 (truecolor+alpha, 8-bit),
// filter type 0 (None) per scanline, no interlacing. `Decode` is more
// general than `Encode` on purpose (same "encoder simple, decoder general"
// split as compress/flate): color types 0 (grayscale), 2 (truecolor), and
// 6 (truecolor+alpha) at 8-bit depth, and ALL FIVE PNG filter types (None/
// Sub/Up/Average/Paeth) per RFC 2083 section 6, not just the one this
// package's own Encode emits -- so it can decode a real-world PNG written
// by any real encoder, not just its own output. NOT supported: palette
// (color type 3), 16-bit depth, interlacing (Adam7), ancillary chunks
// beyond skipping them (gAMA/tRNS/etc. are read past, not interpreted).
// Every chunk's CRC-32 is verified against the stream, not just skipped.
package png

import (
	"bytes"
	"compress/zlib"
	"errors"
	"hash/crc32"
	"image"
	"io"
)

var pngSignature = []byte{137, 80, 78, 71, 13, 10, 26, 10}

var ErrFormat = errors.New("png: invalid format")
var ErrUnsupported = errors.New("png: unsupported PNG feature")
var ErrChecksum = errors.New("png: chunk checksum mismatch")

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

func appendU32BE(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func readU32BE(b []byte, off int) uint32 {
	return (uint32(b[off]) << 24) | (uint32(b[off+1]) << 16) | (uint32(b[off+2]) << 8) | uint32(b[off+3])
}

func writeChunk(w io.Writer, typ string, data []byte) error {
	var out []byte
	out = appendU32BE(out, uint32(len(data)))
	out = append(out, []byte(typ)...)
	out = append(out, data...)
	crcInput := append([]byte(typ), data...)
	crc := crc32.ChecksumIEEE(crcInput)
	out = appendU32BE(out, crc)
	_, err := w.Write(out)
	return err
}

func Encode(w io.Writer, img *image.RGBA) error {
	if _, err := w.Write(pngSignature); err != nil {
		return err
	}
	r := img.Bounds()
	width := r.Dx()
	height := r.Dy()

	var ihdr []byte
	ihdr = appendU32BE(ihdr, uint32(width))
	ihdr = appendU32BE(ihdr, uint32(height))
	ihdr = append(ihdr, 8, 6, 0, 0, 0)
	if err := writeChunk(w, "IHDR", ihdr); err != nil {
		return err
	}

	var raw []byte
	y := 0
	for y < height {
		raw = append(raw, 0)
		rowStart := y * img.Stride
		raw = append(raw, img.Pix[rowStart:rowStart+width*4]...)
		y = y + 1
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	zw.Write(raw)
	zw.Close()
	if err := writeChunk(w, "IDAT", zbuf.Bytes()); err != nil {
		return err
	}
	return writeChunk(w, "IEND", nil)
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func paeth(a byte, b byte, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa := absInt(p - int(a))
	pb := absInt(p - int(b))
	pc := absInt(p - int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}

func unfilter(line []byte, prev []byte, filterType byte, bpp int) {
	n := len(line)
	i := 0
	for i < n {
		var a byte
		var c byte
		if i >= bpp {
			a = line[i-bpp]
			c = prev[i-bpp]
		}
		b := prev[i]
		if filterType == 1 {
			line[i] = line[i] + a
		} else if filterType == 2 {
			line[i] = line[i] + b
		} else if filterType == 3 {
			avg := (int(a) + int(b)) / 2
			line[i] = line[i] + byte(avg)
		} else if filterType == 4 {
			line[i] = line[i] + paeth(a, b, c)
		}
		i = i + 1
	}
}

func Decode(r io.Reader) (*image.RGBA, error) {
	sig, err := readN(r, 8)
	if err != nil {
		return nil, err
	}
	i := 0
	for i < 8 {
		if sig[i] != pngSignature[i] {
			return nil, ErrFormat
		}
		i = i + 1
	}

	width := 0
	height := 0
	var bitDepth byte
	var colorType byte
	var idat []byte
	for {
		lenB, e1 := readN(r, 4)
		if e1 != nil {
			return nil, e1
		}
		length := int(readU32BE(lenB, 0))
		typB, e2 := readN(r, 4)
		if e2 != nil {
			return nil, e2
		}
		data, e3 := readN(r, length)
		if e3 != nil {
			return nil, e3
		}
		crcB, e4 := readN(r, 4)
		if e4 != nil {
			return nil, e4
		}
		expected := readU32BE(crcB, 0)
		actual := crc32.ChecksumIEEE(append([]byte(typB), data...))
		if actual != expected {
			return nil, ErrChecksum
		}
		typ := string(typB)
		if typ == "IHDR" {
			width = int(readU32BE(data, 0))
			height = int(readU32BE(data, 4))
			bitDepth = data[8]
			colorType = data[9]
		} else if typ == "IDAT" {
			idat = append(idat, data...)
		} else if typ == "IEND" {
			break
		}
	}

	if bitDepth != 8 {
		return nil, ErrUnsupported
	}
	var bpp int
	if colorType == 6 {
		bpp = 4
	} else if colorType == 2 {
		bpp = 3
	} else if colorType == 0 {
		bpp = 1
	} else {
		return nil, ErrUnsupported
	}

	zr, zerr := zlib.NewReader(bytes.NewReader(idat))
	if zerr != nil {
		return nil, zerr
	}
	raw, rerr := io.ReadAll(zr)
	if rerr != nil {
		return nil, rerr
	}

	stride := width * bpp
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	prev := make([]byte, stride)
	pos := 0
	y := 0
	for y < height {
		if pos >= len(raw) {
			return nil, ErrFormat
		}
		filterType := raw[pos]
		pos = pos + 1
		line := make([]byte, stride)
		copy(line, raw[pos:pos+stride])
		pos = pos + stride
		unfilter(line, prev, filterType, bpp)

		x := 0
		for x < width {
			var rr, gg, bb, aa byte
			if colorType == 6 {
				rr = line[x*4]
				gg = line[x*4+1]
				bb = line[x*4+2]
				aa = line[x*4+3]
			} else if colorType == 2 {
				rr = line[x*3]
				gg = line[x*3+1]
				bb = line[x*3+2]
				aa = 255
			} else {
				rr = line[x]
				gg = line[x]
				bb = line[x]
				aa = 255
			}
			off := img.PixOffset(x, y)
			img.Pix[off] = rr
			img.Pix[off+1] = gg
			img.Pix[off+2] = bb
			img.Pix[off+3] = aa
			x = x + 1
		}
		prev = line
		y = y + 1
	}
	return img, nil
}
