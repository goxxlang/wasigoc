package main

import (
	"crypto/hkdf"
	"encoding/hex"
	"fmt"
)

func main() {
	ikm := make([]byte, 22)
	i := 0
	for i < 22 {
		ikm[i] = 0x0b
		i = i + 1
	}
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	prk := hkdf.Extract(salt, ikm)
	fmt.Println(hex.EncodeToString(prk) == "077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5")
	okm := hkdf.Expand(prk, info, 42)
	fmt.Println(hex.EncodeToString(okm) == "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865")
	sum := hkdf.Sum(ikm, salt, info, 42)
	fmt.Println(hex.EncodeToString(sum) == hex.EncodeToString(okm))
}
