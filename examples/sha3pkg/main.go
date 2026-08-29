package main

import (
	"crypto/sha3"
	"encoding/hex"
	"fmt"
)

func main() {
	empty := sha3.Sum256(nil)
	fmt.Println(hex.EncodeToString(empty) == "a7ffc6f8bf1ed76651c14756a061d662f580ff4de43b49fa82d80a4b80f8434a")

	abc := sha3.Sum256([]byte("abc"))
	fmt.Println(hex.EncodeToString(abc) == "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532")

	d := sha3.New256()
	d.Write([]byte("ab"))
	d.Write([]byte("c"))
	fmt.Println(hex.EncodeToString(d.Sum(nil)) == "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532")
	fmt.Println(d.Size() == 32)
	fmt.Println(d.BlockSize() == 136)
}
