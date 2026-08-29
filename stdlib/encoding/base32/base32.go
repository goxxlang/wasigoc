// RFC 4648 base32: standard and hex alphabets, standard padding only (no
// raw/no-padding variants) -- same bounded shape as this project's
// encoding/base64. Base32 is 5-bits-per-symbol (not base64's byte-aligned
// 6-bits-from-3-bytes), so encode/decode work off an explicit bit buffer
// instead of base64's per-remainder-count special cases -- simpler to get
// right for a non-byte-aligned symbol size.
package base32

import "errors"

const stdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
const hexAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUV"

type Encoding struct {
	alphabet string
}

var StdEncoding = Encoding{alphabet: stdAlphabet}
var HexEncoding = Encoding{alphabet: hexAlphabet}

func (enc Encoding) EncodeToString(src []byte) string {
	var out []byte
	var bitBuf uint32
	bitCount := 0
	for i := 0; i < len(src); i++ {
		bitBuf = (bitBuf << 8) | uint32(src[i])
		bitCount = bitCount + 8
		for bitCount >= 5 {
			idx := (bitBuf >> uint(bitCount-5)) & 31
			out = append(out, enc.alphabet[idx])
			bitCount = bitCount - 5
		}
	}
	if bitCount > 0 {
		idx := (bitBuf << uint(5-bitCount)) & 31
		out = append(out, enc.alphabet[idx])
	}
	for len(out)%8 != 0 {
		out = append(out, byte(61))
	}
	return string(out)
}

func (enc Encoding) decodeChar(c byte) (byte, bool) {
	for i := 0; i < len(enc.alphabet); i++ {
		if enc.alphabet[i] == c {
			return byte(i), true
		}
	}
	return 0, false
}

func (enc Encoding) DecodeString(s string) ([]byte, error) {
	src := []byte(s)
	n := len(src)
	for n > 0 && src[n-1] == 61 {
		n--
	}
	var out []byte
	var bitBuf uint32
	bitCount := 0
	for i := 0; i < n; i++ {
		v, ok := enc.decodeChar(src[i])
		if !ok {
			return nil, errors.New("encoding/base32: invalid byte")
		}
		bitBuf = (bitBuf << 5) | uint32(v)
		bitCount = bitCount + 5
		if bitCount >= 8 {
			b := byte((bitBuf >> uint(bitCount-8)) & 255)
			out = append(out, b)
			bitCount = bitCount - 8
		}
	}
	return out, nil
}
