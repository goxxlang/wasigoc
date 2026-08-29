// SHA-1 (FIPS 180-4). Cryptographically broken (collision-findable) --
// present for legacy interop (git, old TLS, etc.), never for anything
// where collision resistance matters. Same shape as crypto/md5 and
// crypto/sha256: one-shot Sum(data), plus a streaming Digest.
package sha1

var initH = []uint32{
	0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476, 0xc3d2e1f0,
}

func leftRotate(x uint32, c uint32) uint32 {
	return (x << c) | (x >> (32 - c))
}

func writeBE32(b []byte, off int, v uint32) {
	b[off] = byte(v >> 24)
	b[off+1] = byte(v >> 16)
	b[off+2] = byte(v >> 8)
	b[off+3] = byte(v)
}

func processBlock(chunk []byte, h []uint32) []uint32 {
	w := make([]uint32, 80)
	for i := 0; i < 16; i++ {
		w[i] = uint32(chunk[i*4])<<24 | uint32(chunk[i*4+1])<<16 | uint32(chunk[i*4+2])<<8 | uint32(chunk[i*4+3])
	}
	for i := 16; i < 80; i++ {
		w[i] = leftRotate(w[i-3]^w[i-8]^w[i-14]^w[i-16], 1)
	}

	a := h[0]
	b := h[1]
	c := h[2]
	d := h[3]
	e := h[4]

	for i := 0; i < 80; i++ {
		var f uint32
		var k uint32
		if i < 20 {
			f = (b & c) | (^b & d)
			k = 0x5a827999
		} else if i < 40 {
			f = b ^ c ^ d
			k = 0x6ed9eba1
		} else if i < 60 {
			f = (b & c) | (b & d) | (c & d)
			k = 0x8f1bbcdc
		} else {
			f = b ^ c ^ d
			k = 0xca62c1d6
		}
		temp := leftRotate(a, 5) + f + e + k + w[i]
		e = d
		d = c
		c = leftRotate(b, 30)
		b = a
		a = temp
	}

	out := make([]uint32, 5)
	out[0] = h[0] + a
	out[1] = h[1] + b
	out[2] = h[2] + c
	out[3] = h[3] + d
	out[4] = h[4] + e
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
	out := make([]byte, 20)
	for i := 0; i < 5; i++ {
		writeBE32(out, i*4, h[i])
	}
	return out
}

// Sum returns the 20-byte SHA-1 digest of data.
func Sum(data []byte) []byte {
	h := []uint32{initH[0], initH[1], initH[2], initH[3], initH[4]}

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
	return &Digest{h: []uint32{initH[0], initH[1], initH[2], initH[3], initH[4]}}
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
	h := []uint32{d.h[0], d.h[1], d.h[2], d.h[3], d.h[4]}
	padded := padBlock(d.buf, d.totalLen*8)
	for off := 0; off < len(padded); off += 64 {
		h = processBlock(padded[off:off+64], h)
	}
	return append(b, hashToBytes(h)...)
}

func (d *Digest) Reset() {
	d.h = []uint32{initH[0], initH[1], initH[2], initH[3], initH[4]}
	d.buf = nil
	d.totalLen = 0
}

func (d *Digest) Size() int      { return 20 }
func (d *Digest) BlockSize() int { return 64 }
