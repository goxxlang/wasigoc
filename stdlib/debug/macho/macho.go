// Bounded debug/macho: a read-only Mach-O header parser -- 64-bit only
// (no 32-bit `mach_header`/fat binaries), same "pick the common case"
// bound as debug/pe skipping PE32 and debug/elf skipping 32-bit/big-
// endian. Parses the `mach_header_64` and walks the load command list,
// extracting ONLY `LC_SEGMENT_64` commands (segment name + its sections'
// name/address/size/file offset). No symbol table (`LC_SYMTAB`), no dyld
// info, no code signature, no other load command's contents. `NewFile
// ([]byte)` rather than real Go's `io.ReaderAt`, same bound as debug/pe
// and debug/elf. Like debug/elf (and UNLIKE debug/pe, which WAS checked
// against a real system binary), this package was NOT verified against a
// real Mach-O file -- this project's host (Windows) and target (wasm32-
// wasip1) are both non-Mach-O, so none was available; verified instead by
// careful hand construction against the well-documented, stable Mach-O
// 64-bit load-command layout (fixed 32-byte file header, fixed 72-byte
// segment command header, fixed 80-byte section entries) -- a real,
// stated gap, same honest-boundary spirit as debug/elf's own.
package macho

import "errors"

var ErrFormat = errors.New("macho: not a valid 64-bit Mach-O file")

const Magic64 = 4277009103

const CpuTypeX86_64 = 16777223
const CpuTypeArm64 = 16777228

const LC_SEGMENT_64 = 25

func readU32(b []byte, off int) uint32 {
	return uint32(b[off]) | (uint32(b[off+1]) << 8) | (uint32(b[off+2]) << 16) | (uint32(b[off+3]) << 24)
}

func readU64(b []byte, off int) uint64 {
	lo := uint64(readU32(b, off))
	hi := uint64(readU32(b, off+4))
	return lo | (hi << 32)
}

func cstringTrim(b []byte) string {
	end := len(b)
	i := 0
	for i < len(b) {
		if b[i] == 0 {
			end = i
			break
		}
		i = i + 1
	}
	return string(b[0:end])
}

type FileHeader struct {
	Magic    uint32
	CpuType  int32
	FileType uint32
	Ncmds    uint32
}

type Section struct {
	Name   string
	Addr   uint64
	Size   uint64
	Offset uint32
}

type Segment struct {
	Name     string
	Sections []*Section
}

type File struct {
	FileHeader FileHeader
	Segments   []*Segment
}

func NewFile(data []byte) (*File, error) {
	if len(data) < 32 {
		return nil, ErrFormat
	}
	magic := readU32(data, 0)
	if magic != Magic64 {
		return nil, ErrFormat
	}
	fh := FileHeader{
		Magic:    magic,
		CpuType:  int32(readU32(data, 4)),
		FileType: readU32(data, 12),
		Ncmds:    readU32(data, 16),
	}

	var segments []*Segment
	pos := 32
	i := 0
	for i < int(fh.Ncmds) {
		if pos+8 > len(data) {
			return nil, ErrFormat
		}
		cmd := readU32(data, pos)
		cmdsize := readU32(data, pos+4)
		if cmd == LC_SEGMENT_64 {
			if pos+72 > len(data) {
				return nil, ErrFormat
			}
			segName := cstringTrim(data[pos+8 : pos+24])
			nsects := readU32(data, pos+64)
			seg := &Segment{Name: segName}
			secBase := pos + 72
			j := 0
			for j < int(nsects) {
				sb := secBase + j*80
				if sb+80 > len(data) {
					return nil, ErrFormat
				}
				sec := &Section{
					Name:   cstringTrim(data[sb : sb+16]),
					Addr:   readU64(data, sb+32),
					Size:   readU64(data, sb+40),
					Offset: readU32(data, sb+48),
				}
				seg.Sections = append(seg.Sections, sec)
				j = j + 1
			}
			segments = append(segments, seg)
		}
		pos = pos + int(cmdsize)
		i = i + 1
	}
	return &File{FileHeader: fh, Segments: segments}, nil
}
