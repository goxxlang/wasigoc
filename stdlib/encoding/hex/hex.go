// Tiny subset of encoding/hex.
package hex

import "errors"

const digits = "0123456789abcdef"

func EncodedLen(n int) int {
	return n * 2
}

func Encode(dst []byte, src []byte) int {
	for i := 0; i < len(src); i++ {
		v := src[i]
		dst[i*2] = digits[v>>4]
		dst[i*2+1] = digits[v&15]
	}
	return len(src) * 2
}

func EncodeToString(src []byte) string {
	dst := make([]byte, len(src)*2)
	Encode(dst, src)
	return string(dst)
}

func decodeNibble(c byte) (byte, bool) {
	if c >= 48 && c <= 57 {
		return byte(c - 48), true
	}
	if c >= 97 && c <= 102 {
		return byte(c - 97 + 10), true
	}
	if c >= 65 && c <= 70 {
		return byte(c - 65 + 10), true
	}
	return 0, false
}

func DecodedLen(n int) int {
	return n / 2
}

func Decode(dst []byte, src []byte) (int, error) {
	if len(src)%2 != 0 {
		return 0, errors.New("encoding/hex: odd length hex string")
	}
	for i := 0; i < len(src)/2; i++ {
		hi, ok1 := decodeNibble(src[i*2])
		lo, ok2 := decodeNibble(src[i*2+1])
		if !ok1 || !ok2 {
			return i, errors.New("encoding/hex: invalid byte")
		}
		dst[i] = hi<<4 | lo
	}
	return len(src) / 2, nil
}

func DecodeString(s string) ([]byte, error) {
	src := []byte(s)
	dst := make([]byte, len(src)/2)
	n, err := Decode(dst, src)
	if err != nil {
		return nil, err
	}
	return dst[0:n], nil
}
