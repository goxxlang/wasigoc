// Bounded debug/dwarf: reads the 32-bit compilation-unit header
// (unit_length, version, debug_abbrev_offset, address_size) and
// nothing else -- no abbreviation table, no DIE tree, no line table.
// Same "header reader" scope as debug/pe/elf/macho. 64-bit DWARF
// (unit_length == 0xffffffff) is rejected.
package dwarf

import "errors"

var ErrFormat = errors.New("dwarf: not a valid 32-bit compilation unit header")

type Data struct {
	UnitLength   uint32
	Version      uint16
	AbbrevOffset uint32
	AddrSize     uint8
}

func readU16(b []byte, off int) uint16 {
	return uint16(b[off]) | (uint16(b[off+1]) << 8)
}

func readU32(b []byte, off int) uint32 {
	return uint32(b[off]) | (uint32(b[off+1]) << 8) | (uint32(b[off+2]) << 16) | (uint32(b[off+3]) << 24)
}

func New(b []byte) (*Data, error) {
	if len(b) < 11 {
		return nil, ErrFormat
	}
	unit := readU32(b, 0)
	if unit == 0xffffffff {
		return nil, ErrFormat
	}
	ver := readU16(b, 4)
	if ver < 2 || ver > 5 {
		return nil, ErrFormat
	}
	return &Data{
		UnitLength:   unit,
		Version:      ver,
		AbbrevOffset: readU32(b, 6),
		AddrSize:     b[10],
	}, nil
}
