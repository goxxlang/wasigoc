package main

import (
	"crypto/dsa"
	"fmt"
	"math/big"
)

func main() {
	priv := &dsa.PrivateKey{
		P: big.NewInt(23),
		Q: big.NewInt(11),
		G: big.NewInt(2),
		X: big.NewInt(5),
		Y: big.NewInt(9),
	}
	hash := big.NewInt(4)
	k := big.NewInt(7)
	sig := dsa.SignRaw(priv, hash, k)
	fmt.Println(sig.R.String() == "2")
	fmt.Println(sig.S.String() == "2")
	pub := dsa.Public(priv)
	fmt.Println(dsa.VerifyRaw(pub, hash, sig))
	sig.R = big.NewInt(3)
	fmt.Println(dsa.VerifyRaw(pub, hash, sig) == false)
}
