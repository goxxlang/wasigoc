// Tiny subset of encoding/base64: standard and URL alphabets, standard
// padding only (no raw/no-padding variants).
package base64

import "errors"

const stdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
const urlAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

type Encoding struct {
	alphabet string
}

var StdEncoding = Encoding{alphabet: stdAlphabet}
var URLEncoding = Encoding{alphabet: urlAlphabet}

func (enc Encoding) EncodeToString(src []byte) string {
	var out []byte
	n := len(src)
	i := 0
	for ; i+3 <= n; i += 3 {
		b0 := src[i]
		b1 := src[i+1]
		b2 := src[i+2]
		out = append(out, enc.alphabet[b0>>2])
		out = append(out, enc.alphabet[(b0&3)<<4|b1>>4])
		out = append(out, enc.alphabet[(b1&15)<<2|b2>>6])
		out = append(out, enc.alphabet[b2&63])
	}
	rem := n - i
	if rem == 1 {
		b0 := src[i]
		out = append(out, enc.alphabet[b0>>2])
		out = append(out, enc.alphabet[(b0&3)<<4])
		out = append(out, byte(61))
		out = append(out, byte(61))
	} else if rem == 2 {
		b0 := src[i]
		b1 := src[i+1]
		out = append(out, enc.alphabet[b0>>2])
		out = append(out, enc.alphabet[(b0&3)<<4|b1>>4])
		out = append(out, enc.alphabet[(b1&15)<<2])
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
	i := 0
	for i < n {
		var vals [4]int
		count := 0
		for count < 4 && i < n {
			v, ok := enc.decodeChar(src[i])
			if !ok {
				return nil, errors.New("encoding/base64: invalid byte")
			}
			vals[count] = int(v)
			count++
			i++
		}
		if count >= 2 {
			out = append(out, byte(vals[0]<<2|vals[1]>>4))
		}
		if count >= 3 {
			out = append(out, byte((vals[1]&15)<<4|vals[2]>>2))
		}
		if count >= 4 {
			out = append(out, byte((vals[2]&3)<<6|vals[3]))
		}
	}
	return out, nil
}
