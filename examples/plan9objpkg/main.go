package main

import (
	"debug/plan9obj"
	"fmt"
)

func appendU32BE(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
func appendU64BE(b []byte, v uint64) []byte {
	b = appendU32BE(b, uint32(v>>32))
	b = appendU32BE(b, uint32(v))
	return b
}

func buildMinimalPlan9(magic uint32) []byte {
	var b []byte
	b = appendU32BE(b, magic) // magic
	b = appendU32BE(b, 100)   // text
	b = appendU32BE(b, 50)    // data
	b = appendU32BE(b, 20)    // bss
	b = appendU32BE(b, 30)    // syms
	b = appendU32BE(b, 0)     // entry (32-bit slot)
	b = appendU32BE(b, 10)    // spsz
	b = appendU32BE(b, 5)     // pcsz
	if magic&plan9obj.Magic64 != 0 {
		b = appendU64BE(b, 0x201234) // expanded 64-bit entry
	}
	return b
}

func main() {
	data := buildMinimalPlan9(plan9obj.MagicAMD64)
	f, err := plan9obj.NewFile(data)
	fmt.Println(err == nil)
	fmt.Println(f.FileHeader.Magic == plan9obj.MagicAMD64)
	fmt.Println(f.FileHeader.Bss == 20)
	fmt.Println(f.FileHeader.Entry == 0x201234)
	fmt.Println(f.FileHeader.PtrSize == 8)
	fmt.Println(f.FileHeader.LoadAddress == 0x200000)
	fmt.Println(f.FileHeader.HdrSize == 40)
	fmt.Println(len(f.Sections) == 5)
	fmt.Println(f.Sections[0].Name == "text")
	fmt.Println(f.Sections[0].Size == 100)
	fmt.Println(f.Sections[0].Offset == 40)
	fmt.Println(f.Sections[1].Offset == 140)
	fmt.Println(f.Sections[4].Name == "pcsz")

	tf := f.Section("text")
	fmt.Println(tf != nil)
	fmt.Println(f.Section("nope") == nil)

	_, badErr := plan9obj.NewFile([]byte("not plan9"))
	fmt.Println(badErr != nil)
}
