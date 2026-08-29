// Tiny subset of encoding/binary: fixed-width Uint16/32/64 get/put, little
// and big endian. Real Go's LittleEndian/BigEndian are two distinct
// unexported types both implementing a ByteOrder interface; here they're
// one struct with a flag -- same functional behavior, just not
// distinguishable via reflection (which this compiler doesn't have
// anyway).
package binary

type byteOrder struct {
	little bool
}

var LittleEndian = byteOrder{little: true}
var BigEndian = byteOrder{little: false}

func (o byteOrder) Uint16(b []byte) uint16 {
	if o.little {
		return uint16(b[0]) | uint16(b[1])<<8
	}
	return uint16(b[1]) | uint16(b[0])<<8
}

func (o byteOrder) PutUint16(b []byte, v uint16) {
	if o.little {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
	} else {
		b[0] = byte(v >> 8)
		b[1] = byte(v)
	}
}

func (o byteOrder) Uint32(b []byte) uint32 {
	if o.little {
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	}
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

func (o byteOrder) PutUint32(b []byte, v uint32) {
	if o.little {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
		b[2] = byte(v >> 16)
		b[3] = byte(v >> 24)
	} else {
		b[3] = byte(v)
		b[2] = byte(v >> 8)
		b[1] = byte(v >> 16)
		b[0] = byte(v >> 24)
	}
}

func (o byteOrder) Uint64(b []byte) uint64 {
	if o.little {
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
			uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	}
	return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
		uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
}

func (o byteOrder) PutUint64(b []byte, v uint64) {
	if o.little {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
		b[2] = byte(v >> 16)
		b[3] = byte(v >> 24)
		b[4] = byte(v >> 32)
		b[5] = byte(v >> 40)
		b[6] = byte(v >> 48)
		b[7] = byte(v >> 56)
	} else {
		b[7] = byte(v)
		b[6] = byte(v >> 8)
		b[5] = byte(v >> 16)
		b[4] = byte(v >> 24)
		b[3] = byte(v >> 32)
		b[2] = byte(v >> 40)
		b[1] = byte(v >> 48)
		b[0] = byte(v >> 56)
	}
}

func (o byteOrder) AppendUint16(b []byte, v uint16) []byte {
	out := make([]byte, 2)
	o.PutUint16(out, v)
	return append(b, out...)
}

func (o byteOrder) AppendUint32(b []byte, v uint32) []byte {
	out := make([]byte, 4)
	o.PutUint32(out, v)
	return append(b, out...)
}

func (o byteOrder) AppendUint64(b []byte, v uint64) []byte {
	out := make([]byte, 8)
	o.PutUint64(out, v)
	return append(b, out...)
}
