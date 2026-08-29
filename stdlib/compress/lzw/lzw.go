// Bounded compress/lzw: the classic LZW algorithm (variable code width,
// clear/EOF control codes, growing up to 12-bit codes) real Go's own
// package doc says GIF needs, but NOT the GIF-specific "early change"
// code-width-bump-one-code-early quirk -- real Go's own package doesn't
// implement that either (its own doc says so), so this isn't a deviation
// from real Go, just the same documented boundary. `NewReader`/`NewWriter`
// match real Go's actual signature (`io.Reader`/`io.Writer` + `Order` +
// litWidth), not a bounded simplification -- LZW is pure byte-stream
// compression, no arbitrary-type marshaling, so there's no reflection
// wall here unlike encoding/gob or encoding/asn1. `Writer` is real
// streaming (one byte at a time through the incremental-parse dictionary,
// not slurp-then-process). Verified by round-trip (encode then decode
// recovers the original bytes exactly) rather than byte-for-byte
// comparison against a real GIF/TIFF file, since real Go's own docs
// don't promise interop with either format's LZW variant precisely.
package lzw

import (
	"errors"
	"io"
)

type Order int

const (
	LSB Order = iota
	MSB
)

const maxWidth = 12
const maxCode = 1<<maxWidth - 1

type Writer struct {
	w         io.Writer
	order     Order
	litWidth  uint
	clearCode uint32
	eofCode   uint32
	nextCode  uint32
	codeWidth uint
	dict      map[string]uint32
	prefix    []byte
	bitBuf    uint64
	bitCount  uint
	started   bool
}

func NewWriter(w io.Writer, order Order, litWidth int) *Writer {
	lw := uint(litWidth)
	clear := uint32(1) << lw
	return &Writer{
		w:         w,
		order:     order,
		litWidth:  lw,
		clearCode: clear,
		eofCode:   clear + 1,
		nextCode:  clear + 2,
		codeWidth: lw + 1,
		dict:      make(map[string]uint32),
	}
}

func (w *Writer) emit(code uint32) {
	if w.order == LSB {
		w.bitBuf = w.bitBuf | (uint64(code) << w.bitCount)
		w.bitCount = w.bitCount + w.codeWidth
		for w.bitCount >= 8 {
			w.w.Write([]byte{byte(w.bitBuf)})
			w.bitBuf = w.bitBuf >> 8
			w.bitCount = w.bitCount - 8
		}
		return
	}
	w.bitBuf = (w.bitBuf << w.codeWidth) | uint64(code)
	w.bitCount = w.bitCount + w.codeWidth
	for w.bitCount >= 8 {
		shift := w.bitCount - 8
		w.w.Write([]byte{byte(w.bitBuf >> shift)})
		w.bitCount = w.bitCount - 8
	}
	w.bitBuf = w.bitBuf & ((uint64(1) << w.bitCount) - 1)
}

func (w *Writer) resetDict() {
	w.dict = make(map[string]uint32)
	w.nextCode = w.eofCode + 1
	w.codeWidth = w.litWidth + 1
}

func (w *Writer) codeFor(prefix []byte) uint32 {
	if len(prefix) == 1 {
		return uint32(prefix[0])
	}
	return w.dict[string(prefix)]
}

func (w *Writer) Write(p []byte) (int, error) {
	if !w.started {
		w.emit(w.clearCode)
		w.started = true
	}
	for i := 0; i < len(p); i++ {
		b := p[i]
		if len(w.prefix) == 0 {
			w.prefix = []byte{b}
			continue
		}
		candidate := append(append([]byte{}, w.prefix...), b)
		if _, ok := w.dict[string(candidate)]; ok {
			w.prefix = candidate
			continue
		}
		w.emit(w.codeFor(w.prefix))
		if w.nextCode <= maxCode {
			w.dict[string(candidate)] = w.nextCode
			w.nextCode = w.nextCode + 1
			if w.nextCode == (uint32(1)<<w.codeWidth) && w.codeWidth < maxWidth {
				w.codeWidth = w.codeWidth + 1
			}
			if w.nextCode > maxCode {
				w.emit(w.clearCode)
				w.resetDict()
			}
		}
		w.prefix = []byte{b}
	}
	return len(p), nil
}

func (w *Writer) Close() error {
	if !w.started {
		w.emit(w.clearCode)
		w.started = true
	}
	if len(w.prefix) > 0 {
		w.emit(w.codeFor(w.prefix))
		w.prefix = nil
	}
	w.emit(w.eofCode)
	if w.bitCount > 0 {
		if w.order == LSB {
			w.w.Write([]byte{byte(w.bitBuf)})
		} else {
			w.w.Write([]byte{byte(w.bitBuf << (8 - w.bitCount))})
		}
		w.bitBuf = 0
		w.bitCount = 0
	}
	return nil
}

type Reader struct {
	r         io.Reader
	order     Order
	litWidth  uint
	clearCode uint32
	eofCode   uint32
	nextCode  uint32
	codeWidth uint
	dict      [][]byte
	prev      []byte
	out       []byte
	done      bool
	bitBuf    uint64
	bitCount  uint
}

func NewReader(r io.Reader, order Order, litWidth int) *Reader {
	lw := uint(litWidth)
	clear := uint32(1) << lw
	rd := &Reader{r: r, order: order, litWidth: lw, clearCode: clear, eofCode: clear + 1}
	rd.resetDict()
	return rd
}

func (r *Reader) resetDict() {
	dict := make([][]byte, r.eofCode+1)
	for i := uint32(0); i < r.clearCode; i++ {
		dict[i] = []byte{byte(i)}
	}
	r.dict = dict
	r.nextCode = r.eofCode + 1
	r.codeWidth = r.litWidth + 1
	r.prev = nil
}

func (r *Reader) readCode() (uint32, error) {
	buf := make([]byte, 1)
	for r.bitCount < r.codeWidth {
		n, err := r.r.Read(buf)
		if n == 0 {
			if err != nil {
				return 0, err
			}
			return 0, errors.New("lzw: unexpected EOF")
		}
		if r.order == LSB {
			r.bitBuf = r.bitBuf | (uint64(buf[0]) << r.bitCount)
		} else {
			r.bitBuf = (r.bitBuf << 8) | uint64(buf[0])
		}
		r.bitCount = r.bitCount + 8
	}
	var code uint32
	if r.order == LSB {
		code = uint32(r.bitBuf & ((uint64(1) << r.codeWidth) - 1))
		r.bitBuf = r.bitBuf >> r.codeWidth
		r.bitCount = r.bitCount - r.codeWidth
	} else {
		shift := r.bitCount - r.codeWidth
		code = uint32((r.bitBuf >> shift) & ((uint64(1) << r.codeWidth) - 1))
		r.bitCount = r.bitCount - r.codeWidth
		r.bitBuf = r.bitBuf & ((uint64(1) << r.bitCount) - 1)
	}
	return code, nil
}

func (r *Reader) decodeNext() error {
	code, err := r.readCode()
	if err != nil {
		return err
	}
	if code == r.clearCode {
		r.resetDict()
		return r.decodeNext()
	}
	if code == r.eofCode {
		r.done = true
		return io.EOF
	}
	var entry []byte
	if int(code) < len(r.dict) && r.dict[code] != nil {
		entry = r.dict[code]
	} else if code == r.nextCode && r.prev != nil {
		entry = append(append([]byte{}, r.prev...), r.prev[0])
	} else {
		return errors.New("lzw: invalid code")
	}
	if r.prev != nil {
		newEntry := append(append([]byte{}, r.prev...), entry[0])
		if int(r.nextCode) >= len(r.dict) {
			r.dict = append(r.dict, newEntry)
		} else {
			r.dict[r.nextCode] = newEntry
		}
		r.nextCode = r.nextCode + 1
		// The decoder always adds one dictionary entry fewer than the
		// encoder at any given code position (it skips the add for the
		// very first code after start/clear, since there's no `prev` yet
		// to combine -- the standard, correct shape of an LZW decoder,
		// not a bug), so its own nextCode counter is permanently one
		// position "behind" the encoder's at the same point in the code
		// stream. Bumping on the same threshold (`nextCode ==
		// 1<<codeWidth`) the encoder uses therefore widens the decoder's
		// read width one code too LATE, corrupting every code from that
		// point on. Bumping one threshold earlier (`nextCode ==
		// (1<<codeWidth)-1`) compensates for exactly that one-position
		// lag and keeps both sides reading/writing the same code at the
		// same width -- found by comparing an emit-by-emit encoder trace
		// against a read-by-read decoder trace side by side.
		if r.nextCode == (uint32(1)<<r.codeWidth)-1 && r.codeWidth < maxWidth {
			r.codeWidth = r.codeWidth + 1
		}
	}
	r.prev = entry
	r.out = append(r.out, entry...)
	return nil
}

func (r *Reader) Read(p []byte) (int, error) {
	for len(r.out) == 0 && !r.done {
		err := r.decodeNext()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}
	}
	if len(r.out) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	return n, nil
}
