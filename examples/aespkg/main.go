package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
)

func main() {
	key, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	plain, _ := hex.DecodeString("00112233445566778899aabbccddeeff")

	c, err := aes.NewCipher(key)
	fmt.Println(err == nil)

	var block cipher.Block = c
	fmt.Println(block.BlockSize())

	enc := make([]byte, 16)
	block.Encrypt(enc, plain)
	fmt.Println(hex.EncodeToString(enc))

	dec := make([]byte, 16)
	block.Decrypt(dec, enc)
	fmt.Println(hex.EncodeToString(dec))

	_, badErr := aes.NewCipher([]byte("short"))
	fmt.Println(badErr == nil)
}
