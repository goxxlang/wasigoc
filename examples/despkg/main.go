package main

import (
	"crypto/cipher"
	"crypto/des"
	"encoding/hex"
	"fmt"
)

func main() {
	key, _ := hex.DecodeString("133457799BBCDFF1")
	plain, _ := hex.DecodeString("0123456789ABCDEF")

	c, err := des.NewCipher(key)
	fmt.Println(err == nil)

	var block cipher.Block = c
	fmt.Println(block.BlockSize())

	enc := make([]byte, 8)
	block.Encrypt(enc, plain)
	fmt.Println(hex.EncodeToString(enc))

	dec := make([]byte, 8)
	block.Decrypt(dec, enc)
	fmt.Println(hex.EncodeToString(dec))

	_, badErr := des.NewCipher([]byte("short"))
	fmt.Println(badErr == nil)

	zkey, _ := hex.DecodeString("0000000000000000")
	zplain, _ := hex.DecodeString("0000000000000000")
	zc, _ := des.NewCipher(zkey)
	zenc := make([]byte, 8)
	zc.Encrypt(zenc, zplain)
	fmt.Println(hex.EncodeToString(zenc))
}
