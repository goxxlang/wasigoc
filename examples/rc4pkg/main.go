package main

import (
	"crypto/rc4"
	"encoding/hex"
	"fmt"
)

func encrypt(key string, plain string) string {
	c, err := rc4.NewCipher([]byte(key))
	if err != nil {
		return "error"
	}
	dst := make([]byte, len(plain))
	c.XORKeyStream(dst, []byte(plain))
	return hex.EncodeToString(dst)
}

func main() {
	fmt.Println(encrypt("Key", "Plaintext") == "bbf316e8d940af0ad3")
	fmt.Println(encrypt("Wiki", "pedia") == "1021bf0420")
	fmt.Println(encrypt("Secret", "Attack at dawn") == "45a01f645fc35b383552544b9bf5")

	// RC4 is symmetric: encrypting the ciphertext with a fresh cipher
	// (same key) recovers the plaintext.
	c1, _ := rc4.NewCipher([]byte("Key"))
	ct := make([]byte, len("Plaintext"))
	c1.XORKeyStream(ct, []byte("Plaintext"))

	c2, _ := rc4.NewCipher([]byte("Key"))
	pt := make([]byte, len(ct))
	c2.XORKeyStream(pt, ct)
	fmt.Println(string(pt) == "Plaintext")

	// An invalid key size is a real error, not silently accepted.
	_, err := rc4.NewCipher([]byte{})
	fmt.Println(err != nil)

	// Streaming across multiple XORKeyStream calls (keystream state
	// carries over) matches one shot.
	c3, _ := rc4.NewCipher([]byte("Key"))
	dst1 := make([]byte, 4)
	dst2 := make([]byte, 5)
	c3.XORKeyStream(dst1, []byte("Plai"))
	c3.XORKeyStream(dst2, []byte("ntext"))
	combined := append(append([]byte{}, dst1...), dst2...)
	fmt.Println(hex.EncodeToString(combined) == "bbf316e8d940af0ad3")
}
