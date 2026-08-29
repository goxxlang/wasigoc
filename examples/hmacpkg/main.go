package main

import (
	"crypto/hmac"
	"encoding/hex"
	"fmt"
)

func main() {
	// RFC 4231 test case 1: Key = 20 bytes of 0x0b, Data = "Hi There".
	key1 := make([]byte, 20)
	for i := 0; i < 20; i++ {
		key1[i] = 0x0b
	}
	data1 := []byte("Hi There")
	mac1 := hmac.SumSHA256(key1, data1)
	fmt.Println(hex.EncodeToString(mac1) == "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7")

	// RFC 4231 test case 2: Key = "Jefe", Data = "what do ya want for nothing?".
	key2 := []byte("Jefe")
	data2 := []byte("what do ya want for nothing?")
	mac2 := hmac.SumSHA256(key2, data2)
	fmt.Println(hex.EncodeToString(mac2) == "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843")

	// RFC 2202 HMAC-SHA1 test case 1: key = 20 bytes 0x0b, data = "Hi There".
	macSha1 := hmac.SumSHA1(key1, data1)
	fmt.Println(hex.EncodeToString(macSha1) == "b617318655057264e28bc0b6fb378c8ef146be00")

	// RFC 2202 HMAC-MD5 test case 1: key = 16 bytes 0x0b, data = "Hi There".
	key3 := make([]byte, 16)
	for i := 0; i < 16; i++ {
		key3[i] = 0x0b
	}
	macMd5 := hmac.SumMD5(key3, data1)
	fmt.Println(hex.EncodeToString(macMd5) == "9294727a3638bb1c13f48ef8158bfc9d")

	// A key longer than the block size (64 bytes) must be hashed down first
	// (RFC 4231 test case 6: key = 131 bytes of 0xaa).
	longKey := make([]byte, 131)
	for i := 0; i < 131; i++ {
		longKey[i] = 0xaa
	}
	longData := []byte("Test Using Larger Than Block-Size Key - Hash Key First")
	macLong := hmac.SumSHA256(longKey, longData)
	fmt.Println(hex.EncodeToString(macLong) == "60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54")

	// Equal.
	fmt.Println(hmac.Equal(mac1, hmac.SumSHA256(key1, data1)))
	fmt.Println(hmac.Equal(mac1, mac2))
}
