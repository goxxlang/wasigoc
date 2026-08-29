// Bounded image/jpeg: `Decode` only (no `Encode`/`DecodeConfig`/`DecodeAll`
// equivalent). Baseline (SOF0) sequential DCT only -- no progressive
// (SOF2), no extended sequential (SOF1), no arithmetic coding (none of
// these are Huffman-based baseline, a substantially different decode
// path real Go itself splits into a much larger `scan.go`). 8-bit
// precision only. Grayscale (1 component) and YCbCr (3 component) only --
// no CMYK/YCbCrK (4 component, needs Adobe APP14 interpretation this
// package doesn't do). Any (h,v) subsampling ratio real Go itself accepts
// (4:4:4, 4:4:0, 4:2:2, 4:2:0, 4:1:1, 4:1:0) is supported, upsampled by
// nearest-neighbor exactly like real Go's own `convertToRGB`. Restart
// markers (DRI/RSTn) are supported, but -- unlike real Go's `findRST` --
// a restart marker that doesn't exactly match the expected marker is a
// hard FormatError rather than attempting libjpeg-style resynchronization
// against corrupt streams; same "honest narrower bound, not a silent gap"
// precedent as image/gif's own skipped trailing-sub-block leniency.
// Requires the (near-universal, for any ordinary encoder) single fully
// interleaved scan -- a scan naming fewer than all of the frame's
// components (only otherwise possible in a progressive or unusual
// multi-scan baseline file) is rejected as unsupported. Returns a
// concrete `*image.RGBA` (this project's `image` has no Gray/YCbCr
// concrete types), converting through the exact integer arithmetic real
// Go's own `image/color.YCbCrToRGB` uses.
//
// The Huffman decoder ports only real Go's own general "slow path" bit-
// by-bit algorithm (`compress/lzw`-style bit accumulator), not its
// look-up-table fast path -- the LUT is a pure speed optimization real Go
// itself falls back off of for any code longer than 8 bits or for the
// tail end of a scan's data, so the slow path alone is already complete
// and correct on its own, just slower; this project doesn't need JPEG
// decode to be fast, only correct.
//
// The IDCT is instead ported VERBATIM (dctBox, the fixed-point constants
// scaled by `c()`/`dctC`, idctRows, idctCols) from real Go's own
// `image/jpeg/dct.go` (a fixed-point Loeffler/Lightenberg/Mostchytz fast
// IDCT) rather than a simpler direct-formula IDCT, specifically so this
// package's decoded pixels can be verified BIT-IDENTICAL against real
// Go's own `image/jpeg.Decode` on the same file, not merely "close" --
// stronger evidence than the usual JPEG-decoder-to-JPEG-decoder tolerance
// (different decoders' IDCTs are normally expected to differ by up to a
// few least-significant bits).
package jpeg

import (
	"errors"
	"image"
	"io"
)

// Unlike real Go's own `FormatError`/`UnsupportedError` (defined `string`
// types with an `Error()` method, letting a caller distinguish the two by
// type-asserting the returned error), every error here is a plain
// `errors.New(...)` sentinel/value -- this project's codegen has no
// adapter from a user struct satisfying `error` to the builtin `error`
// interface's own `wasigo::Error` runtime representation (every other
// stdlib error in this project already goes through `errors.New`
// directly, never a custom struct type; this package follows the same,
// only proven, precedent rather than being the first to need that
// adapter).
const blockSize = 64

type block [64]int32

const (
	dcTable       = 0
	acTable       = 1
	maxTq         = 3
	maxComponents = 3
)

const (
	sof0Marker  = 0xc0
	sof2Marker  = 0xc2
	dhtMarker   = 0xc4
	rst0Marker  = 0xd0
	rst7Marker  = 0xd7
	soiMarker   = 0xd8
	eoiMarker   = 0xd9
	sosMarker   = 0xda
	dqtMarker   = 0xdb
	driMarker   = 0xdd
	app0Marker  = 0xe0
	app15Marker = 0xef
	comMarker   = 0xfe
)

var unzig = []int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

type component struct {
	h  int
	v  int
	c  byte
	tq byte
}

type huffman struct {
	nCodes int32
	// `vals` is a `[]byte` slice, allocated fresh in processDHT, not a
	// fixed `[256]byte` array -- this project's array-to-slice conversion
	// (`arr[lo:hi]`) copies the windowed bytes into a new backing slice
	// rather than aliasing the array's own storage (`slice_array` in
	// runtime.hpp says so explicitly: "Arrays live on the stack; slicing
	// copies the window... WASM has no growable stacks to alias a stack
	// array"), so writing through `d.readFull(vals[:n])` into a `[256]byte`
	// field would silently write into a throwaway copy and leave the real
	// field zeroed. A `[]byte` field sidesteps this entirely, since slice
	// re-slicing (as opposed to array-to-slice conversion) already aliases
	// correctly everywhere else in this project.
	vals        []byte
	minCodes    [16]int32
	maxCodes    [16]int32
	valsIndices [16]int32
}

type jbits struct {
	acc uint32
	n   int32
}

type decoder struct {
	r       io.Reader
	pending []byte
	bits    jbits

	width  int
	height int
	nComp  int
	ri     int

	comp  [3]component
	huff  [2][2]huffman
	quant [4]block

	plane       [3][]byte
	planeStride [3]int

	// `[]byte`, not `[128]byte` -- same array-to-slice-copy reason as
	// huffman.vals above; every read into `d.tmp[:n]` needs real slice
	// aliasing back into this field.
	tmp []byte
}

// ---- byte-level reading (marker/header data; never byte-stuffed) --------

func (d *decoder) readRawByte() (byte, error) {
	if len(d.pending) > 0 {
		b := d.pending[0]
		d.pending = d.pending[1:]
		return b, nil
	}
	buf := make([]byte, 1)
	n, err := d.r.Read(buf)
	if n == 0 {
		if err != nil {
			return 0, err
		}
		return 0, io.ErrUnexpectedEOF
	}
	return buf[0], nil
}

func (d *decoder) readByte() (byte, error) {
	return d.readRawByte()
}

func (d *decoder) readFull(p []byte) error {
	i := 0
	for i < len(p) {
		b, err := d.readRawByte()
		if err != nil {
			return err
		}
		p[i] = b
		i = i + 1
	}
	return nil
}

func (d *decoder) ignore(n int) error {
	i := 0
	for i < n {
		_, err := d.readRawByte()
		if err != nil {
			return err
		}
		i = i + 1
	}
	return nil
}

// readByteStuffed reads one byte of entropy-coded (Huffman) data, undoing
// JPEG's 0xFF 0x00 -> 0xFF byte stuffing. When a 0xFF is followed by
// anything else, that is the start of a marker (a restart marker, or the
// end of the scan) -- both bytes are pushed back for the caller (the
// restart-marker check in processSOS, or the top-level marker scan) to
// see, and errEndOfEntropyData is returned.
var errEndOfEntropyData = errors.New("missing 0xff00 sequence")

func (d *decoder) readByteStuffed() (byte, error) {
	b, err := d.readRawByte()
	if err != nil {
		return 0, err
	}
	if b != 0xff {
		return b, nil
	}
	b2, err := d.readRawByte()
	if err != nil {
		return 0, err
	}
	if b2 == 0x00 {
		return 0xff, nil
	}
	d.pending = []byte{0xff, b2}
	return 0, errEndOfEntropyData
}

// ---- bit-level reading (entropy-coded data) ------------------------------

func (d *decoder) ensureBits(n int32) error {
	for d.bits.n < n {
		c, err := d.readByteStuffed()
		if err != nil {
			return err
		}
		d.bits.acc = (d.bits.acc << 8) | uint32(c)
		d.bits.n = d.bits.n + 8
	}
	return nil
}

func (d *decoder) decodeBit() (bool, error) {
	if err := d.ensureBits(1); err != nil {
		return false, err
	}
	d.bits.n = d.bits.n - 1
	bit := (d.bits.acc >> uint(d.bits.n)) & 1
	return bit != 0, nil
}

func (d *decoder) decodeBits(n int32) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if err := d.ensureBits(n); err != nil {
		return 0, err
	}
	d.bits.n = d.bits.n - n
	v := (d.bits.acc >> uint(d.bits.n)) & ((uint32(1) << uint(n)) - 1)
	return v, nil
}

// receiveExtend is JPEG's RECEIVE+EXTEND (section F.2.2.1): read t bits
// and sign-extend per the standard EXTEND procedure.
func (d *decoder) receiveExtend(t byte) (int32, error) {
	if t == 0 {
		return 0, nil
	}
	v, err := d.decodeBits(int32(t))
	if err != nil {
		return 0, err
	}
	s := int32(1) << t
	x := int32(v)
	if x < s>>1 {
		x = x - s + 1
	}
	return x, nil
}

// ---- Huffman decoding (general bit-by-bit algorithm, no LUT) ------------

func (d *decoder) decodeHuffman(h *huffman) (byte, error) {
	if h.nCodes == 0 {
		return 0, errors.New("uninitialized Huffman table")
	}
	code := int32(0)
	i := 0
	for i < 16 {
		bit, err := d.decodeBit()
		if err != nil {
			return 0, err
		}
		if bit {
			code = code | 1
		}
		if h.maxCodes[i] >= 0 && code <= h.maxCodes[i] {
			return h.vals[h.valsIndices[i]+code-h.minCodes[i]], nil
		}
		code = code << 1
		i = i + 1
	}
	return 0, errors.New("bad Huffman code")
}

func (d *decoder) processDHT(n int) error {
	for n > 0 {
		if n < 17 {
			return errors.New("DHT has wrong length")
		}
		if err := d.readFull(d.tmp[:17]); err != nil {
			return err
		}
		tc := d.tmp[0] >> 4
		if tc > 1 {
			return errors.New("bad Tc value")
		}
		th := d.tmp[0] & 0x0f
		if th > 1 {
			return errors.New("bad Th value")
		}
		h := &d.huff[tc][th]

		var nCodes [16]int32
		h.nCodes = 0
		i := 0
		for i < 16 {
			nCodes[i] = int32(d.tmp[i+1])
			h.nCodes = h.nCodes + nCodes[i]
			i = i + 1
		}
		if h.nCodes == 0 {
			return errors.New("Huffman table has zero length")
		}
		if h.nCodes > 256 {
			return errors.New("Huffman table has excessive length")
		}
		n = n - int(h.nCodes) - 17
		if n < 0 {
			return errors.New("DHT has wrong length")
		}
		h.vals = make([]byte, h.nCodes)
		if err := d.readFull(h.vals); err != nil {
			return err
		}

		c := int32(0)
		index := int32(0)
		i = 0
		for i < 16 {
			cnt := nCodes[i]
			if cnt == 0 {
				h.minCodes[i] = -1
				h.maxCodes[i] = -1
				h.valsIndices[i] = -1
			} else {
				h.minCodes[i] = c
				h.maxCodes[i] = c + cnt - 1
				h.valsIndices[i] = index
				c = c + cnt
				index = index + cnt
			}
			c = c << 1
			i = i + 1
		}
	}
	return nil
}

// ---- marker segment parsing ----------------------------------------------

func (d *decoder) processSOF(n int) error {
	if d.nComp != 0 {
		return errors.New("multiple SOF markers")
	}
	if n == 6+3*1 {
		d.nComp = 1
	} else if n == 6+3*3 {
		d.nComp = 3
	} else {
		return errors.New("number of components")
	}
	if err := d.readFull(d.tmp[:n]); err != nil {
		return err
	}
	if d.tmp[0] != 8 {
		return errors.New("precision")
	}
	d.height = (int(d.tmp[1]) << 8) + int(d.tmp[2])
	d.width = (int(d.tmp[3]) << 8) + int(d.tmp[4])
	if int(d.tmp[5]) != d.nComp {
		return errors.New("SOF has wrong length")
	}

	i := 0
	for i < d.nComp {
		d.comp[i].c = d.tmp[6+3*i]
		j := 0
		for j < i {
			if d.comp[i].c == d.comp[j].c {
				return errors.New("repeated component identifier")
			}
			j = j + 1
		}
		d.comp[i].tq = d.tmp[8+3*i]
		if d.comp[i].tq > maxTq {
			return errors.New("bad Tq value")
		}
		hv := d.tmp[7+3*i]
		h := int(hv >> 4)
		v := int(hv & 0x0f)
		if h < 1 || h > 4 || v < 1 || v > 4 {
			return errors.New("luma/chroma subsampling ratio")
		}
		if h == 3 || v == 3 {
			return errors.New("luma/chroma subsampling ratio")
		}
		if d.nComp == 1 {
			h = 1
			v = 1
		} else if i == 0 {
			if v == 4 {
				return errors.New("luma/chroma subsampling ratio")
			}
		} else if i == 1 {
			if d.comp[0].h%h != 0 || d.comp[0].v%v != 0 {
				return errors.New("luma/chroma subsampling ratio")
			}
		} else if i == 2 {
			if d.comp[1].h != h || d.comp[1].v != v {
				return errors.New("luma/chroma subsampling ratio")
			}
		}
		d.comp[i].h = h
		d.comp[i].v = v
		i = i + 1
	}
	return nil
}

func (d *decoder) processDQT(n int) error {
	for n > 0 {
		n = n - 1
		x, err := d.readByte()
		if err != nil {
			return err
		}
		tq := x & 0x0f
		if tq > maxTq {
			return errors.New("bad Tq value")
		}
		pq := x >> 4
		if pq == 0 {
			if n < blockSize {
				break
			}
			n = n - blockSize
			if err := d.readFull(d.tmp[:blockSize]); err != nil {
				return err
			}
			i := 0
			for i < blockSize {
				d.quant[tq][i] = int32(d.tmp[i])
				i = i + 1
			}
		} else if pq == 1 {
			if n < 2*blockSize {
				break
			}
			n = n - 2*blockSize
			if err := d.readFull(d.tmp[:2*blockSize]); err != nil {
				return err
			}
			i := 0
			for i < blockSize {
				d.quant[tq][i] = (int32(d.tmp[2*i]) << 8) + int32(d.tmp[2*i+1])
				i = i + 1
			}
		} else {
			return errors.New("bad Pq value")
		}
	}
	if n != 0 {
		return errors.New("DQT has wrong length")
	}
	return nil
}

func (d *decoder) processDRI(n int) error {
	if n != 2 {
		return errors.New("DRI has wrong length")
	}
	if err := d.readFull(d.tmp[:2]); err != nil {
		return err
	}
	d.ri = (int(d.tmp[0]) << 8) + int(d.tmp[1])
	return nil
}

func (d *decoder) allocPlanes(mxx int, myy int) {
	i := 0
	for i < d.nComp {
		w := 8 * d.comp[i].h * mxx
		h := 8 * d.comp[i].v * myy
		d.plane[i] = make([]byte, w*h)
		d.planeStride[i] = w
		i = i + 1
	}
}

func (d *decoder) processSOS(n int) error {
	if d.nComp == 0 {
		return errors.New("missing SOF marker")
	}
	if n < 6 || n > 4+2*d.nComp || n%2 != 0 {
		return errors.New("SOS has wrong length")
	}
	if err := d.readFull(d.tmp[:n]); err != nil {
		return err
	}
	nComp := int(d.tmp[0])
	if n != 4+2*nComp {
		return errors.New("SOS length inconsistent with number of components")
	}
	if nComp != d.nComp {
		return errors.New("scan does not cover all components")
	}

	var scanCompIndex [3]int
	var scanTd [3]byte
	var scanTa [3]byte
	i := 0
	for i < nComp {
		cs := d.tmp[1+2*i]
		compIndex := -1
		j := 0
		for j < d.nComp {
			if cs == d.comp[j].c {
				compIndex = j
			}
			j = j + 1
		}
		if compIndex < 0 {
			return errors.New("unknown component selector")
		}
		scanCompIndex[i] = compIndex
		scanTd[i] = d.tmp[2+2*i] >> 4
		scanTa[i] = d.tmp[2+2*i] & 0x0f
		if scanTd[i] > 1 || scanTa[i] > 1 {
			return errors.New("bad Td/Ta value")
		}
		i = i + 1
	}

	h0 := d.comp[0].h
	v0 := d.comp[0].v
	mxx := (d.width + 8*h0 - 1) / (8 * h0)
	myy := (d.height + 8*v0 - 1) / (8 * v0)
	if d.plane[0] == nil {
		d.allocPlanes(mxx, myy)
	}

	d.bits = jbits{}
	var dc [3]int32
	mcu := 0
	expectedRST := byte(rst0Marker)

	my := 0
	for my < myy {
		mx := 0
		for mx < mxx {
			ci := 0
			for ci < nComp {
				compIndex := scanCompIndex[ci]
				hi := d.comp[compIndex].h
				vi := d.comp[compIndex].v
				j := 0
				for j < hi*vi {
					bx := hi*mx + j%hi
					by := vi*my + j/hi

					var b block

					dcVal, err := d.decodeHuffman(&d.huff[dcTable][scanTd[ci]])
					if err != nil {
						return err
					}
					if dcVal > 16 {
						return errors.New("excessive DC component")
					}
					dcDelta, err := d.receiveExtend(dcVal)
					if err != nil {
						return err
					}
					dc[compIndex] = dc[compIndex] + dcDelta
					b[0] = dc[compIndex]

					huff := &d.huff[acTable][scanTa[ci]]
					zig := 1
					for zig <= 63 {
						value, err := d.decodeHuffman(huff)
						if err != nil {
							return err
						}
						val0 := value >> 4
						val1 := value & 0x0f
						if val1 != 0 {
							zig = zig + int(val0)
							if zig > 63 {
								break
							}
							ac, err := d.receiveExtend(val1)
							if err != nil {
								return err
							}
							b[unzig[zig]] = ac
							zig = zig + 1
						} else {
							if val0 != 0x0f {
								break
							}
							zig = zig + 16
						}
					}

					d.reconstructBlock(&b, bx, by, compIndex)
					j = j + 1
				}
				ci = ci + 1
			}
			mcu = mcu + 1
			if d.ri > 0 && mcu%d.ri == 0 && mcu < mxx*myy {
				if err := d.readFull(d.tmp[:2]); err != nil {
					return err
				}
				if d.tmp[0] != 0xff || d.tmp[1] != expectedRST {
					return errors.New("missing restart marker")
				}
				expectedRST = expectedRST + 1
				if expectedRST == rst7Marker+1 {
					expectedRST = rst0Marker
				}
				d.bits = jbits{}
				dc[0] = 0
				dc[1] = 0
				dc[2] = 0
			}
			mx = mx + 1
		}
		my = my + 1
	}
	return nil
}

func (d *decoder) reconstructBlock(b *block, bx int, by int, compIndex int) {
	qt := &d.quant[d.comp[compIndex].tq]
	zig := 0
	for zig < blockSize {
		b[unzig[zig]] = b[unzig[zig]] * qt[zig]
		zig = zig + 1
	}
	idct(b)

	stride := d.planeStride[compIndex]
	base := 8 * (by*stride + bx)
	y := 0
	for y < 8 {
		row := base + y*stride
		x := 0
		for x < 8 {
			c := b[y*8+x]
			if c < -128 {
				c = 0
			} else if c > 127 {
				c = 255
			} else {
				c = c + 128
			}
			d.plane[compIndex][row+x] = byte(c)
			x = x + 1
		}
		y = y + 1
	}
}

// ---- inverse DCT, ported verbatim from real Go's image/jpeg/dct.go ------
// (the Loeffler/Lightenberg/Mostchytz fast fixed-point algorithm) so that
// decoded pixels are bit-identical to real Go's own decoder, not merely
// close. See this file's own header comment.

const (
	dctCos1         = 1130768441178740757
	dctSin1         = 224923827593068887
	dctCos3         = 958619196450722178
	dctSin3         = 640528868967736374
	dctSqrt2inv     = 815238614083298888
	dctSqrt2invCos6 = 311978311033955632
	dctSqrt2invSin6 = 753182269664427492
)

func dctC(x uint64, bits int) int32 {
	return int32((x + (uint64(1) << uint(59-bits))) >> uint(60-bits))
}

func dctBox(x0 int32, x1 int32, kcos int32, ksin int32) (int32, int32) {
	ksum := kcos * (x0 + x1)
	y0 := ksum + (ksin-kcos)*x1
	y1 := ksum - (kcos+ksin)*x0
	return y0, y1
}

func idctRows(b *block) {
	i := 0
	for i < 8 {
		base := 8 * i
		x0 := b[base+0]
		x7 := b[base+1]
		x2 := b[base+2]
		x5 := b[base+3]
		x1 := b[base+4]
		x6 := b[base+5]
		x3 := b[base+6]
		x4 := b[base+7]

		x0 = x0 << 17
		x1 = x1 << 17
		x0, x1 = x0+x1, x0-x1

		x2, x3 = dctBox(x2, x3, dctC(dctSqrt2invCos6, 18), -dctC(dctSqrt2invSin6, 18))
		x1, x2 = x1+x2, x1-x2
		x0, x3 = x0+x3, x0-x3

		x4 = x4 << 7
		x7 = x7 << 7
		x7, x4 = x7+x4, x7-x4

		x6 = x6 * dctC(dctSqrt2inv, 8)
		x5 = x5 * dctC(dctSqrt2inv, 8)

		x7, x5 = x7+x5, x7-x5
		x4, x6 = x4+x6, x4-x6

		x4, x7 = dctBox(x4>>2, x7>>2, dctC(dctCos3, 12), -dctC(dctSin3, 12))
		x5, x6 = dctBox(x5>>2, x6>>2, dctC(dctCos1, 12), -dctC(dctSin1, 12))

		x0, x7 = x0+x7, x0-x7
		x1, x6 = x1+x6, x1-x6
		x2, x5 = x2+x5, x2-x5
		x3, x4 = x3+x4, x3-x4

		b[base+0] = x0
		b[base+1] = x1
		b[base+2] = x2
		b[base+3] = x3
		b[base+4] = x4
		b[base+5] = x5
		b[base+6] = x6
		b[base+7] = x7
		i = i + 1
	}
}

func idctCols(b *block) {
	i := 0
	for i < 8 {
		x0 := b[0*8+i]
		x7 := b[1*8+i]
		x2 := b[2*8+i]
		x5 := b[3*8+i]
		x1 := b[4*8+i]
		x6 := b[5*8+i]
		x3 := b[6*8+i]
		x4 := b[7*8+i]

		x0 = x0 + (1 << 19)

		x0, x1 = (x0+x1)>>2, (x0-x1)>>2
		x2, x3 = dctBox(x2>>13, x3>>13, dctC(dctSqrt2invCos6, 12), -dctC(dctSqrt2invSin6, 12))

		x1, x2 = x1+x2, x1-x2
		x0, x3 = x0+x3, x0-x3

		x7, x4 = x7+x4, x7-x4

		x5 = (x5 >> 13) * dctC(dctSqrt2inv, 14)
		x6 = (x6 >> 13) * dctC(dctSqrt2inv, 14)

		x7, x5 = x7+x5, x7-x5
		x4, x6 = x4+x6, x4-x6

		x4, x7 = dctBox(x4>>14, x7>>14, dctC(dctCos3, 12), -dctC(dctSin3, 12))
		x5, x6 = dctBox(x5>>14, x6>>14, dctC(dctCos1, 12), -dctC(dctSin1, 12))

		x0, x7 = x0+x7, x0-x7
		x1, x6 = x1+x6, x1-x6
		x2, x5 = x2+x5, x2-x5
		x3, x4 = x3+x4, x3-x4

		x0 = x0 >> 18
		x1 = x1 >> 18
		x2 = x2 >> 18
		x3 = x3 >> 18
		x4 = x4 >> 18
		x5 = x5 >> 18
		x6 = x6 >> 18
		x7 = x7 >> 18

		b[0*8+i] = x0
		b[1*8+i] = x1
		b[2*8+i] = x2
		b[3*8+i] = x3
		b[4*8+i] = x4
		b[5*8+i] = x5
		b[6*8+i] = x6
		b[7*8+i] = x7
		i = i + 1
	}
}

func idct(b *block) {
	idctRows(b)
	idctCols(b)
}

// ---- final assembly: YCbCr/gray planes -> *image.RGBA -------------------

// ycbcrToRGB is real Go's own image/color.YCbCrToRGB integer formula,
// ported verbatim for bit-identical output.
func ycbcrToRGB(y byte, cb byte, cr byte) (byte, byte, byte) {
	yy1 := int32(y) * 0x10101
	cb1 := int32(cb) - 128
	cr1 := int32(cr) - 128

	r := (yy1 + 91881*cr1) >> 16
	g := (yy1 - 22554*cb1 - 46802*cr1) >> 16
	b := (yy1 + 116130*cb1) >> 16

	if r < 0 {
		r = 0
	} else if r > 255 {
		r = 255
	}
	if g < 0 {
		g = 0
	} else if g > 255 {
		g = 255
	}
	if b < 0 {
		b = 0
	} else if b > 255 {
		b = 255
	}
	return byte(r), byte(g), byte(b)
}

func (d *decoder) assembleRGBA() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, d.width, d.height))
	if d.nComp == 1 {
		stride := d.planeStride[0]
		y := 0
		for y < d.height {
			x := 0
			for x < d.width {
				v := d.plane[0][y*stride+x]
				off := img.PixOffset(x, y)
				img.Pix[off] = v
				img.Pix[off+1] = v
				img.Pix[off+2] = v
				img.Pix[off+3] = 255
				x = x + 1
			}
			y = y + 1
		}
		return img
	}

	yStride := d.planeStride[0]
	cStride := d.planeStride[1]
	hRatio := d.comp[0].h / d.comp[1].h
	vRatio := d.comp[0].v / d.comp[1].v
	y := 0
	for y < d.height {
		cy := y / vRatio
		x := 0
		for x < d.width {
			cx := x / hRatio
			yy := d.plane[0][y*yStride+x]
			cb := d.plane[1][cy*cStride+cx]
			cr := d.plane[2][cy*cStride+cx]
			r, g, b := ycbcrToRGB(yy, cb, cr)
			off := img.PixOffset(x, y)
			img.Pix[off] = r
			img.Pix[off+1] = g
			img.Pix[off+2] = b
			img.Pix[off+3] = 255
			x = x + 1
		}
		y = y + 1
	}
	return img
}

func Decode(r io.Reader) (*image.RGBA, error) {
	var d decoder
	d.r = r
	d.tmp = make([]byte, 128)

	if err := d.readFull(d.tmp[:2]); err != nil {
		return nil, err
	}
	if d.tmp[0] != 0xff || d.tmp[1] != soiMarker {
		return nil, errors.New("missing SOI marker")
	}

	for {
		if err := d.readFull(d.tmp[:2]); err != nil {
			return nil, err
		}
		if d.tmp[0] != 0xff {
			return nil, errors.New("expected marker")
		}
		marker := d.tmp[1]
		for marker == 0xff {
			b, err := d.readByte()
			if err != nil {
				return nil, err
			}
			marker = b
		}
		if marker == 0x00 {
			continue
		}
		if marker == eoiMarker {
			break
		}
		if marker >= rst0Marker && marker <= rst7Marker {
			continue
		}

		if err := d.readFull(d.tmp[:2]); err != nil {
			return nil, err
		}
		n := ((int(d.tmp[0]) << 8) + int(d.tmp[1])) - 2
		if n < 0 {
			return nil, errors.New("short segment length")
		}

		var err error
		switch marker {
		case sof0Marker:
			err = d.processSOF(n)
		case sof2Marker:
			return nil, errors.New("progressive JPEG")
		case dhtMarker:
			err = d.processDHT(n)
		case dqtMarker:
			err = d.processDQT(n)
		case sosMarker:
			err = d.processSOS(n)
		case driMarker:
			err = d.processDRI(n)
		case comMarker:
			err = d.ignore(n)
		default:
			if marker >= app0Marker && marker <= app15Marker {
				err = d.ignore(n)
			} else if marker < 0xc0 {
				return nil, errors.New("unknown marker")
			} else {
				return nil, errors.New("unknown marker")
			}
		}
		if err != nil {
			return nil, err
		}
	}

	if d.nComp == 0 {
		return nil, errors.New("missing SOF marker")
	}
	if d.plane[0] == nil {
		return nil, errors.New("missing SOS marker")
	}
	return d.assembleRGBA(), nil
}
