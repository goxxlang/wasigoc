// Bounded debug/elf: a read-only ELF header parser -- 64-bit little-endian
// only (no 32-bit, no big-endian), same "pick the common case, document
// the rest as a gap" bound as debug/pe skipping PE32 in favor of PE32+
// where it matters. Parses the ELF identification bytes + file header,
// and the section header table with names resolved against the section
// header string table (`e_shstrndx`) -- no program headers, no symbol
// table, no relocations, no DWARF debug info (that's `debug/dwarf`'s own,
// separate, unimplemented job). `NewFile([]byte)` rather than real Go's
// `io.ReaderAt`, same bound as debug/pe and archive/zip. UNLIKE debug/pe
// (verified against a real system binary, wasigoc.exe itself, cross-
// checked with objdump), this package was NOT verified against a real
// ELF binary -- this project's own toolchain and host are both PE-based
// (Windows MSVC host, wasm32-wasip1 target, neither ELF), so there was no
// real ELF file available to test against. Verified instead by careful
// hand construction against the well-documented, stable ELF64 spec (fixed
// 64-byte file header, fixed 64-byte section headers) -- a real, honest
// gap, not glossed over, same spirit as compress/lzw's "not verified
// against a real GIF" caveat.
package elf

import "errors"

var ErrFormat = errors.New("elf: not a valid ELF64 file")

const ET_REL = 1
const ET_EXEC = 2
const ET_DYN = 3
const ET_CORE = 4

const EM_386 = 3
const EM_X86_64 = 62
const EM_AARCH64 = 183

func readU16(b []byte, off int) uint16 {
	return uint16(b[off]) | (uint16(b[off+1]) << 8)
}

func readU32(b []byte, off int) uint32 {
	return uint32(b[off]) | (uint32(b[off+1]) << 8) | (uint32(b[off+2]) << 16) | (uint32(b[off+3]) << 24)
}

func readU64(b []byte, off int) uint64 {
	lo := uint64(readU32(b, off))
	hi := uint64(readU32(b, off+4))
	return lo | (hi << 32)
}

func cstringAt(b []byte, off int) string {
	end := off
	for end < len(b) && b[end] != 0 {
		end = end + 1
	}
	return string(b[off:end])
}

type FileHeader struct {
	Type    uint16
	Machine uint16
	Entry   uint64
}

type Section struct {
	Name   string
	Type   uint32
	Flags  uint64
	Addr   uint64
	Offset uint64
	Size   uint64
}

type File struct {
	FileHeader FileHeader
	Sections   []*Section
}

func NewFile(data []byte) (*File, error) {
	if len(data) < 64 {
		return nil, ErrFormat
	}
	if data[0] != 127 || data[1] != 'E' || data[2] != 'L' || data[3] != 'F' {
		return nil, ErrFormat
	}
	if data[4] != 2 {
		return nil, ErrFormat
	}
	if data[5] != 1 {
		return nil, ErrFormat
	}

	fh := FileHeader{
		Type:    readU16(data, 16),
		Machine: readU16(data, 18),
		Entry:   readU64(data, 24),
	}
	shoff := readU64(data, 40)
	shentsize := int(readU16(data, 58))
	shnum := int(readU16(data, 60))
	shstrndx := int(readU16(data, 62))

	rawSections := make([][]byte, 0)
	i := 0
	for i < shnum {
		base := int(shoff) + i*shentsize
		if base+64 > len(data) {
			return nil, ErrFormat
		}
		rawSections = append(rawSections, data[base:base+64])
		i = i + 1
	}

	var strtab []byte
	if shstrndx >= 0 && shstrndx < len(rawSections) {
		sh := rawSections[shstrndx]
		strOff := readU64(sh, 24)
		strSize := readU64(sh, 32)
		if int(strOff+strSize) <= len(data) {
			strtab = data[strOff : strOff+strSize]
		}
	}

	var sections []*Section
	i = 0
	for i < len(rawSections) {
		sh := rawSections[i]
		nameOff := int(readU32(sh, 0))
		name := ""
		if strtab != nil {
			name = cstringAt(strtab, nameOff)
		}
		s := &Section{
			Name:   name,
			Type:   readU32(sh, 4),
			Flags:  readU64(sh, 8),
			Addr:   readU64(sh, 16),
			Offset: readU64(sh, 24),
			Size:   readU64(sh, 32),
		}
		sections = append(sections, s)
		i = i + 1
	}
	return &File{FileHeader: fh, Sections: sections}, nil
}
