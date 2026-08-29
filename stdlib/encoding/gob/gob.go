// Bounded encoding/gob: a tiny tagged binary for string, int64, and
// []byte only -- not real Go's gob type-graph. Decode of arbitrary
// structs is the same settable-reflect wall as encoding/json.Unmarshal.
// Encoder/Decoder wrap a []byte buffer rather than a streaming
// io.Writer/Reader (same "buffer, don't stream" bound as encoding/csv).
package gob

import "errors"

var ErrFormat = errors.New("gob: invalid message")
var ErrType = errors.New("gob: unsupported type")

const tagString byte = 1
const tagInt byte = 2
const tagBytes byte = 3

type Encoder struct {
	buf []byte
}

type Decoder struct {
	buf []byte
	off int
}

func NewEncoder() *Encoder { return &Encoder{} }

func NewDecoder(data []byte) *Decoder { return &Decoder{buf: data} }

func (e *Encoder) Bytes() []byte { return e.buf }

func putUvarint(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	return []byte{byte(128) | byte(n&127), byte(n >> 7)}
}

func (e *Encoder) Encode(v any) error {
	switch x := v.(type) {
	case string:
		b := []byte(x)
		e.buf = append(e.buf, tagString)
		e.buf = append(e.buf, putUvarint(len(b))...)
		e.buf = append(e.buf, b...)
		return nil
	case int:
		e.buf = append(e.buf, tagInt)
		e.buf = append(e.buf, putUvarint(int(x))...)
		return nil
	case int64:
		e.buf = append(e.buf, tagInt)
		e.buf = append(e.buf, putUvarint(int(x))...)
		return nil
	case []byte:
		e.buf = append(e.buf, tagBytes)
		e.buf = append(e.buf, putUvarint(len(x))...)
		e.buf = append(e.buf, x...)
		return nil
	}
	return ErrType
}

func (d *Decoder) takeUvarint() (int, error) {
	if d.off >= len(d.buf) {
		return 0, ErrFormat
	}
	b := d.buf[d.off]
	d.off = d.off + 1
	if b < 128 {
		return int(b), nil
	}
	if d.off >= len(d.buf) {
		return 0, ErrFormat
	}
	n := int(b&127) | (int(d.buf[d.off]) << 7)
	d.off = d.off + 1
	return n, nil
}

func (d *Decoder) Decode(val any) error {
	if d.off >= len(d.buf) {
		return ErrFormat
	}
	tag := d.buf[d.off]
	d.off = d.off + 1
	n, err := d.takeUvarint()
	if err != nil {
		return err
	}
	switch v := val.(type) {
	case *string:
		if tag != tagString || d.off+n > len(d.buf) {
			return ErrFormat
		}
		*v = string(d.buf[d.off : d.off+n])
		d.off = d.off + n
		return nil
	case *int:
		if tag != tagInt {
			return ErrFormat
		}
		*v = n
		return nil
	case *int64:
		if tag != tagInt {
			return ErrFormat
		}
		*v = int64(n)
		return nil
	case *[]byte:
		if tag != tagBytes || d.off+n > len(d.buf) {
			return ErrFormat
		}
		*v = append([]byte{}, d.buf[d.off:d.off+n]...)
		d.off = d.off + n
		return nil
	}
	return ErrType
}
