// CRC-64, ISO and ECMA polynomials (real Go's own two named constants) --
// same shape as this project's hash/crc32: a []uint64 table (not a fixed-
// size array), Checksum as the everyday entry point, plus a streaming
// Digest. Constants and the reflected table algorithm verified against an
// independent Python computation before trusting them (see
// examples/crc64pkg): Checksum("123456789", ISO) == 0xb90956c775a41001,
// Checksum("123456789", ECMA) == 0x995dc9bbdf1939fa -- the standard
// CRC-64/GO-ISO and CRC-64/XZ check values.
package crc64

const ISO = 0xD800000000000000
const ECMA = 0xC96C5795D7870F42

func MakeTable(poly uint64) []uint64 {
	t := make([]uint64, 256)
	for i := 0; i < 256; i++ {
		crc := uint64(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ poly
			} else {
				crc = crc >> 1
			}
		}
		t[i] = crc
	}
	return t
}

func Checksum(data []byte, tab []uint64) uint64 {
	var crc uint64 = 18446744073709551615
	for i := 0; i < len(data); i++ {
		crc = tab[byte(crc)^data[i]] ^ (crc >> 8)
	}
	return crc ^ 18446744073709551615
}

type Digest struct {
	crc uint64
	tab []uint64
}

func New(tab []uint64) *Digest {
	return &Digest{crc: 18446744073709551615, tab: tab}
}

func (d *Digest) Write(data []byte) (int, error) {
	crc := d.crc
	for i := 0; i < len(data); i++ {
		crc = d.tab[byte(crc)^data[i]] ^ (crc >> 8)
	}
	d.crc = crc
	return len(data), nil
}

func (d *Digest) Sum64() uint64 { return d.crc ^ 18446744073709551615 }
func (d *Digest) Reset()        { d.crc = 18446744073709551615 }
func (d *Digest) Size() int     { return 8 }
func (d *Digest) BlockSize() int { return 1 }
func (d *Digest) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
