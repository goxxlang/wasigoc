package main

import (
	"crypto/pbkdf2"
	"encoding/hex"
	"fmt"
)

func main() {
	k1 := pbkdf2.Key([]byte("password"), []byte("salt"), 1, 32)
	fmt.Println(hex.EncodeToString(k1) == "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b")
	k2 := pbkdf2.Key([]byte("password"), []byte("salt"), 2, 32)
	fmt.Println(hex.EncodeToString(k2) == "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43")
}
