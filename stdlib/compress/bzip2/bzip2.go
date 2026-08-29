// Bounded compress/bzip2: decompression only, same as real Go's own
// package (bzip2 has never had a Go encoder in the standard library
// either). Bounded further than real Go: single-member streams only (no
// concatenated "BZh#"-prefixed continuation members after the first
// end-of-stream magic+CRC) -- an honest, narrow gap, not a glossed-over
// one, since a normal one-shot compressor call (Python's `bz2.compress`
// included) always produces exactly one member anyway. `NewReader(r)`
// reads ALL of r via `io.ReadAll` and decodes the entire stream in one
// pass at construction time, same "buffer, don't stream" bound as this
// project's `compress/flate`/`gzip`/`zlib` Readers.
//
// Ported from real Go's own algorithm (there's no RFC for bzip2; real
// Go's own package comment says the same) -- MSB-first bit reader,
// canonical-Huffman-tree-by-recursive-bisection exactly matching real
// Go's `newHuffmanTree`/`buildHuffmanNode` (this project's `slices` has
// no `SortFunc`, so the two sorts real Go's version does with
// `slices.SortFunc` are done here with a plain insertion sort instead --
// same asymptotic shape as this project's own bounded `sort` package),
// move-to-front decoding, RUNA/RUNB run-length decoding of the MTF
// zero-symbol, the "single array" inverse Burrows-Wheeler transform, and
// the initial RLE1 pass (any run of 4 equal bytes is followed by a
// repeat-count byte) -- reimplemented as one direct pass over the fully
// un-BWT'd block instead of real Go's incremental per-Read-call state
// machine, since eager whole-buffer decoding doesn't need incremental
// state. The block/file CRC-32 variant (bit-reversed shift direction
// from ordinary CRC-32) is checked, not skipped.
//
// The per-block used-symbol bitmap is a `[]byte` (0/1) rather than
// `[]bool` -- this project's `[]bool` hits `wasigo::Slice<bool>`
// backing onto `std::vector<bool>`, whose `operator[]` returns a bit-
// packed proxy instead of a real `bool&` (a real, general compiler/
// runtime gap, found here since `[]bool` is rare enough in Go that no
// earlier package exercised it; worked around here rather than changed
// in the shared `Slice<T>` template, same "avoid the gap" precedent as
// several earlier packages -- see the README tracker entry). Also
// needed `io.ErrUnexpectedEOF`, missing from this project's `io`
// package until now; added there (ordinary stdlib source, ordinary
// gap, not compiler-level).
//
// Verified against real bzip2 data produced by Python's own `bz2`
// module, not a hand-built fixture or self-consistency: (1) a small
// (2,360-byte) mixed-repetition input compressed with `bz2.compress`
// (level 9) -- this package's checked-in golden test; (2) separately, a
// larger (362,749-byte) random-word input compressed at level 1 (100k
// block size), forcing 4 real blocks with a live tree-selector switch
// partway through -- confirmed decoded byte-for-byte identical to the
// original input by this port, exercising the multi-block loop and
// per-50-symbol Huffman-tree-selector switch the small single-block
// fixture can't reach on its own (not part of the checked-in golden
// test, verified separately since embedding an 86KB fixture inline
// isn't warranted just to keep this one path covered).
package bzip2

import (
	"errors"
	"io"
)

var ErrStructural = errors.New("bzip2: invalid data")

const bzip2FileMagic = 0x425a
const bzip2BlockMagic = 0x314159265359
const bzip2FinalMagic = 0x177245385090

type bitReader struct {
	data []byte
	pos  int
	n    uint64
	bits uint
	err  error
}

func (br *bitReader) fill(want uint) {
	for br.bits < want {
		if br.pos >= len(br.data) {
			br.err = io.ErrUnexpectedEOF
			return
		}
		br.n = (br.n << 8) | uint64(br.data[br.pos])
		br.pos = br.pos + 1
		br.bits = br.bits + 8
	}
}

func (br *bitReader) ReadBits64(bits uint) uint64 {
	br.fill(bits)
	if br.err != nil {
		return 0
	}
	n := (br.n >> (br.bits - bits)) & ((1 << bits) - 1)
	br.bits = br.bits - bits
	return n
}

func (br *bitReader) ReadBits(bits uint) int {
	return int(br.ReadBits64(bits))
}

func (br *bitReader) ReadBit() bool {
	return br.ReadBits(1) != 0
}

const invalidNodeValue = 0xffff

type huffmanNode struct {
	left      uint16
	right     uint16
	leftValue uint16
	rightValue uint16
}

type huffmanTree struct {
	nodes    []huffmanNode
	nextNode int
}

func (t *huffmanTree) Decode(br *bitReader) uint16 {
	nodeIndex := uint16(0)
	for {
		node := t.nodes[nodeIndex]
		bit := 0
		if br.ReadBit() {
			bit = 1
		}
		var next uint16
		if bit == 1 {
			next = node.left
		} else {
			next = node.right
		}
		if next == invalidNodeValue {
			if bit == 1 {
				return node.leftValue
			}
			return node.rightValue
		}
		nodeIndex = next
	}
}

type huffmanCode struct {
	code    uint32
	codeLen uint8
	value   uint16
}

type symLenPair struct {
	value  uint16
	length uint8
}

func newHuffmanTree(lengths []uint8) (*huffmanTree, error) {
	if len(lengths) < 2 {
		return nil, ErrStructural
	}

	pairs := make([]symLenPair, len(lengths))
	i := 0
	for i < len(lengths) {
		pairs[i] = symLenPair{value: uint16(i), length: lengths[i]}
		i = i + 1
	}
	// Sort by (length, value) ascending -- insertion sort, since this
	// project's slices has no SortFunc.
	i = 1
	for i < len(pairs) {
		j := i
		for j > 0 {
			swap := pairs[j-1].length > pairs[j].length
			if pairs[j-1].length == pairs[j].length {
				swap = pairs[j-1].value > pairs[j].value
			}
			if !swap {
				break
			}
			pairs[j-1], pairs[j] = pairs[j], pairs[j-1]
			j = j - 1
		}
		i = i + 1
	}

	code := uint32(0)
	length := uint8(32)
	codes := make([]huffmanCode, len(lengths))
	i = len(pairs) - 1
	for i >= 0 {
		if length > pairs[i].length {
			length = pairs[i].length
		}
		codes[i] = huffmanCode{code: code, codeLen: length, value: pairs[i].value}
		code = code + (uint32(1) << (32 - uint32(length)))
		i = i - 1
	}

	// Sort by code ascending -- insertion sort again.
	i = 1
	for i < len(codes) {
		j := i
		for j > 0 && codes[j-1].code > codes[j].code {
			codes[j-1], codes[j] = codes[j], codes[j-1]
			j = j - 1
		}
		i = i + 1
	}

	t := &huffmanTree{}
	t.nodes = make([]huffmanNode, len(codes))
	_, err := buildHuffmanNode(t, codes, 0)
	return t, err
}

func buildHuffmanNode(t *huffmanTree, codes []huffmanCode, level uint32) (uint16, error) {
	test := uint32(1) << (31 - level)
	firstRightIndex := len(codes)
	i := 0
	for i < len(codes) {
		if codes[i].code&test != 0 {
			firstRightIndex = i
			break
		}
		i = i + 1
	}
	left := codes[:firstRightIndex]
	right := codes[firstRightIndex:]

	if len(left) == 0 || len(right) == 0 {
		if len(codes) < 2 {
			return 0, ErrStructural
		}
		if level == 31 {
			return 0, ErrStructural
		}
		if len(left) == 0 {
			return buildHuffmanNode(t, right, level+1)
		}
		return buildHuffmanNode(t, left, level+1)
	}

	nodeIndex := uint16(t.nextNode)
	t.nextNode = t.nextNode + 1

	var leftIdx uint16
	var rightIdx uint16
	var leftVal uint16
	var rightVal uint16
	var err error
	if len(left) == 1 {
		leftIdx = invalidNodeValue
		leftVal = left[0].value
	} else {
		leftIdx, err = buildHuffmanNode(t, left, level+1)
		if err != nil {
			return 0, err
		}
	}
	if len(right) == 1 {
		rightIdx = invalidNodeValue
		rightVal = right[0].value
	} else {
		rightIdx, err = buildHuffmanNode(t, right, level+1)
		if err != nil {
			return 0, err
		}
	}

	t.nodes[nodeIndex] = huffmanNode{left: leftIdx, right: rightIdx, leftValue: leftVal, rightValue: rightVal}
	return nodeIndex, nil
}

type mtfDecoder struct {
	list []byte
}

func newMTFDecoder(symbols []byte) *mtfDecoder {
	return &mtfDecoder{list: symbols}
}

func newMTFDecoderWithRange(n int) *mtfDecoder {
	m := make([]byte, n)
	i := 0
	for i < n {
		m[i] = byte(i)
		i = i + 1
	}
	return &mtfDecoder{list: m}
}

func (m *mtfDecoder) Decode(n int) byte {
	b := m.list[n]
	i := n
	for i > 0 {
		m.list[i] = m.list[i-1]
		i = i - 1
	}
	m.list[0] = b
	return b
}

func (m *mtfDecoder) First() byte {
	return m.list[0]
}

var crctab [256]uint32

func init() {
	poly := uint32(0x04C11DB7)
	i := 0
	for i < 256 {
		crc := uint32(i) << 24
		j := 0
		for j < 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc = crc << 1
			}
			j = j + 1
		}
		crctab[i] = crc
		i = i + 1
	}
}

func updateCRC(val uint32, b []byte) uint32 {
	crc := ^val
	i := 0
	for i < len(b) {
		crc = crctab[byte(crc>>24)^b[i]] ^ (crc << 8)
		i = i + 1
	}
	return ^crc
}

// inverseBWT implements the "single array" inverse Burrows-Wheeler
// transform: tt[i] holds the byte value in its low 8 bits and, after
// this pass, the index of the next output byte in its upper 24 bits.
// Returns the index of the first output byte.
func inverseBWT(tt []uint32, origPtr uint, c []uint) uint32 {
	sum := uint(0)
	i := 0
	for i < 256 {
		sum = sum + c[i]
		c[i] = sum - c[i]
		i = i + 1
	}
	i = 0
	for i < len(tt) {
		b := tt[i] & 0xff
		tt[c[b]] = tt[c[b]] | (uint32(i) << 8)
		c[b] = c[b] + 1
		i = i + 1
	}
	return tt[origPtr] >> 8
}

// decodeRLE1 reverses bzip2's initial run-length pass: a run of 4 equal
// bytes is always followed by a repeat-count byte giving the number of
// EXTRA copies (0-255) -- a valid encoder never emits more than 4 equal
// bytes without one.
func decodeRLE1(data []byte) []byte {
	var out []byte
	n := len(data)
	i := 0
	for i < n {
		b := data[i]
		run := 1
		for run < 4 && i+run < n && data[i+run] == b {
			run = run + 1
		}
		k := 0
		for k < run {
			out = append(out, b)
			k = k + 1
		}
		i = i + run
		if run == 4 && i < n {
			extra := int(data[i])
			i = i + 1
			k = 0
			for k < extra {
				out = append(out, b)
				k = k + 1
			}
		}
	}
	return out
}

// readBlock decodes one bzip2 block (the block magic has already been
// consumed) and returns its fully RLE1-decoded bytes plus its (already
// verified) block CRC, for the caller to fold into the running file CRC.
func readBlock(br *bitReader, blockSize int) ([]byte, uint32, error) {
	wantBlockCRC := uint32(br.ReadBits64(32))

	randomized := br.ReadBits(1)
	if randomized != 0 {
		return nil, 0, ErrStructural
	}
	origPtr := uint(br.ReadBits(24))

	symbolRangeUsedBitmap := br.ReadBits(16)
	// []byte (0/1), not []bool -- wasigo::Slice<bool> backs onto
	// std::vector<bool>, whose operator[] returns a bit-packed proxy
	// object instead of a real bool&, which doesn't satisfy this
	// project's generic Slice<T>::operator[]'s `T&` return type. []bool
	// is rare enough in Go that this is the first package to hit it;
	// working around it here (this project has plenty of other
	// "avoid the gap, don't fix the compiler for a rare case" precedent)
	// rather than changing the shared Slice<T> template for every caller.
	symbolPresent := make([]byte, 256)
	numSymbols := 0
	symRange := uint(0)
	for symRange < 16 {
		if symbolRangeUsedBitmap&(1<<(15-symRange)) != 0 {
			bits := br.ReadBits(16)
			symbol := uint(0)
			for symbol < 16 {
				if bits&(1<<(15-symbol)) != 0 {
					symbolPresent[16*symRange+symbol] = 1
					numSymbols = numSymbols + 1
				}
				symbol = symbol + 1
			}
		}
		symRange = symRange + 1
	}
	if numSymbols == 0 {
		return nil, 0, ErrStructural
	}

	numHuffmanTrees := br.ReadBits(3)
	if numHuffmanTrees < 2 || numHuffmanTrees > 6 {
		return nil, 0, ErrStructural
	}

	numSelectors := br.ReadBits(15)
	treeIndexes := make([]uint8, numSelectors)

	mtfTreeDecoder := newMTFDecoderWithRange(numHuffmanTrees)
	i := 0
	for i < numSelectors {
		c := 0
		for {
			inc := br.ReadBits(1)
			if inc == 0 {
				break
			}
			c = c + 1
		}
		if c >= numHuffmanTrees {
			return nil, 0, ErrStructural
		}
		treeIndexes[i] = mtfTreeDecoder.Decode(c)
		i = i + 1
	}

	symbols := make([]byte, numSymbols)
	nextSymbol := 0
	i = 0
	for i < 256 {
		if symbolPresent[i] != 0 {
			symbols[nextSymbol] = byte(i)
			nextSymbol = nextSymbol + 1
		}
		i = i + 1
	}
	mtf := newMTFDecoder(symbols)

	numSymbols = numSymbols + 2
	huffmanTrees := make([]*huffmanTree, numHuffmanTrees)
	lengths := make([]uint8, numSymbols)
	i = 0
	for i < numHuffmanTrees {
		length := br.ReadBits(5)
		j := 0
		for j < len(lengths) {
			for {
				if length < 1 || length > 20 {
					return nil, 0, ErrStructural
				}
				if !br.ReadBit() {
					break
				}
				if br.ReadBit() {
					length = length - 1
				} else {
					length = length + 1
				}
			}
			lengths[j] = uint8(length)
			j = j + 1
		}
		ht, err := newHuffmanTree(lengths)
		if err != nil {
			return nil, 0, err
		}
		huffmanTrees[i] = ht
		i = i + 1
	}

	if len(treeIndexes) == 0 {
		return nil, 0, ErrStructural
	}
	if int(treeIndexes[0]) >= len(huffmanTrees) {
		return nil, 0, ErrStructural
	}
	currentHuffmanTree := huffmanTrees[treeIndexes[0]]
	selectorIndex := 1

	tt := make([]uint32, blockSize)
	var c [256]uint
	bufIndex := 0
	repeat := 0
	repeatPower := 0
	decoded := 0

	for {
		if decoded == 50 {
			if selectorIndex >= numSelectors {
				return nil, 0, ErrStructural
			}
			if int(treeIndexes[selectorIndex]) >= len(huffmanTrees) {
				return nil, 0, ErrStructural
			}
			currentHuffmanTree = huffmanTrees[treeIndexes[selectorIndex]]
			selectorIndex = selectorIndex + 1
			decoded = 0
		}

		v := currentHuffmanTree.Decode(br)
		decoded = decoded + 1

		if v < 2 {
			if repeat == 0 {
				repeatPower = 1
			}
			repeat = repeat + (repeatPower << v)
			repeatPower = repeatPower << 1
			if repeat > 2*1024*1024 {
				return nil, 0, ErrStructural
			}
			continue
		}

		if repeat > 0 {
			if repeat > blockSize-bufIndex {
				return nil, 0, ErrStructural
			}
			k := 0
			for k < repeat {
				b := mtf.First()
				tt[bufIndex] = uint32(b)
				c[b] = c[b] + 1
				bufIndex = bufIndex + 1
				k = k + 1
			}
			repeat = 0
		}

		if int(v) == numSymbols-1 {
			break
		}

		b := mtf.Decode(int(v - 1))
		if bufIndex >= blockSize {
			return nil, 0, ErrStructural
		}
		tt[bufIndex] = uint32(b)
		c[b] = c[b] + 1
		bufIndex = bufIndex + 1
	}

	if origPtr >= uint(bufIndex) {
		return nil, 0, ErrStructural
	}

	preRLE := tt[:bufIndex]
	tPos := inverseBWT(preRLE, origPtr, c[:])

	bwtOut := make([]byte, bufIndex)
	pos := tPos
	i = 0
	for i < bufIndex {
		pos = preRLE[pos]
		bwtOut[i] = byte(pos)
		pos = pos >> 8
		i = i + 1
	}

	out := decodeRLE1(bwtOut)
	if updateCRC(0, out) != wantBlockCRC {
		return nil, 0, ErrStructural
	}
	return out, wantBlockCRC, nil
}

func decodeAll(data []byte) ([]byte, error) {
	br := &bitReader{data: data}
	if br.ReadBits(16) != bzip2FileMagic {
		return nil, ErrStructural
	}
	if br.ReadBits(8) != 'h' {
		return nil, ErrStructural
	}
	level := br.ReadBits(8)
	if level < '1' || level > '9' {
		return nil, ErrStructural
	}
	blockSize := 100 * 1000 * (level - '0')

	fileCRC := uint32(0)
	var out []byte
	for {
		magic := br.ReadBits64(48)
		if br.err != nil {
			return nil, br.err
		}
		if magic == bzip2FinalMagic {
			wantFileCRC := uint32(br.ReadBits64(32))
			if br.err != nil {
				return nil, br.err
			}
			if fileCRC != wantFileCRC {
				return nil, ErrStructural
			}
			break
		}
		if magic != bzip2BlockMagic {
			return nil, ErrStructural
		}
		blockBytes, blockCRC, err := readBlock(br, blockSize)
		if err != nil {
			return nil, err
		}
		out = append(out, blockBytes...)
		fileCRC = (fileCRC<<1 | fileCRC>>31) ^ blockCRC
	}
	return out, nil
}

type Reader struct {
	data []byte
	pos  int
}

func NewReader(r io.Reader) (*Reader, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	decoded, err2 := decodeAll(raw)
	if err2 != nil {
		return nil, err2
	}
	return &Reader{data: decoded}, nil
}

func (br *Reader) Read(p []byte) (int, error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n := copy(p, br.data[br.pos:])
	br.pos = br.pos + n
	return n, nil
}
