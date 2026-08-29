package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"
	"math/big"
)

func main() {
	c := elliptic.P256()
	fmt.Println(c.IsOnCurve(c.Gx, c.Gy))

	g := c.Generator()
	p1 := c.ScalarBaseMult(big.NewInt(1))
	fmt.Println(p1.X.Cmp(g.X) == 0)
	fmt.Println(p1.Y.Cmp(g.Y) == 0)

	p2 := c.ScalarBaseMult(big.NewInt(2))
	fmt.Println(c.IsOnCurve(p2.X, p2.Y))

	alice := &ecdh.PrivateKey{D: big.NewInt(2)}
	bob := &ecdh.PrivateKey{D: big.NewInt(3)}
	aPub := ecdh.Public(alice)
	bPub := ecdh.Public(bob)
	sa := ecdh.SharedSecret(alice, bPub)
	sb := ecdh.SharedSecret(bob, aPub)
	fmt.Println(len(sa) > 0)
	fmt.Println(string(sa) == string(sb))

	d := big.NewInt(3)
	pub := ecdsa.GeneratePublic(d)
	priv := &ecdsa.PrivateKey{PublicKey: *pub, D: d}
	hash := big.NewInt(5)
	sig := ecdsa.SignRaw(priv, hash, big.NewInt(2))
	fmt.Println(ecdsa.Verify(pub, hash, sig))
	sig.R = big.NewInt(1)
	fmt.Println(ecdsa.Verify(pub, hash, sig) == false)
}
