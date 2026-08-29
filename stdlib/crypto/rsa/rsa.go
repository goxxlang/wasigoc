// Bounded crypto/rsa: raw textbook modular exponentiation only -- c =
// m^E mod N to encrypt, m = c^D mod N to decrypt -- the single
// arithmetic operation every RSA padding scheme is built on top of, now
// possible at all because this project's `math/big.Int.Exp` supports a
// modulus. NOT real Go's actual public API (`EncryptPKCS1v15`/
// `DecryptPKCS1v15`/`GenerateKey`/etc. don't exist here) -- deliberately
// scoped down two ways real Go's own rsa package would never accept in
// production code, both stated plainly rather than glossed over:
//
//  1. No padding scheme (PKCS1v15/OAEP). Textbook RSA applied directly
//     to a message is deterministic and malleable -- exactly why real Go
//     removed bare Encrypt/Decrypt decades ago. `EncryptRaw`/`DecryptRaw`
//     are named to say so: this is the primitive, not something safe to
//     use as-is, the same "present for legacy/textbook interop only, not
//     hardened" framing this project already uses for `crypto/rc4`/
//     `crypto/des`/`crypto/aes`.
//  2. No `GenerateKey`. Real key generation needs two large probable
//     primes plus a modular inverse (`d = e^-1 mod phi(n)`) to derive D
//     from E -- this project's `math/big` has no `GCD`/modular-inverse
//     support at all (see its own tracker line), so a `PrivateKey` here
//     must be constructed directly from an externally supplied N/E/D
//     (e.g. via `big.Int.SetString`), not generated in-package.
//
// Verified against real Go itself (go1.26.4, installed locally): both
// the classic textbook example (p=61, q=53, n=3233, e=17, d=2753,
// m=65 -> c=2790) and a second, independently chosen vector (n=589,
// e=7, d=463, m=123 -> c=61) were computed with real Go's own
// `math/big.Int.Exp` first to confirm the arithmetic, then reproduced
// exactly by this port's `EncryptRaw`/`DecryptRaw`.
package rsa

import "math/big"

type PublicKey struct {
	N *big.Int
	E int
}

type PrivateKey struct {
	PublicKey
	D *big.Int
}

// EncryptRaw computes m^E mod N. m must already be less than N -- no
// padding is applied.
func EncryptRaw(pub *PublicKey, m *big.Int) *big.Int {
	e := big.NewInt(int64(pub.E))
	return new(big.Int).Exp(m, e, pub.N)
}

// DecryptRaw computes c^D mod N.
func DecryptRaw(priv *PrivateKey, c *big.Int) *big.Int {
	return new(big.Int).Exp(c, priv.D, priv.N)
}
