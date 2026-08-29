package main

import (
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"strings"
)

// This hex string is the exact raw build-info blob (magic + header + the
// two varint-length-prefixed strings) extracted byte-for-byte from a
// real Go 1.26.4-built binary -- not hand-constructed. See
// stdlib/debug/buildinfo/buildinfo.go's own header comment for how it
// was obtained and cross-checked against real Go's own
// debug/buildinfo.ReadFile output for the same binary.
const blobHex = "ff20476f206275696c64696e663a08020000000000000000000000000000000008676f312e32362e34a6023077af0c9274080241e1c107e6d618e67061746809666978747572652e6578616d706c652f74696e7968656c6c6f0a6d6f6409666978747572652e6578616d706c652f74696e7968656c6c6f0928646576656c29090a6275696c64092d6275696c646d6f64653d6578650a6275696c64092d636f6d70696c65723d67630a6275696c640943474f5f454e41424c45443d310a6275696c640943474f5f43464c4147533d0a6275696c640943474f5f435050464c4147533d0a6275696c640943474f5f435858464c4147533d0a6275696c640943474f5f4c44464c4147533d0a6275696c6409474f415243483d616d6436340a6275696c6409474f4f533d77696e646f77730a6275696c6409474f414d4436343d76310af932433186182072008242104116d8f2"

func main() {
	blob, herr := hex.DecodeString(blobHex)
	fmt.Println(herr == nil)

	bi, err := buildinfo.Read(blob)
	fmt.Println(err == nil)
	fmt.Println(bi.GoVersion == "go1.26.4")
	fmt.Println(strings.Contains(bi.ModInfo, "fixture.example/tinyhello"))
	fmt.Println(strings.Contains(bi.ModInfo, "GOOS=windows"))

	_, badErr := buildinfo.Read([]byte("not a go binary at all"))
	fmt.Println(badErr != nil)
}
