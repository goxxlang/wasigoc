// Adler-32 checksum (RFC 1950 -- used by zlib).
package adler32

const mod = 65521

func Checksum(data []byte) uint32 {
	var a uint32 = 1
	var b uint32 = 0
	for i := 0; i < len(data); i++ {
		a = (a + uint32(data[i])) % mod
		b = (b + a) % mod
	}
	return (b << 16) | a
}

type Digest struct {
	a uint32
	b uint32
}

func New() *Digest {
	return &Digest{a: 1, b: 0}
}

func (d *Digest) Write(data []byte) (int, error) {
	a := d.a
	b := d.b
	for i := 0; i < len(data); i++ {
		a = (a + uint32(data[i])) % mod
		b = (b + a) % mod
	}
	d.a = a
	d.b = b
	return len(data), nil
}

func (d *Digest) Sum32() uint32  { return (d.b << 16) | d.a }
func (d *Digest) Reset()         { d.a = 1; d.b = 0 }
func (d *Digest) Size() int      { return 4 }
func (d *Digest) BlockSize() int { return 4 }
func (d *Digest) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
