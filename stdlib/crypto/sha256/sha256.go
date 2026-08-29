// SHA-256 (FIPS 180-4). Constants verified against Wikipedia's SHA-2
// pseudocode independently before writing this. One-shot Sum(data) is the
// primary entry point; Digest gives the streaming Write/Sum/Reset/Size/
// BlockSize shape, buffering a partial 64-byte block across calls and
// tracking the total length for the final (big-endian, unlike MD5's
// little-endian) length suffix.
package sha256

var initH = []uint32{
	0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
	0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
}

var kTable = []uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

func rightRotate(x uint32, c uint32) uint32 {
	return (x >> c) | (x << (32 - c))
}

func writeBE32(b []byte, off int, v uint32) {
	b[off] = byte(v >> 24)
	b[off+1] = byte(v >> 16)
	b[off+2] = byte(v >> 8)
	b[off+3] = byte(v)
}

func processBlock(chunk []byte, h []uint32) []uint32 {
	w := make([]uint32, 64)
	for i := 0; i < 16; i++ {
		w[i] = uint32(chunk[i*4])<<24 | uint32(chunk[i*4+1])<<16 | uint32(chunk[i*4+2])<<8 | uint32(chunk[i*4+3])
	}
	for i := 16; i < 64; i++ {
		s0 := rightRotate(w[i-15], 7) ^ rightRotate(w[i-15], 18) ^ (w[i-15] >> 3)
		s1 := rightRotate(w[i-2], 17) ^ rightRotate(w[i-2], 19) ^ (w[i-2] >> 10)
		w[i] = w[i-16] + s0 + w[i-7] + s1
	}

	a := h[0]
	b := h[1]
	c := h[2]
	d := h[3]
	e := h[4]
	f := h[5]
	g := h[6]
	hh := h[7]

	for i := 0; i < 64; i++ {
		s1 := rightRotate(e, 6) ^ rightRotate(e, 11) ^ rightRotate(e, 25)
		ch := (e & f) ^ (^e & g)
		temp1 := hh + s1 + ch + kTable[i] + w[i]
		s0 := rightRotate(a, 2) ^ rightRotate(a, 13) ^ rightRotate(a, 22)
		maj := (a & b) ^ (a & c) ^ (b & c)
		temp2 := s0 + maj

		hh = g
		g = f
		f = e
		e = d + temp1
		d = c
		c = b
		b = a
		a = temp1 + temp2
	}

	out := make([]uint32, 8)
	out[0] = h[0] + a
	out[1] = h[1] + b
	out[2] = h[2] + c
	out[3] = h[3] + d
	out[4] = h[4] + e
	out[5] = h[5] + f
	out[6] = h[6] + g
	out[7] = h[7] + hh
	return out
}

func padBlock(tail []byte, totalLenBits uint64) []byte {
	out := append([]byte{}, tail...)
	out = append(out, 0x80)
	for len(out)%64 != 56 {
		out = append(out, 0)
	}
	for i := 7; i >= 0; i-- {
		out = append(out, byte(totalLenBits>>(uint(i)*8)))
	}
	return out
}

func hashToBytes(h []uint32) []byte {
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		writeBE32(out, i*4, h[i])
	}
	return out
}

// Sum returns the 32-byte SHA-256 digest of data.
func Sum(data []byte) []byte {
	h := []uint32{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}

	full := len(data) / 64 * 64
	for off := 0; off < full; off += 64 {
		h = processBlock(data[off:off+64], h)
	}
	padded := padBlock(data[full:], uint64(len(data))*8)
	for off := 0; off < len(padded); off += 64 {
		h = processBlock(padded[off:off+64], h)
	}
	return hashToBytes(h)
}

type Digest struct {
	h        []uint32
	buf      []byte
	totalLen uint64
}

func New() *Digest {
	return &Digest{h: []uint32{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}}
}

func (d *Digest) Write(p []byte) (int, error) {
	d.totalLen = d.totalLen + uint64(len(p))
	d.buf = append(d.buf, p...)
	full := len(d.buf) / 64 * 64
	for off := 0; off < full; off += 64 {
		d.h = processBlock(d.buf[off:off+64], d.h)
	}
	d.buf = d.buf[full:]
	return len(p), nil
}

func (d *Digest) Sum(b []byte) []byte {
	h := []uint32{d.h[0], d.h[1], d.h[2], d.h[3], d.h[4], d.h[5], d.h[6], d.h[7]}
	padded := padBlock(d.buf, d.totalLen*8)
	for off := 0; off < len(padded); off += 64 {
		h = processBlock(padded[off:off+64], h)
	}
	return append(b, hashToBytes(h)...)
}

func (d *Digest) Reset() {
	d.h = []uint32{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}
	d.buf = nil
	d.totalLen = 0
}

func (d *Digest) Size() int      { return 32 }
func (d *Digest) BlockSize() int { return 64 }
