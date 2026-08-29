// MD5 (RFC 1321). Constants verified against the RFC's shift/init-vector
// table and Wikipedia's pseudocode K table, both independently. One-shot
// Sum(data) is the primary entry point; Digest gives the streaming
// Write/Sum/Reset/Size/BlockSize shape, buffering a partial 64-byte block
// across calls and tracking the total length for the final padding.
package md5

const (
	init0 = 0x67452301
	init1 = 0xefcdab89
	init2 = 0x98badcfe
	init3 = 0x10325476
)

var kTable = []uint32{
	0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee,
	0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
	0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be,
	0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
	0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa,
	0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
	0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed,
	0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
	0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c,
	0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
	0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05,
	0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
	0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039,
	0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
	0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1,
	0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
}

var sTable = []uint32{
	7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
	5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
	4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
	6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
}

func leftRotate(x uint32, c uint32) uint32 {
	return (x << c) | (x >> (32 - c))
}

func writeLE32(b []byte, off int, v uint32) {
	b[off] = byte(v)
	b[off+1] = byte(v >> 8)
	b[off+2] = byte(v >> 16)
	b[off+3] = byte(v >> 24)
}

// processBlock runs the 64-round compression function on one 64-byte
// chunk, folding it into the running (a0,b0,c0,d0) state.
func processBlock(chunk []byte, a0 uint32, b0 uint32, c0 uint32, d0 uint32) (uint32, uint32, uint32, uint32) {
	m := make([]uint32, 16)
	for i := 0; i < 16; i++ {
		m[i] = uint32(chunk[i*4]) | uint32(chunk[i*4+1])<<8 | uint32(chunk[i*4+2])<<16 | uint32(chunk[i*4+3])<<24
	}
	a := a0
	b := b0
	c := c0
	d := d0
	for i := 0; i < 64; i++ {
		var f uint32
		var g int
		if i < 16 {
			f = (b & c) | (^b & d)
			g = i
		} else if i < 32 {
			f = (d & b) | (^d & c)
			g = (5*i + 1) % 16
		} else if i < 48 {
			f = b ^ c ^ d
			g = (3*i + 5) % 16
		} else {
			f = c ^ (b | ^d)
			g = (7 * i) % 16
		}
		f = f + a + kTable[i] + m[g]
		a = d
		d = c
		c = b
		b = b + leftRotate(f, sTable[i])
	}
	return a0 + a, b0 + b, c0 + c, d0 + d
}

func padBlock(tail []byte, totalLenBits uint64) []byte {
	out := append([]byte{}, tail...)
	out = append(out, 0x80)
	for len(out)%64 != 56 {
		out = append(out, 0)
	}
	for i := 0; i < 8; i++ {
		out = append(out, byte(totalLenBits>>(uint(i)*8)))
	}
	return out
}

// Sum returns the 16-byte MD5 digest of data.
func Sum(data []byte) []byte {
	a0 := uint32(init0)
	b0 := uint32(init1)
	c0 := uint32(init2)
	d0 := uint32(init3)

	full := len(data) / 64 * 64
	for off := 0; off < full; off += 64 {
		a0, b0, c0, d0 = processBlock(data[off:off+64], a0, b0, c0, d0)
	}
	padded := padBlock(data[full:], uint64(len(data))*8)
	for off := 0; off < len(padded); off += 64 {
		a0, b0, c0, d0 = processBlock(padded[off:off+64], a0, b0, c0, d0)
	}

	out := make([]byte, 16)
	writeLE32(out, 0, a0)
	writeLE32(out, 4, b0)
	writeLE32(out, 8, c0)
	writeLE32(out, 12, d0)
	return out
}

type Digest struct {
	a0       uint32
	b0       uint32
	c0       uint32
	d0       uint32
	buf      []byte
	totalLen uint64
}

func New() *Digest {
	return &Digest{a0: init0, b0: init1, c0: init2, d0: init3}
}

func (d *Digest) Write(p []byte) (int, error) {
	d.totalLen = d.totalLen + uint64(len(p))
	d.buf = append(d.buf, p...)
	full := len(d.buf) / 64 * 64
	for off := 0; off < full; off += 64 {
		d.a0, d.b0, d.c0, d.d0 = processBlock(d.buf[off:off+64], d.a0, d.b0, d.c0, d.d0)
	}
	d.buf = d.buf[full:]
	return len(p), nil
}

func (d *Digest) Sum(b []byte) []byte {
	a0 := d.a0
	b0 := d.b0
	c0 := d.c0
	d0 := d.d0
	padded := padBlock(d.buf, d.totalLen*8)
	for off := 0; off < len(padded); off += 64 {
		a0, b0, c0, d0 = processBlock(padded[off:off+64], a0, b0, c0, d0)
	}
	out := make([]byte, 16)
	writeLE32(out, 0, a0)
	writeLE32(out, 4, b0)
	writeLE32(out, 8, c0)
	writeLE32(out, 12, d0)
	return append(b, out...)
}

func (d *Digest) Reset() {
	d.a0 = init0
	d.b0 = init1
	d.c0 = init2
	d.d0 = init3
	d.buf = nil
	d.totalLen = 0
}

func (d *Digest) Size() int      { return 16 }
func (d *Digest) BlockSize() int { return 64 }
