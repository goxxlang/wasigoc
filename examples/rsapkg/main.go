package main

import (
	"crypto/rsa"
	"fmt"
	"math/big"
)

func main() {
	pub := &rsa.PublicKey{N: big.NewInt(3233), E: 17}
	priv := &rsa.PrivateKey{PublicKey: *pub, D: big.NewInt(2753)}

	m := big.NewInt(65)
	c := rsa.EncryptRaw(pub, m)
	fmt.Println(c.String() == "2790")

	back := rsa.DecryptRaw(priv, c)
	fmt.Println(back.String() == "65")

	n2, _ := new(big.Int).SetString("589", 10)
	pub2 := &rsa.PublicKey{N: n2, E: 7}
	d2, _ := new(big.Int).SetString("463", 10)
	priv2 := &rsa.PrivateKey{PublicKey: *pub2, D: d2}

	m2 := big.NewInt(123)
	c2 := rsa.EncryptRaw(pub2, m2)
	fmt.Println(c2.String() == "61")
	back2 := rsa.DecryptRaw(priv2, c2)
	fmt.Println(back2.String() == "123")
}
