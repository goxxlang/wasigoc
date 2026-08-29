package main

import (
	"bytes"
	"compress/bzip2"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// Real bzip2 data (level 9) produced by Python's own bz2.compress, not
// hand-constructed -- see stdlib/compress/bzip2/bzip2.go's own header
// comment for how it was generated and cross-checked.
const compressedHex = "425a683931415926535905924a050003b83180400040013ffffff03000d805000340000500034000014a94f5403434c86d4f0a673f08b745d516e8b9a2d11611688bce17445eb73fb45845d916117622d917245dd16a45b22ca2fc45845c916e8bba2ca2f945a22d516517545fc8b6459a171c22e117452332b8c22d556eab445fe2ee48a70a1200b24940a0"

func main() {
	compressed, herr := hex.DecodeString(compressedHex)
	fmt.Println(herr == nil)

	r, err := bzip2.NewReader(bytes.NewReader(compressed))
	fmt.Println(err == nil)

	out, rerr := io.ReadAll(r)
	fmt.Println(rerr == nil)

	expected := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 50) + "aaaaaaaaaaaaaaaaaaaa" + strings.Repeat("xyz", 30)
	fmt.Println(len(out) == len(expected))
	fmt.Println(string(out) == expected)

	_, badErr := bzip2.NewReader(bytes.NewReader([]byte("not bzip2 data")))
	fmt.Println(badErr != nil)
}
