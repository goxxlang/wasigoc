package main

import (
	"debug/elf"
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

func buildMinimalELF() []byte {
	var b []byte
	b = append(b, 127, 'E', 'L', 'F')
	b = append(b, 2, 1, 1, 0)
	for len(b) < 16 {
		b = append(b, 0)
	}
	b = appendU16(b, 2)  // e_type ET_EXEC
	b = appendU16(b, 62) // e_machine EM_X86_64
	b = appendU32(b, 1)  // e_version
	b = appendU64(b, 4096) // e_entry
	b = appendU64(b, 0)    // e_phoff
	b = appendU64(b, 81)   // e_shoff
	b = appendU32(b, 0)    // e_flags
	b = appendU16(b, 64)   // e_ehsize
	b = appendU16(b, 0)    // e_phentsize
	b = appendU16(b, 0)    // e_phnum
	b = appendU16(b, 64)   // e_shentsize
	b = appendU16(b, 3)    // e_shnum
	b = appendU16(b, 2)    // e_shstrndx

	// shstrtab content at offset 64: "\0.text\0.shstrtab\0"
	b = append(b, 0)
	b = append(b, '.', 't', 'e', 'x', 't', 0)
	b = append(b, '.', 's', 'h', 's', 't', 'r', 't', 'a', 'b', 0)

	// section 0: NULL
	for len(b) < 81+64 {
		b = append(b, 0)
	}
	// section 1: .text
	b = appendU32(b, 1)  // sh_name
	b = appendU32(b, 1)  // sh_type PROGBITS
	b = appendU64(b, 6)  // sh_flags ALLOC|EXECINSTR
	b = appendU64(b, 4096) // sh_addr
	b = appendU64(b, 1024) // sh_offset
	b = appendU64(b, 256)  // sh_size
	b = appendU32(b, 0)    // sh_link
	b = appendU32(b, 0)    // sh_info
	b = appendU64(b, 16)   // sh_addralign
	b = appendU64(b, 0)    // sh_entsize

	// section 2: .shstrtab
	b = appendU32(b, 7)  // sh_name (offset of "shstrtab" in table)
	b = appendU32(b, 3)  // sh_type STRTAB
	b = appendU64(b, 0)
	b = appendU64(b, 0)
	b = appendU64(b, 64) // sh_offset
	b = appendU64(b, 17) // sh_size
	b = appendU32(b, 0)
	b = appendU32(b, 0)
	b = appendU64(b, 1)
	b = appendU64(b, 0)

	return b
}

func main() {
	data := buildMinimalELF()
	f, err := elf.NewFile(data)
	fmt.Println(err == nil)
	fmt.Println(f.FileHeader.Type == elf.ET_EXEC)
	fmt.Println(f.FileHeader.Machine == elf.EM_X86_64)
	fmt.Println(f.FileHeader.Entry == 4096)
	fmt.Println(len(f.Sections) == 3)
	fmt.Println(f.Sections[0].Name == "")
	fmt.Println(f.Sections[1].Name == ".text")
	fmt.Println(f.Sections[1].Type == 1)
	fmt.Println(f.Sections[1].Addr == 4096)
	fmt.Println(f.Sections[2].Name == ".shstrtab")
	fmt.Println(f.Sections[2].Size == 17)

	_, badErr := elf.NewFile([]byte("not an elf file"))
	fmt.Println(badErr != nil)
}
