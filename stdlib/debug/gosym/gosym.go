// Bounded debug/gosym: reads the pclntab magic / quantum / ptrsize
// header and nothing else -- no PC-to-line, no Func table. Same
// "header reader" scope as debug/dwarf. Recognizes the well-known
// little-endian magics Go 1.2 (0xfffffffb) through Go 1.20
// (0xfffffff1).
package gosym

import "errors"

var ErrFormat = errors.New("gosym: not a valid pclntab header")

const Magic12 uint32 = 0xfffffffb
const Magic116 uint32 = 0xfffffffa
const Magic118 uint32 = 0xfffffff0
const Magic120 uint32 = 0xfffffff1

type Table struct {
	Magic   uint32
	Quantum uint8
	Ptrsize uint8
}

func readU32(b []byte, off int) uint32 {
	return uint32(b[off]) | (uint32(b[off+1]) << 8) | (uint32(b[off+2]) << 16) | (uint32(b[off+3]) << 24)
}

func NewTable(pclntab []byte) (*Table, error) {
	if len(pclntab) < 8 {
		return nil, ErrFormat
	}
	m := readU32(pclntab, 0)
	if m != Magic12 && m != Magic116 && m != Magic118 && m != Magic120 {
		return nil, ErrFormat
	}
	return &Table{
		Magic:   m,
		Quantum: pclntab[6],
		Ptrsize: pclntab[7],
	}, nil
}
