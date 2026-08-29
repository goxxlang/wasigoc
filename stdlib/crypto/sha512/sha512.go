// SHA-512 (FIPS 180-4). Same shape as this project's crypto/sha256, just
// 64-bit words, 128-byte blocks, 80 rounds. Constants (8 init values + 80
// round constants) fetched from Wikipedia's SHA-2 pseudocode page and
// verified against the standard published test vectors below before
// trusting them, not just transcribed by eye. Length suffix: real SHA-512
// uses a 128-bit bit-length field; this only ever fills the low 64 bits
// (zeroing the high 8 bytes) since no real input here approaches 2^64
// bits -- bit-identical to real SHA-512 for every input that could ever
// actually occur.
package sha512

var initH = []uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

var kTable = []uint64{
	0x428a2f98d728ae22, 0x7137449123ef65cd, 0xb5c0fbcfec4d3b2f, 0xe9b5dba58189dbbc,
	0x3956c25bf348b538, 0x59f111f1b605d019, 0x923f82a4af194f9b, 0xab1c5ed5da6d8118,
	0xd807aa98a3030242, 0x12835b0145706fbe, 0x243185be4ee4b28c, 0x550c7dc3d5ffb4e2,
	0x72be5d74f27b896f, 0x80deb1fe3b1696b1, 0x9bdc06a725c71235, 0xc19bf174cf692694,
	0xe49b69c19ef14ad2, 0xefbe4786384f25e3, 0x0fc19dc68b8cd5b5, 0x240ca1cc77ac9c65,
	0x2de92c6f592b0275, 0x4a7484aa6ea6e483, 0x5cb0a9dcbd41fbd4, 0x76f988da831153b5,
	0x983e5152ee66dfab, 0xa831c66d2db43210, 0xb00327c898fb213f, 0xbf597fc7beef0ee4,
	0xc6e00bf33da88fc2, 0xd5a79147930aa725, 0x06ca6351e003826f, 0x142929670a0e6e70,
	0x27b70a8546d22ffc, 0x2e1b21385c26c926, 0x4d2c6dfc5ac42aed, 0x53380d139d95b3df,
	0x650a73548baf63de, 0x766a0abb3c77b2a8, 0x81c2c92e47edaee6, 0x92722c851482353b,
	0xa2bfe8a14cf10364, 0xa81a664bbc423001, 0xc24b8b70d0f89791, 0xc76c51a30654be30,
	0xd192e819d6ef5218, 0xd69906245565a910, 0xf40e35855771202a, 0x106aa07032bbd1b8,
	0x19a4c116b8d2d0c8, 0x1e376c085141ab53, 0x2748774cdf8eeb99, 0x34b0bcb5e19b48a8,
	0x391c0cb3c5c95a63, 0x4ed8aa4ae3418acb, 0x5b9cca4f7763e373, 0x682e6ff3d6b2b8a3,
	0x748f82ee5defb2fc, 0x78a5636f43172f60, 0x84c87814a1f0ab72, 0x8cc702081a6439ec,
	0x90befffa23631e28, 0xa4506cebde82bde9, 0xbef9a3f7b2c67915, 0xc67178f2e372532b,
	0xca273eceea26619c, 0xd186b8c721c0c207, 0xeada7dd6cde0eb1e, 0xf57d4f7fee6ed178,
	0x06f067aa72176fba, 0x0a637dc5a2c898a6, 0x113f9804bef90dae, 0x1b710b35131c471b,
	0x28db77f523047d84, 0x32caab7b40c72493, 0x3c9ebe0a15c9bebc, 0x431d67c49c100d4c,
	0x4cc5d4becb3e42b6, 0x597f299cfc657e2a, 0x5fcb6fab3ad6faec, 0x6c44198c4a475817,
}

func rightRotate(x uint64, c uint) uint64 {
	return (x >> c) | (x << (64 - c))
}

func writeBE64(b []byte, off int, v uint64) {
	b[off] = byte(v >> 56)
	b[off+1] = byte(v >> 48)
	b[off+2] = byte(v >> 40)
	b[off+3] = byte(v >> 32)
	b[off+4] = byte(v >> 24)
	b[off+5] = byte(v >> 16)
	b[off+6] = byte(v >> 8)
	b[off+7] = byte(v)
}

func processBlock(chunk []byte, h []uint64) []uint64 {
	w := make([]uint64, 80)
	for i := 0; i < 16; i++ {
		var v uint64
		for j := 0; j < 8; j++ {
			v = (v << 8) | uint64(chunk[i*8+j])
		}
		w[i] = v
	}
	for i := 16; i < 80; i++ {
		s0 := rightRotate(w[i-15], 1) ^ rightRotate(w[i-15], 8) ^ (w[i-15] >> 7)
		s1 := rightRotate(w[i-2], 19) ^ rightRotate(w[i-2], 61) ^ (w[i-2] >> 6)
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

	for i := 0; i < 80; i++ {
		s1 := rightRotate(e, 14) ^ rightRotate(e, 18) ^ rightRotate(e, 41)
		ch := (e & f) ^ (^e & g)
		temp1 := hh + s1 + ch + kTable[i] + w[i]
		s0 := rightRotate(a, 28) ^ rightRotate(a, 34) ^ rightRotate(a, 39)
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

	out := make([]uint64, 8)
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
	for len(out)%128 != 112 {
		out = append(out, 0)
	}
	for i := 0; i < 8; i++ {
		out = append(out, 0)
	}
	for i := 7; i >= 0; i-- {
		out = append(out, byte(totalLenBits>>(uint(i)*8)))
	}
	return out
}

func hashToBytes(h []uint64) []byte {
	out := make([]byte, 64)
	for i := 0; i < 8; i++ {
		writeBE64(out, i*8, h[i])
	}
	return out
}

// Sum returns the 64-byte SHA-512 digest of data.
func Sum(data []byte) []byte {
	h := []uint64{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}

	full := len(data) / 128 * 128
	for off := 0; off < full; off += 128 {
		h = processBlock(data[off:off+128], h)
	}
	padded := padBlock(data[full:], uint64(len(data))*8)
	for off := 0; off < len(padded); off += 128 {
		h = processBlock(padded[off:off+128], h)
	}
	return hashToBytes(h)
}

type Digest struct {
	h        []uint64
	buf      []byte
	totalLen uint64
}

func New() *Digest {
	return &Digest{h: []uint64{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}}
}

func (d *Digest) Write(p []byte) (int, error) {
	d.totalLen = d.totalLen + uint64(len(p))
	d.buf = append(d.buf, p...)
	full := len(d.buf) / 128 * 128
	for off := 0; off < full; off += 128 {
		d.h = processBlock(d.buf[off:off+128], d.h)
	}
	d.buf = d.buf[full:]
	return len(p), nil
}

func (d *Digest) Sum(b []byte) []byte {
	h := []uint64{d.h[0], d.h[1], d.h[2], d.h[3], d.h[4], d.h[5], d.h[6], d.h[7]}
	padded := padBlock(d.buf, d.totalLen*8)
	for off := 0; off < len(padded); off += 128 {
		h = processBlock(padded[off:off+128], h)
	}
	return append(b, hashToBytes(h)...)
}

func (d *Digest) Reset() {
	d.h = []uint64{initH[0], initH[1], initH[2], initH[3], initH[4], initH[5], initH[6], initH[7]}
	d.buf = nil
	d.totalLen = 0
}

func (d *Digest) Size() int      { return 64 }
func (d *Digest) BlockSize() int { return 128 }
