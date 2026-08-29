// Bounded ECDSA over P-256. SignRaw takes a caller-supplied k (no
// crypto/rand CSPRNG here -- same bound as crypto/dsa). Verify the
// usual u1*G + u2*Q check. Hash is interpreted as a big-endian integer
// already truncated/reduced by the caller if needed.
package ecdsa

import (
	"crypto/elliptic"
	"math/big"
)

type PublicKey struct {
	X *big.Int
	Y *big.Int
}

type PrivateKey struct {
	PublicKey
	D *big.Int
}

type Signature struct {
	R *big.Int
	S *big.Int
}

func SignRaw(priv *PrivateKey, hash *big.Int, k *big.Int) *Signature {
	c := elliptic.P256()
	p := c.ScalarBaseMult(k)
	r := new(big.Int).Mod(p.X, c.N)
	kInv := new(big.Int).ModInverse(k, c.N)
	rd := new(big.Int).Mul(r, priv.D)
	sum := new(big.Int).Add(hash, rd)
	s := new(big.Int).Mul(kInv, sum)
	s.Mod(s, c.N)
	return &Signature{R: r, S: s}
}

func Verify(pub *PublicKey, hash *big.Int, sig *Signature) bool {
	c := elliptic.P256()
	if sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		return false
	}
	if sig.R.Cmp(c.N) >= 0 || sig.S.Cmp(c.N) >= 0 {
		return false
	}
	w := new(big.Int).ModInverse(sig.S, c.N)
	if w == nil {
		return false
	}
	u1 := new(big.Int).Mul(hash, w)
	u1.Mod(u1, c.N)
	u2 := new(big.Int).Mul(sig.R, w)
	u2.Mod(u2, c.N)
	p1 := c.ScalarBaseMult(u1)
	p2 := c.ScalarMult(&elliptic.Point{X: pub.X, Y: pub.Y}, u2)
	sum := c.Add(p1, p2)
	if sum.Inf {
		return false
	}
	v := new(big.Int).Mod(sum.X, c.N)
	return v.Cmp(sig.R) == 0
}

func GeneratePublic(d *big.Int) *PublicKey {
	c := elliptic.P256()
	p := c.ScalarBaseMult(d)
	return &PublicKey{X: p.X, Y: p.Y}
}
