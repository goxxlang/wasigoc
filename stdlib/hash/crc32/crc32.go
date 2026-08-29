// CRC-32, IEEE polynomial only (the checksum used by gzip/zip/PNG). No
// Castagnoli/Koopman tables, no hash.Hash32-returning New() -- just the
// package-level ChecksumIEEE, the everyday entry point. The table is a
// []uint32 slice (not Go's fixed-size [256]uint32 array type, untested
// here) built once by a package-level var calling a helper function,
// avoiding `func init()` (also untested) entirely.
package crc32

const polynomial = 3988292384 // 0xEDB88320, the reflected IEEE polynomial

func makeIEEETable() []uint32 {
	t := make([]uint32, 256)
	for i := 0; i < 256; i++ {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ polynomial
			} else {
				crc = crc >> 1
			}
		}
		t[i] = crc
	}
	return t
}

var ieeeTable = makeIEEETable()

func ChecksumIEEE(data []byte) uint32 {
	var crc uint32 = 4294967295
	for i := 0; i < len(data); i++ {
		crc = ieeeTable[byte(crc)^data[i]] ^ (crc >> 8)
	}
	return crc ^ 4294967295
}

type Digest struct {
	crc uint32
}

func NewIEEE() *Digest {
	return &Digest{crc: 4294967295}
}

func (d *Digest) Write(data []byte) (int, error) {
	crc := d.crc
	for i := 0; i < len(data); i++ {
		crc = ieeeTable[byte(crc)^data[i]] ^ (crc >> 8)
	}
	d.crc = crc
	return len(data), nil
}

func (d *Digest) Sum32() uint32 { return d.crc ^ 4294967295 }
func (d *Digest) Reset()        { d.crc = 4294967295 }
func (d *Digest) Size() int     { return 4 }
func (d *Digest) BlockSize() int { return 1 }
func (d *Digest) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
