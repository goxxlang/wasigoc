// Bounded debug/pe: a read-only Windows PE/COFF header parser -- DOS
// header (just enough to find `e_lfanew`), COFF file header, the handful
// of optional-header fields that matter (`Magic`/`AddressOfEntryPoint`/
// `ImageBase`, correctly sized for BOTH PE32 and PE32+ since `ImageBase`
// is 4 bytes in one and 8 in the other), and the section table (name/
// sizes/addresses/characteristics). No symbol table, no import/export
// directory, no relocations, no resources -- real Go's own `debug/pe` has
// all of that, this is the bounded "header reader" slice of it, same
// scope precedent as `archive/tar`'s `Header{Name,Mode,Size,Typeflag}`.
// `NewFile` takes a `[]byte` directly rather than real Go's `io.ReaderAt`
// (no ReaderAt interface here), same bound as `archive/zip.NewReader`.
// `FileHeader`/`OptionalHeader` are named fields, not embedded the way
// real Go's `pe.File` embeds `FileHeader` -- avoids relying on struct
// embedding for field promotion here, a deliberate simplification, not a
// missing feature.
package pe

import "errors"

var ErrFormat = errors.New("pe: not a valid PE file")

const MachineI386 = 332
const MachineAMD64 = 34404
const MachineARM64 = 43620

const Magic32 = 267
const Magic32Plus = 523

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
	Machine              uint16
	NumberOfSections     uint16
	TimeDateStamp        uint32
	SizeOfOptionalHeader uint16
	Characteristics      uint16
}

type OptionalHeader struct {
	Magic               uint16
	AddressOfEntryPoint uint32
	ImageBase           uint64
}

type Section struct {
	Name            string
	VirtualSize     uint32
	VirtualAddress  uint32
	Size            uint32
	Offset          uint32
	Characteristics uint32
}

type File struct {
	FileHeader     FileHeader
	OptionalHeader OptionalHeader
	Sections       []*Section
}

func NewFile(data []byte) (*File, error) {
	if len(data) < 64 {
		return nil, ErrFormat
	}
	if data[0] != 'M' || data[1] != 'Z' {
		return nil, ErrFormat
	}
	lfanew := int(readU32(data, 60))
	if lfanew < 0 || lfanew+24 > len(data) {
		return nil, ErrFormat
	}
	if data[lfanew] != 'P' || data[lfanew+1] != 'E' || data[lfanew+2] != 0 || data[lfanew+3] != 0 {
		return nil, ErrFormat
	}

	coffOff := lfanew + 4
	fh := FileHeader{
		Machine:              readU16(data, coffOff),
		NumberOfSections:     readU16(data, coffOff+2),
		TimeDateStamp:        readU32(data, coffOff+4),
		SizeOfOptionalHeader: readU16(data, coffOff+16),
		Characteristics:      readU16(data, coffOff+18),
	}

	optOff := coffOff + 20
	var oh OptionalHeader
	if fh.SizeOfOptionalHeader > 0 {
		if optOff+2 > len(data) {
			return nil, ErrFormat
		}
		oh.Magic = readU16(data, optOff)
		oh.AddressOfEntryPoint = readU32(data, optOff+16)
		if oh.Magic == Magic32Plus {
			oh.ImageBase = readU64(data, optOff+24)
		} else {
			oh.ImageBase = uint64(readU32(data, optOff+28))
		}
	}

	secOff := optOff + int(fh.SizeOfOptionalHeader)
	var sections []*Section
	i := 0
	for i < int(fh.NumberOfSections) {
		base := secOff + i*40
		if base+40 > len(data) {
			return nil, ErrFormat
		}
		s := &Section{
			Name:            cstringTrim(data[base : base+8]),
			VirtualSize:     readU32(data, base+8),
			VirtualAddress:  readU32(data, base+12),
			Size:            readU32(data, base+16),
			Offset:          readU32(data, base+20),
			Characteristics: readU32(data, base+36),
		}
		sections = append(sections, s)
		i = i + 1
	}
	return &File{FileHeader: fh, OptionalHeader: oh, Sections: sections}, nil
}
