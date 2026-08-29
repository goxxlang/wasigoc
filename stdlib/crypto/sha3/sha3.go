// SHA3-256 (FIPS 202 / Keccak sponge). SHA3-224/384/512 and SHAKE are
// not implemented -- one rate/output size, the same "one concrete
// function" bound as crypto/aes being AES-128 only. Domain suffix 0x06
// (SHA3), not SHAKE's 0x1f. Verified against the standard empty-string
// and "abc" FIPS 202 test vectors.
package sha3

var rc = []uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808a, 0x8000000080008000,
	0x000000000000808b, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008a, 0x0000000000000088, 0x0000000080008009, 0x000000008000000a,
	0x000000008000808b, 0x800000000000008b, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800a, 0x800000008000000a,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

var rho = []uint{
	0, 1, 62, 28, 27,
	36, 44, 6, 55, 20,
	3, 10, 43, 25, 39,
	41, 45, 15, 21, 8,
	18, 2, 61, 56, 14,
}

func rotl64(x uint64, n uint) uint64 {
	if n == 0 {
		return x
	}
	return (x << n) | (x >> (64 - n))
}

func keccakF(a []uint64) {
	var b []uint64
	b = make([]uint64, 25)
	var c []uint64
	c = make([]uint64, 5)
	var d []uint64
	d = make([]uint64, 5)
	round := 0
	for round < 24 {
		x := 0
		for x < 5 {
			c[x] = a[x] ^ a[x+5] ^ a[x+10] ^ a[x+15] ^ a[x+20]
			x = x + 1
		}
		x = 0
		for x < 5 {
			d[x] = c[(x+4)%5] ^ rotl64(c[(x+1)%5], 1)
			x = x + 1
		}
		i := 0
		for i < 25 {
			a[i] = a[i] ^ d[i%5]
			i = i + 1
		}
		x = 0
		for x < 5 {
			y := 0
			for y < 5 {
				b[y+5*((2*x+3*y)%5)] = rotl64(a[x+5*y], rho[x+5*y])
				y = y + 1
			}
			x = x + 1
		}
		x = 0
		for x < 5 {
			y := 0
			for y < 5 {
				a[x+5*y] = b[x+5*y] ^ ((^b[(x+1)%5+5*y]) & b[(x+2)%5+5*y])
				y = y + 1
			}
			x = x + 1
		}
		a[0] = a[0] ^ rc[round]
		round = round + 1
	}
}

func xorBytes(a []uint64, p []byte) {
	i := 0
	for i < len(p) {
		lane := i / 8
		shift := uint((i % 8) * 8)
		a[lane] = a[lane] ^ (uint64(p[i]) << shift)
		i = i + 1
	}
}

func squeeze(a []uint64, n int) []byte {
	out := make([]byte, n)
	i := 0
	for i < n {
		lane := a[i/8]
		out[i] = byte(lane >> uint((i%8)*8))
		i = i + 1
	}
	return out
}

const rate256 = 136

func Sum256(data []byte) []byte {
	a := make([]uint64, 25)
	off := 0
	for off+rate256 <= len(data) {
		xorBytes(a, data[off:off+rate256])
		keccakF(a)
		off = off + rate256
	}
	block := make([]byte, rate256)
	rest := data[off:]
	copy(block, rest)
	block[len(rest)] = 0x06
	block[rate256-1] = block[rate256-1] | 0x80
	xorBytes(a, block)
	keccakF(a)
	return squeeze(a, 32)
}

type Digest struct {
	a   []uint64
	buf []byte
}

func New256() *Digest {
	return &Digest{a: make([]uint64, 25)}
}

func (d *Digest) Write(p []byte) (int, error) {
	d.buf = append(d.buf, p...)
	for len(d.buf) >= rate256 {
		xorBytes(d.a, d.buf[0:rate256])
		keccakF(d.a)
		d.buf = d.buf[rate256:]
	}
	return len(p), nil
}

func (d *Digest) Sum(b []byte) []byte {
	a := make([]uint64, 25)
	copy(a, d.a)
	block := make([]byte, rate256)
	copy(block, d.buf)
	block[len(d.buf)] = 0x06
	block[rate256-1] = block[rate256-1] | 0x80
	xorBytes(a, block)
	keccakF(a)
	return append(b, squeeze(a, 32)...)
}

func (d *Digest) Reset() {
	d.a = make([]uint64, 25)
	d.buf = nil
}

func (d *Digest) Size() int      { return 32 }
func (d *Digest) BlockSize() int { return rate256 }
