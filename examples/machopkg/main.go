package main

import (
	"debug/macho"
	"fmt"
)

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
func appendU64(b []byte, v uint64) []byte {
	b = appendU32(b, uint32(v))
	b = appendU32(b, uint32(v>>32))
	return b
}
func namePad(b []byte, name string, width int) []byte {
	b = append(b, name...)
	n := len(name)
	for n < width {
		b = append(b, 0)
		n = n + 1
	}
	return b
}

// buildMinimalMachO hand-constructs a minimal but structurally real
// 64-bit Mach-O header + one segment/section, the same "own tiny
// fixture" approach debug/pe's and debug/elf's golden tests use -- see
// the tracker entry for why (no real Mach-O file was available to test
// against on this Windows host).
func buildMinimalMachO() []byte {
	var b []byte
	b = appendU32(b, macho.Magic64)
	b = appendU32(b, uint32(macho.CpuTypeX86_64))
	b = appendU32(b, 0)
	b = appendU32(b, 2)
	b = appendU32(b, 1)
	b = appendU32(b, 152+72)
	b = appendU32(b, 0)
	b = appendU32(b, 0)

	b = appendU32(b, 25)
	b = appendU32(b, 152)
	b = namePad(b, "__TEXT", 16)
	b = appendU64(b, 4096)
	b = appendU64(b, 8192)
	b = appendU64(b, 0)
	b = appendU64(b, 8192)
	b = appendU32(b, 7)
	b = appendU32(b, 5)
	b = appendU32(b, 1)
	b = appendU32(b, 0)

	b = namePad(b, "__text", 16)
	b = namePad(b, "__TEXT", 16)
	b = appendU64(b, 4096)
	b = appendU64(b, 100)
	b = appendU32(b, 400)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU32(b, 0)

	return b
}

func main() {
	data := buildMinimalMachO()
	f, err := macho.NewFile(data)
	fmt.Println(err == nil)
	fmt.Println(f.FileHeader.CpuType == macho.CpuTypeX86_64)
	fmt.Println(int(f.FileHeader.Ncmds) == 1)
	fmt.Println(len(f.Segments) == 1)
	fmt.Println(f.Segments[0].Name == "__TEXT")
	fmt.Println(len(f.Segments[0].Sections) == 1)
	fmt.Println(f.Segments[0].Sections[0].Name == "__text")
	fmt.Println(f.Segments[0].Sections[0].Addr == 4096)
	fmt.Println(f.Segments[0].Sections[0].Size == 100)
	fmt.Println(f.Segments[0].Sections[0].Offset == 400)

	_, badErr := macho.NewFile([]byte("not macho"))
	fmt.Println(badErr != nil)
}
