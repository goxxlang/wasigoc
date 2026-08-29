// Bounded subset of mime/quotedprintable (RFC 2045 section 6.7).
//
// Writer: real streaming semantics, one byte of lookahead buffered only
// for pending trailing whitespace (space/tab immediately before a line
// break must be `=XX`-encoded, everywhere else literal) -- soft line
// breaks (`=\r\n`) inserted at 75 columns.
//
// Reader: bounded like encoding/csv's Reader -- decodes the whole input
// up front (via io.ReadAll) at NewReader time rather than truly streaming,
// since there's no need to support unbounded/infinite input here. Malformed
// `=XX` escapes are passed through literally instead of erroring (lenient,
// not real Go's stricter behavior).
package quotedprintable

import "io"

const hexDigits = "0123456789ABCDEF"

func isQPLiteral(b byte) bool {
	return b >= 33 && b <= 126 && b != 61
}

type Writer struct {
	w         io.Writer
	pendingWS []byte
	lineLen   int
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) softBreakIfNeeded(width int) {
	if w.lineLen+width > 76 {
		w.w.Write([]byte("=\r\n"))
		w.lineLen = 0
	}
}

func (w *Writer) emitLiteral(b byte) {
	w.softBreakIfNeeded(1)
	w.w.Write([]byte{b})
	w.lineLen++
}

func (w *Writer) emitEncoded(b byte) {
	w.softBreakIfNeeded(3)
	w.w.Write([]byte{61, hexDigits[b>>4], hexDigits[b&15]})
	w.lineLen += 3
}

func (w *Writer) flushPendingAsEncoded() {
	for i := 0; i < len(w.pendingWS); i++ {
		w.emitEncoded(w.pendingWS[i])
	}
	w.pendingWS = nil
}

func (w *Writer) flushPendingAsLiteral() {
	for i := 0; i < len(w.pendingWS); i++ {
		w.emitLiteral(w.pendingWS[i])
	}
	w.pendingWS = nil
}

func (w *Writer) Write(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		b := p[i]
		if b == 10 {
			w.flushPendingAsEncoded()
			w.w.Write([]byte("\r\n"))
			w.lineLen = 0
			continue
		}
		if b == 13 {
			continue
		}
		if b == 32 || b == 9 {
			w.pendingWS = append(w.pendingWS, b)
			continue
		}
		w.flushPendingAsLiteral()
		if isQPLiteral(b) {
			w.emitLiteral(b)
		} else {
			w.emitEncoded(b)
		}
	}
	return len(p), nil
}

func (w *Writer) Close() error {
	w.flushPendingAsEncoded()
	return nil
}

func isHexDigit(b byte) bool {
	return (b >= 48 && b <= 57) || (b >= 65 && b <= 70) || (b >= 97 && b <= 102)
}

func hexVal(b byte) byte {
	if b >= 48 && b <= 57 {
		return b - 48
	}
	if b >= 65 && b <= 70 {
		return b - 65 + 10
	}
	return b - 97 + 10
}

func decodeAll(raw []byte) []byte {
	var out []byte
	n := len(raw)
	i := 0
	for i < n {
		b := raw[i]
		if b == 61 {
			if i+2 < n && raw[i+1] == 13 && raw[i+2] == 10 {
				i += 3
				continue
			}
			if i+1 < n && raw[i+1] == 10 {
				i += 2
				continue
			}
			if i+2 < n && isHexDigit(raw[i+1]) && isHexDigit(raw[i+2]) {
				out = append(out, hexVal(raw[i+1])<<4|hexVal(raw[i+2]))
				i += 3
				continue
			}
			out = append(out, b)
			i++
			continue
		}
		out = append(out, b)
		i++
	}
	return out
}

type Reader struct {
	out []byte
	pos int
}

func NewReader(r io.Reader) *Reader {
	raw, _ := io.ReadAll(r)
	return &Reader{out: decodeAll(raw)}
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.pos >= len(r.out) {
		return 0, io.EOF
	}
	n := copy(p, r.out[r.pos:])
	r.pos += n
	return n, nil
}
