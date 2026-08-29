package main

import (
	"debug/pe"
	"fmt"
)

func appendU16(b []byte, v uint16) []byte {
	return append(b, byte(v), byte(v>>8))
}

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

func appendU64(b []byte, v uint64) []byte {
	b = appendU32(b, uint32(v))
	b = appendU32(b, uint32(v>>32))
	return b
}

func padTo(b []byte, n int) []byte {
	for len(b) < n {
		b = append(b, 0)
	}
	return b
}

// buildMinimalPE hand-constructs a minimal but structurally real PE32+
// header + one section, the same "own tiny fixture, no real linker
// needed" approach crypto/aes's FIPS test vector uses -- this package's
// field-by-field correctness against an ACTUAL system binary (wasigoc.exe
// itself, cross-checked against objdump's independent parse) was verified
// separately during development; see the tracker entry.
func buildMinimalPE() []byte {
	var b []byte
	b = append(b, 'M', 'Z')
	b = padTo(b, 60)
	b = appendU32(b, 64)
	b = padTo(b, 64)

	b = append(b, 'P', 'E', 0, 0)
	b = appendU16(b, pe.MachineAMD64)
	b = appendU16(b, 1)
	b = appendU32(b, 1234567890)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU16(b, 32)
	b = appendU16(b, 34)

	optStart := len(b)
	b = appendU16(b, pe.Magic32Plus)
	b = padTo(b, optStart+16)
	b = appendU32(b, 4096)
	b = padTo(b, optStart+24)
	b = appendU64(b, 5368709120)
	b = padTo(b, optStart+32)

	b = append(b, '.', 't', 'e', 's', 't', 0, 0, 0)
	b = appendU32(b, 500)
	b = appendU32(b, 4096)
	b = appendU32(b, 512)
	b = appendU32(b, 1024)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU16(b, 0)
	b = appendU16(b, 0)
	b = appendU32(b, 1610612768)
	return b
}

func main() {
	data := buildMinimalPE()
	f, err := pe.NewFile(data)
	fmt.Println(err == nil)
	fmt.Println(f.FileHeader.Machine == pe.MachineAMD64)
	fmt.Println(int(f.FileHeader.NumberOfSections) == 1)
	fmt.Println(f.OptionalHeader.Magic == pe.Magic32Plus)
	fmt.Println(f.OptionalHeader.AddressOfEntryPoint == 4096)
	fmt.Println(f.OptionalHeader.ImageBase == 5368709120)
	fmt.Println(len(f.Sections) == 1)
	fmt.Println(f.Sections[0].Name == ".test")
	fmt.Println(f.Sections[0].VirtualSize == 500)
	fmt.Println(f.Sections[0].VirtualAddress == 4096)
	fmt.Println(f.Sections[0].Offset == 1024)

	_, badErr := pe.NewFile([]byte("not a pe file at all"))
	fmt.Println(badErr != nil)
}
