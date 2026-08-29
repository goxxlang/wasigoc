// Bounded ECDH over this project's P-256. SharedSecret is the x
// coordinate of priv*peer; no X25519, no HKDF wrapping. Alice's
// SharedSecret(alice, bobPub) equals Bob's SharedSecret(bob, alicePub)
// -- verified that way, not against a NIST CAVP file.
package ecdh

import (
	"crypto/elliptic"
	"math/big"
)

type PrivateKey struct {
	D *big.Int
}

type PublicKey struct {
	X *big.Int
	Y *big.Int
}

func Public(priv *PrivateKey) *PublicKey {
	c := elliptic.P256()
	p := c.ScalarBaseMult(priv.D)
	return &PublicKey{X: p.X, Y: p.Y}
}

func SharedSecret(priv *PrivateKey, peer *PublicKey) []byte {
	c := elliptic.P256()
	p := c.ScalarMult(&elliptic.Point{X: peer.X, Y: peer.Y}, priv.D)
	return p.X.Bytes()
}
