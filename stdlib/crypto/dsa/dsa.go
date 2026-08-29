// Bounded DSA: textbook SignRaw/VerifyRaw with a caller-supplied k,
// same "primitive, not GenerateKey" shape as crypto/rsa. No padding,
// no parameter generation (needs probable primes). Present for
// textbook interop only.
package dsa

import "math/big"

type PublicKey struct {
	P *big.Int
	Q *big.Int
	G *big.Int
	Y *big.Int
}

type PrivateKey struct {
	P *big.Int
	Q *big.Int
	G *big.Int
	Y *big.Int
	X *big.Int
}

type Signature struct {
	R *big.Int
	S *big.Int
}

func SignRaw(priv *PrivateKey, hash *big.Int, k *big.Int) *Signature {
	r := new(big.Int).Exp(priv.G, k, priv.P)
	r.Mod(r, priv.Q)
	kInv := new(big.Int).ModInverse(k, priv.Q)
	xr := new(big.Int).Mul(priv.X, r)
	sum := new(big.Int).Add(hash, xr)
	s := new(big.Int).Mul(kInv, sum)
	s.Mod(s, priv.Q)
	return &Signature{R: r, S: s}
}

func Public(priv *PrivateKey) *PublicKey {
	return &PublicKey{P: priv.P, Q: priv.Q, G: priv.G, Y: priv.Y}
}

func VerifyRaw(pub *PublicKey, hash *big.Int, sig *Signature) bool {
	if sig.R.Sign() <= 0 || sig.S.Sign() <= 0 {
		return false
	}
	if sig.R.Cmp(pub.Q) >= 0 || sig.S.Cmp(pub.Q) >= 0 {
		return false
	}
	w := new(big.Int).ModInverse(sig.S, pub.Q)
	if w == nil {
		return false
	}
	u1 := new(big.Int).Mul(hash, w)
	u1.Mod(u1, pub.Q)
	u2 := new(big.Int).Mul(sig.R, w)
	u2.Mod(u2, pub.Q)
	gu1 := new(big.Int).Exp(pub.G, u1, pub.P)
	yu2 := new(big.Int).Exp(pub.Y, u2, pub.P)
	v := new(big.Int).Mul(gu1, yu2)
	v.Mod(v, pub.P)
	v.Mod(v, pub.Q)
	return v.Cmp(sig.R) == 0
}
