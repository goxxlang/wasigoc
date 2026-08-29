// Bounded subset of encoding/ascii85 (Adobe's btoa/ascii85, RFC 1924-
// adjacent): the low-level `Encode`/`Decode`/`MaxEncodedLen`/`EncodedLen`
// functions, matching real Go's own actual signatures (already []byte-in
// []byte-out, not streaming). No `NewEncoder`/`NewDecoder` (streaming
// io.Writer/io.Reader wrappers) -- callers size their own `dst` buffer and
// call Encode/Decode directly, same as real Go's low-level functions
// already require. `[N]byte` fixed-size arrays are avoided in favor of a
// `make([]byte, 5)` scratch slice, same "prefer []byte over [N]byte"
// precedent as crypto/rc4 and hash/crc32.
package ascii85

import "errors"

func isSpace(c byte) bool {
	return c == 32 || c == 9 || c == 10 || c == 13 || c == 11 || c == 12
}

func Encode(dst []byte, src []byte) int {
	n := 0
	for len(src) > 0 {
		chunkLen := 4
		if len(src) < 4 {
			chunkLen = len(src)
		}
		var v uint32
		for i := 0; i < 4; i++ {
			v = v << 8
			if i < chunkLen {
				v = v | uint32(src[i])
			}
		}
		if chunkLen == 4 && v == 0 {
			dst[n] = byte(122)
			n = n + 1
		} else {
			buf := make([]byte, 5)
			for i := 4; i >= 0; i-- {
				buf[i] = byte(v%85) + 33
				v = v / 85
			}
			for i := 0; i < chunkLen+1; i++ {
				dst[n+i] = buf[i]
			}
			n = n + chunkLen + 1
		}
		src = src[chunkLen:]
	}
	return n
}

func MaxEncodedLen(n int) int {
	return (n + 3) / 4 * 5
}

func EncodedLen(n int) int {
	return (n + 3) / 4 * 5
}

func Decode(dst []byte, src []byte, flush bool) (int, int, error) {
	ndst := 0
	nsrc := 0
	buf := make([]byte, 5)
	used := 0
	i := 0
	for i < len(src) {
		c := src[i]
		i = i + 1
		if c == 122 && used == 0 {
			dst[ndst] = 0
			dst[ndst+1] = 0
			dst[ndst+2] = 0
			dst[ndst+3] = 0
			ndst = ndst + 4
			nsrc = i
			continue
		}
		if isSpace(c) {
			nsrc = i
			continue
		}
		if c < 33 || c > 117 {
			return ndst, nsrc, errors.New("illegal ascii85 data")
		}
		buf[used] = c - 33
		used = used + 1
		if used == 5 {
			var v uint32
			for k := 0; k < 5; k++ {
				v = v*85 + uint32(buf[k])
			}
			dst[ndst] = byte(v >> 24)
			dst[ndst+1] = byte(v >> 16)
			dst[ndst+2] = byte(v >> 8)
			dst[ndst+3] = byte(v)
			ndst = ndst + 4
			used = 0
			nsrc = i
		}
	}
	if flush && used > 0 {
		for k := used; k < 5; k++ {
			buf[k] = 84
		}
		var v uint32
		for k := 0; k < 5; k++ {
			v = v*85 + uint32(buf[k])
		}
		full := make([]byte, 4)
		full[0] = byte(v >> 24)
		full[1] = byte(v >> 16)
		full[2] = byte(v >> 8)
		full[3] = byte(v)
		for k := 0; k < used-1; k++ {
			dst[ndst] = full[k]
			ndst = ndst + 1
		}
		nsrc = i
	}
	return ndst, nsrc, nil
}
