// Bounded debug/plan9obj: a read-only Plan 9 a.out header parser, same
// "header reader" scope precedent as debug/elf/debug/pe/debug/macho.
// `NewFile([]byte)` rather than real Go's `io.ReaderAt`, same bound as its
// sibling debug/* packages. Parses the 32-byte fixed a.out header (magic/
// text/data/bss/syms/entry/spsz/pcsz, all big-endian uint32 -- Plan 9 a.out
// is big-endian regardless of target byte order) plus the 8-byte expanded
// 64-bit entry field real Go's own format adds when `Magic&Magic64 != 0`,
// and derives the five fixed section slots (text/data/syms/spsz/pcsz) at
// their real Go offsets. No symbol table decoding (`Symbols`/`walksymtab`
// in real Go) -- that needs the same reflection-free byte-table walk this
// project's sibling packages already skip for the analogous "no symbol
// table" bound. Verified against real Go itself, not Python or hand
// derivation: a real Go program built a MagicAMD64 (64-bit) fixture and
// parsed it with the actual `debug/plan9obj` package (`go run`, go1.26.4),
// and every FileHeader/Section field this package computes from the same
// bytes matched the real package's output exactly (Magic, Bss, Entry,
// PtrSize, LoadAddress, HdrSize, and all five section Name/Size/Offset
// triples).
package plan9obj

import "errors"

var ErrFormat = errors.New("plan9obj: not a valid Plan 9 a.out file")

const Magic64 = 0x8000
const Magic386 = (4*11+0)*11 + 7
const MagicAMD64 = (4*26+0)*26 + 7 + Magic64
const MagicARM = (4*20+0)*20 + 7

func readU32(b []byte, off int) uint32 {
	return (uint32(b[off]) << 24) | (uint32(b[off+1]) << 16) | (uint32(b[off+2]) << 8) | uint32(b[off+3])
}

func readU64(b []byte, off int) uint64 {
	hi := uint64(readU32(b, off))
	lo := uint64(readU32(b, off+4))
	return (hi << 32) | lo
}

type FileHeader struct {
	Magic       uint32
	Bss         uint32
	Entry       uint64
	PtrSize     int
	LoadAddress uint64
	HdrSize     uint64
}

type Section struct {
	Name   string
	Size   uint32
	Offset uint32
}

type File struct {
	FileHeader FileHeader
	Sections   []*Section
}

func NewFile(data []byte) (*File, error) {
	if len(data) < 32 {
		return nil, ErrFormat
	}
	magic := readU32(data, 0)
	if magic != Magic386 && magic != MagicAMD64 && magic != MagicARM {
		return nil, ErrFormat
	}
	text := readU32(data, 4)
	dsize := readU32(data, 8)
	bss := readU32(data, 12)
	syms := readU32(data, 16)
	entry32 := readU32(data, 20)
	spsz := readU32(data, 24)
	pcsz := readU32(data, 28)

	fh := FileHeader{
		Magic:       magic,
		Bss:         bss,
		Entry:       uint64(entry32),
		PtrSize:     4,
		LoadAddress: 0x1000,
		HdrSize:     32,
	}

	hdrEnd := 32
	if magic&Magic64 != 0 {
		if len(data) < 40 {
			return nil, ErrFormat
		}
		fh.Entry = readU64(data, 32)
		fh.PtrSize = 8
		fh.LoadAddress = 0x200000
		fh.HdrSize = 40
		hdrEnd = 40
	}

	names := []string{"text", "data", "syms", "spsz", "pcsz"}
	sizes := []uint32{text, dsize, syms, spsz, pcsz}

	var sections []*Section
	off := uint32(hdrEnd)
	i := 0
	for i < 5 {
		sections = append(sections, &Section{Name: names[i], Size: sizes[i], Offset: off})
		off = off + sizes[i]
		i = i + 1
	}

	return &File{FileHeader: fh, Sections: sections}, nil
}

func (f *File) Section(name string) *Section {
	i := 0
	for i < len(f.Sections) {
		if f.Sections[i].Name == name {
			return f.Sections[i]
		}
		i = i + 1
	}
	return nil
}
