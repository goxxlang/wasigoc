// Bounded compress/flate: a real DEFLATE (RFC 1951) implementation.
// Decoder (inflate) handles all three block types -- stored, fixed Huffman,
// and dynamic Huffman -- so it can decompress real-world deflate streams
// produced by zlib/gzip/Python, not just this package's own output.
// Encoder (deflate) always emits a single final block using real greedy
// LZ77 matching (hash-bucketed, window 32768, min match 3, max match 258)
// PLUS fixed Huffman coding (RFC 1951 3.2.6) -- never dynamic Huffman --
// so compression ratio is a bit behind real Go's own flate (which prefers
// dynamic tables), but the bitstream itself is standard and any real
// decoder (zlib, gzip, this package's own Reader) reads it correctly. The
// `level` parameter of `NewWriter` is accepted (for real Go call-site
// compatibility, since gzip/zlib forward it) but has no effect -- same
// "accepted, no behavioral difference" bound as `time.Sleep` being a
// no-op. `Writer` buffers the whole input and compresses once at `Close`
// (like `encoding/csv`'s `Reader` slurping via `io.ReadAll`), not real
// streaming. Canonical Huffman code construction (`buildHuffman`) is one
// shared implementation of RFC 1951 3.2.2's algorithm, used for BOTH the
// fixed tables (built from the fixed length arrays, not hand-transcribed
// numeric codes) and dynamic per-block tables read from the stream --
// verified this reproduces the well-known fixed-Huffman code ranges
// (0-143 -> 8 bits starting at 48, 144-255 -> 9 bits starting at 400,
// 256-279 -> 7 bits starting at 0, 280-287 -> 8 bits starting at 192) by
// hand-tracing the algorithm before relying on it, not just trusting the
// general construction blind.
package flate

import (
	"errors"
	"io"
)

const NoCompression = 0
const BestSpeed = 1
const BestCompression = 9
const DefaultCompression = -1

var lengthBase = []int{3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258}
var lengthExtraBits = []int{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
var distBase = []int{1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577}
var distExtraBits = []int{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
var clOrder = []int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

type huffTree struct {
	codes   []uint32
	lengths []int
	decode  map[uint32]int
}

// buildHuffman implements RFC 1951 3.2.2 exactly: count codes per length,
// find each length's first (smallest) code, then assign consecutive codes
// to symbols in increasing symbol order within each length.
func buildHuffman(lengths []int) *huffTree {
	maxBits := 0
	for i := 0; i < len(lengths); i++ {
		if lengths[i] > maxBits {
			maxBits = lengths[i]
		}
	}
	blCount := make([]int, maxBits+1)
	for i := 0; i < len(lengths); i++ {
		if lengths[i] > 0 {
			blCount[lengths[i]] = blCount[lengths[i]] + 1
		}
	}
	nextCode := make([]int, maxBits+1)
	code := 0
	for bits := 1; bits <= maxBits; bits++ {
		code = (code + blCount[bits-1]) << 1
		nextCode[bits] = code
	}
	codes := make([]uint32, len(lengths))
	decode := make(map[uint32]int)
	for sym := 0; sym < len(lengths); sym++ {
		l := lengths[sym]
		if l != 0 {
			c := nextCode[l]
			nextCode[l] = nextCode[l] + 1
			codes[sym] = uint32(c)
			key := (uint32(l) << 16) | uint32(c)
			decode[key] = sym
		}
	}
	return &huffTree{codes: codes, lengths: lengths, decode: decode}
}

var fixedLitTree *huffTree
var fixedDistTree *huffTree

func init() {
	litLens := make([]int, 288)
	for i := 0; i < 144; i++ {
		litLens[i] = 8
	}
	for i := 144; i < 256; i++ {
		litLens[i] = 9
	}
	for i := 256; i < 280; i++ {
		litLens[i] = 7
	}
	for i := 280; i < 288; i++ {
		litLens[i] = 8
	}
	fixedLitTree = buildHuffman(litLens)

	distLens := make([]int, 30)
	for i := 0; i < 30; i++ {
		distLens[i] = 5
	}
	fixedDistTree = buildHuffman(distLens)
}

func lengthCodeIndex(length int) int {
	idx := 0
	for i := 0; i < len(lengthBase); i++ {
		if lengthBase[i] <= length {
			idx = i
		} else {
			break
		}
	}
	return idx
}

func distCodeIndex(dist int) int {
	idx := 0
	for i := 0; i < len(distBase); i++ {
		if distBase[i] <= dist {
			idx = i
		} else {
			break
		}
	}
	return idx
}

// --- bit-level writer ---
//
// DEFLATE packs plain values (block headers, extra bits, stored-block
// lengths) LSB-first into the stream, but packs a Huffman code's bits
// starting with the code's MOST significant bit -- writeBits and writeCode
// share the same one-bit-at-a-time emitBit primitive, differing only in
// which end of the value they start consuming from, so there is exactly
// one place that knows how bits pack into bytes.
type bitWriter struct {
	out      []byte
	bitBuf   uint32
	bitCount uint
}

func (bw *bitWriter) emitBit(bit uint32) {
	bw.bitBuf = bw.bitBuf | (bit << bw.bitCount)
	bw.bitCount = bw.bitCount + 1
	if bw.bitCount == 8 {
		bw.out = append(bw.out, byte(bw.bitBuf))
		bw.bitBuf = 0
		bw.bitCount = 0
	}
}

func (bw *bitWriter) writeBits(value uint32, n uint) {
	for i := uint(0); i < n; i++ {
		bw.emitBit((value >> i) & 1)
	}
}

func (bw *bitWriter) writeCode(code uint32, length int) {
	for i := length - 1; i >= 0; i-- {
		bw.emitBit((code >> uint(i)) & 1)
	}
}

func (bw *bitWriter) writeSymbol(tree *huffTree, sym int) {
	bw.writeCode(tree.codes[sym], tree.lengths[sym])
}

func (bw *bitWriter) flushByte() {
	if bw.bitCount > 0 {
		bw.out = append(bw.out, byte(bw.bitBuf))
		bw.bitBuf = 0
		bw.bitCount = 0
	}
}

func matchLength(data []byte, p int, i int) int {
	n := len(data)
	maxLen := 258
	if n-i < maxLen {
		maxLen = n - i
	}
	l := 0
	for l < maxLen && data[p+l] == data[i+l] {
		l = l + 1
	}
	return l
}

func emitMatch(bw *bitWriter, length int, dist int) {
	idx := lengthCodeIndex(length)
	bw.writeSymbol(fixedLitTree, 257+idx)
	extra := lengthExtraBits[idx]
	if extra > 0 {
		bw.writeBits(uint32(length-lengthBase[idx]), uint(extra))
	}
	dIdx := distCodeIndex(dist)
	bw.writeSymbol(fixedDistTree, dIdx)
	dExtra := distExtraBits[dIdx]
	if dExtra > 0 {
		bw.writeBits(uint32(dist-distBase[dIdx]), uint(dExtra))
	}
}

// deflate compresses data into one final fixed-Huffman block via greedy
// LZ77: a hash bucket per 3-byte prefix, searching the most recent 32
// candidates in that bucket (a bounded-effort cap, not exhaustive --
// same "insertion sort over linear-time" precedent as index/suffixarray).
func deflate(data []byte) []byte {
	bw := &bitWriter{}
	bw.writeBits(1, 1)
	bw.writeBits(1, 2)

	positions := make(map[string][]int)
	n := len(data)
	i := 0
	for i < n {
		matchLen := 0
		matchDist := 0
		if i+3 <= n {
			key := string(data[i : i+3])
			cand := positions[key]
			limit := len(cand)
			start := 0
			if limit > 32 {
				start = limit - 32
			}
			for ci := limit - 1; ci >= start; ci-- {
				p := cand[ci]
				dist := i - p
				if dist <= 0 || dist > 32768 {
					continue
				}
				l := matchLength(data, p, i)
				if l > matchLen {
					matchLen = l
					matchDist = dist
				}
			}
		}
		if matchLen >= 3 {
			end := i + matchLen
			for j := i; j < end && j+3 <= n; j++ {
				k := string(data[j : j+3])
				positions[k] = append(positions[k], j)
			}
			emitMatch(bw, matchLen, matchDist)
			i = end
		} else {
			if i+3 <= n {
				key := string(data[i : i+3])
				positions[key] = append(positions[key], i)
			}
			bw.writeSymbol(fixedLitTree, int(data[i]))
			i = i + 1
		}
	}
	bw.writeSymbol(fixedLitTree, 256)
	bw.flushByte()
	return bw.out
}

type Writer struct {
	w   io.Writer
	buf []byte
}

func NewWriter(w io.Writer, level int) (*Writer, error) {
	return &Writer{w: w}, nil
}

func (fw *Writer) Write(p []byte) (int, error) {
	fw.buf = append(fw.buf, p...)
	return len(p), nil
}

func (fw *Writer) Flush() error {
	return nil
}

func (fw *Writer) Close() error {
	compressed := deflate(fw.buf)
	_, err := fw.w.Write(compressed)
	return err
}

// --- bit-level reader ---

type bitReader struct {
	r        io.Reader
	bitBuf   uint32
	bitCount uint
	err      error
}

func (br *bitReader) readBit() uint32 {
	if br.bitCount == 0 {
		buf := make([]byte, 1)
		n, err := br.r.Read(buf)
		if n == 0 {
			if err == nil {
				err = io.EOF
			}
			br.err = err
			return 0
		}
		br.bitBuf = uint32(buf[0])
		br.bitCount = 8
	}
	bit := br.bitBuf & 1
	br.bitBuf = br.bitBuf >> 1
	br.bitCount = br.bitCount - 1
	return bit
}

func (br *bitReader) readBits(n uint) uint32 {
	var v uint32
	for i := uint(0); i < n; i++ {
		v = v | (br.readBit() << i)
	}
	return v
}

func (br *bitReader) alignByte() {
	br.bitBuf = 0
	br.bitCount = 0
}

func (br *bitReader) readSymbol(tree *huffTree) (int, error) {
	var code uint32 = 0
	for length := 1; length <= 15; length++ {
		bit := br.readBit()
		if br.err != nil {
			return 0, br.err
		}
		code = (code << 1) | bit
		key := (uint32(length) << 16) | code
		sym, ok := tree.decode[key]
		if ok {
			return sym, nil
		}
	}
	return 0, errors.New("flate: invalid huffman code")
}

func readDynamicTrees(br *bitReader) (*huffTree, *huffTree, error) {
	hlit := int(br.readBits(5)) + 257
	hdist := int(br.readBits(5)) + 1
	hclen := int(br.readBits(4)) + 4
	if br.err != nil {
		return nil, nil, br.err
	}
	clLens := make([]int, 19)
	for i := 0; i < hclen; i++ {
		v := br.readBits(3)
		clLens[clOrder[i]] = int(v)
	}
	if br.err != nil {
		return nil, nil, br.err
	}
	clTree := buildHuffman(clLens)

	total := hlit + hdist
	lens := make([]int, total)
	i := 0
	for i < total {
		sym, err := br.readSymbol(clTree)
		if err != nil {
			return nil, nil, err
		}
		if sym <= 15 {
			lens[i] = sym
			i = i + 1
		} else if sym == 16 {
			if i == 0 {
				return nil, nil, errors.New("flate: repeat code with no previous length")
			}
			repeat := int(br.readBits(2)) + 3
			prev := lens[i-1]
			k := 0
			for k < repeat && i < total {
				lens[i] = prev
				i = i + 1
				k = k + 1
			}
		} else if sym == 17 {
			repeat := int(br.readBits(3)) + 3
			k := 0
			for k < repeat && i < total {
				lens[i] = 0
				i = i + 1
				k = k + 1
			}
		} else if sym == 18 {
			repeat := int(br.readBits(7)) + 11
			k := 0
			for k < repeat && i < total {
				lens[i] = 0
				i = i + 1
				k = k + 1
			}
		} else {
			return nil, nil, errors.New("flate: invalid code-length symbol")
		}
	}
	litTree := buildHuffman(lens[0:hlit])
	distTree := buildHuffman(lens[hlit:total])
	return litTree, distTree, nil
}

func inflateBlock(br *bitReader, out []byte, litTree *huffTree, distTree *huffTree) ([]byte, error) {
	for {
		sym, err := br.readSymbol(litTree)
		if err != nil {
			return out, err
		}
		if sym < 256 {
			out = append(out, byte(sym))
			continue
		}
		if sym == 256 {
			return out, nil
		}
		idx := sym - 257
		if idx < 0 || idx >= len(lengthBase) {
			return out, errors.New("flate: invalid length code")
		}
		length := lengthBase[idx]
		if lengthExtraBits[idx] > 0 {
			extra := br.readBits(uint(lengthExtraBits[idx]))
			if br.err != nil {
				return out, br.err
			}
			length = length + int(extra)
		}
		dsym, derr := br.readSymbol(distTree)
		if derr != nil {
			return out, derr
		}
		if dsym < 0 || dsym >= len(distBase) {
			return out, errors.New("flate: invalid distance code")
		}
		dist := distBase[dsym]
		if distExtraBits[dsym] > 0 {
			dextra := br.readBits(uint(distExtraBits[dsym]))
			if br.err != nil {
				return out, br.err
			}
			dist = dist + int(dextra)
		}
		if dist > len(out) {
			return out, errors.New("flate: distance too far back")
		}
		start := len(out) - dist
		k := 0
		for k < length {
			out = append(out, out[start+k])
			k = k + 1
		}
	}
}

func inflate(r io.Reader) ([]byte, error) {
	br := &bitReader{r: r}
	var out []byte
	for {
		bfinal := br.readBits(1)
		if br.err != nil {
			if br.err == io.EOF {
				return out, nil
			}
			return out, br.err
		}
		btype := br.readBits(2)
		if br.err != nil {
			return out, br.err
		}
		if btype == 0 {
			br.alignByte()
			hdr := make([]byte, 4)
			k := 0
			for k < 4 {
				b := make([]byte, 1)
				n, err := br.r.Read(b)
				if n == 0 {
					return out, err
				}
				hdr[k] = b[0]
				k = k + 1
			}
			length := int(hdr[0]) | (int(hdr[1]) << 8)
			data := make([]byte, length)
			got := 0
			for got < length {
				b := make([]byte, 1)
				n, err := br.r.Read(b)
				if n == 0 {
					return out, err
				}
				data[got] = b[0]
				got = got + 1
			}
			out = append(out, data...)
		} else if btype == 1 {
			var derr error
			out, derr = inflateBlock(br, out, fixedLitTree, fixedDistTree)
			if derr != nil {
				return out, derr
			}
		} else if btype == 2 {
			litTree, distTree, derr := readDynamicTrees(br)
			if derr != nil {
				return out, derr
			}
			out, derr = inflateBlock(br, out, litTree, distTree)
			if derr != nil {
				return out, derr
			}
		} else {
			return out, errors.New("flate: invalid block type")
		}
		if bfinal == 1 {
			break
		}
	}
	return out, nil
}

type Reader struct {
	data []byte
	pos  int
	err  error
}

func NewReader(r io.Reader) *Reader {
	data, err := inflate(r)
	return &Reader{data: data, err: err}
}

func (fr *Reader) Read(p []byte) (int, error) {
	if fr.pos >= len(fr.data) {
		if fr.err != nil {
			return 0, fr.err
		}
		return 0, io.EOF
	}
	n := copy(p, fr.data[fr.pos:])
	fr.pos = fr.pos + n
	return n, nil
}

func (fr *Reader) Close() error {
	return nil
}
