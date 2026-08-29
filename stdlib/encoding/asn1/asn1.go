// Bounded encoding/asn1: DER Marshal/Unmarshal of int64, bool, and
// []byte (INTEGER / BOOLEAN / OCTET STRING) only. No arbitrary-struct
// Unmarshal (same settable-reflect wall as encoding/json), no OID,
// no SET, no IMPLICIT tagging. A struct can be Marshaled as a SEQUENCE
// of those three scalars via reflect, write-only, same bound as
// encoding/xml.
package asn1

import (
	"errors"
	"reflect"
)

var ErrFormat = errors.New("asn1: invalid DER")

func marshalLength(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	if n < 256 {
		return []byte{0x81, byte(n)}
	}
	return []byte{0x82, byte(n >> 8), byte(n)}
}

func marshalTLV(tag byte, val []byte) []byte {
	out := []byte{tag}
	out = append(out, marshalLength(len(val))...)
	return append(out, val...)
}

func marshalInt64(n int64) []byte {
	if n < 0 {
		n = 0
	}
	if n == 0 {
		return marshalTLV(0x02, []byte{0})
	}
	var tmp []byte
	v := uint64(n)
	for v > 0 {
		tmp = append([]byte{byte(v)}, tmp...)
		v = v >> 8
	}
	if tmp[0]&0x80 != 0 {
		tmp = append([]byte{0}, tmp...)
	}
	return marshalTLV(0x02, tmp)
}

func Marshal(v any) ([]byte, error) {
	switch x := v.(type) {
	case bool:
		if x {
			return marshalTLV(0x01, []byte{0xff}), nil
		}
		return marshalTLV(0x01, []byte{0x00}), nil
	case int:
		return marshalInt64(int64(x)), nil
	case int64:
		return marshalInt64(x), nil
	case string:
		return marshalTLV(0x0c, []byte(x)), nil
	case []byte:
		return marshalTLV(0x04, x), nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct {
		var body []byte
		n := rv.NumField()
		i := 0
		for i < n {
			fb, err := Marshal(rv.Field(i).Interface())
			if err != nil {
				return nil, err
			}
			body = append(body, fb...)
			i = i + 1
		}
		return marshalTLV(0x30, body), nil
	}
	return nil, errors.New("asn1: unsupported type")
}

func Unmarshal(b []byte, val any) error {
	if len(b) < 2 {
		return ErrFormat
	}
	tag := b[0]
	ln := int(b[1])
	off := 2
	if ln >= 128 {
		return ErrFormat
	}
	if off+ln != len(b) {
		return ErrFormat
	}
	content := b[off : off+ln]
	switch v := val.(type) {
	case *int64:
		if tag != 0x02 || len(content) == 0 {
			return ErrFormat
		}
		n := int64(0)
		i := 0
		for i < len(content) {
			n = (n << 8) | int64(content[i])
			i = i + 1
		}
		*v = n
		return nil
	case *bool:
		if tag != 0x01 || len(content) != 1 {
			return ErrFormat
		}
		*v = content[0] != 0
		return nil
	case *[]byte:
		if tag != 0x04 && tag != 0x0c {
			return ErrFormat
		}
		out := append([]byte{}, content...)
		*v = out
		return nil
	}
	return errors.New("asn1: unsupported dest")
}
