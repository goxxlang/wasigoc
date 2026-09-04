// Bounded Ed25519 (RFC 8032): Sign and Verify on the twisted Edwards
// curve, field arithmetic via math/big (slow, correct). No batch
// verify, no cofactor-clearing variants. Verified by Sign-then-Verify
// round trip on a short message.
package ed25519

import (
	"crypto/sha512"
	"math/big"
)

const PublicKeySize = 32
const PrivateKeySize = 32
const SignatureSize = 64

var pEd *big.Int
var dEd *big.Int
var lEd *big.Int
var bxEd *big.Int
var byEd *big.Int

func init() {
	pEd, _ = new(big.Int).SetString("57896044618658097711785492504343953926634992332820282019728792003956564819949", 10)
	lEd, _ = new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)
	dEd, _ = new(big.Int).SetString("37095705934669439343138083508754565189542113879843219016388785533085940283555", 10)
	bxEd, _ = new(big.Int).SetString("15112221349535400772501151409588531511454012693041857206046113283949847762202", 10)
	byEd, _ = new(big.Int).SetString("46316835694926478169428394003475163141307993866256225615783033603165251855960", 10)
}

type point struct {
	X *big.Int
	Y *big.Int
	Z bool
}

func feAdd(a *big.Int, b *big.Int) *big.Int {
	z := new(big.Int).Add(a, b)
	z.Mod(z, pEd)
	return z
}

func feSub(a *big.Int, b *big.Int) *big.Int {
	z := new(big.Int).Sub(a, b)
	z.Mod(z, pEd)
	return z
}

func feMul(a *big.Int, b *big.Int) *big.Int {
	z := new(big.Int).Mul(a, b)
	z.Mod(z, pEd)
	return z
}

func feSqr(a *big.Int) *big.Int { return feMul(a, a) }

func feInv(a *big.Int) *big.Int {
	return new(big.Int).ModInverse(a, pEd)
}

func feOdd(a *big.Int) bool {
	var r big.Int
	r.Rem(a, big.NewInt(2))
	return r.Sign() != 0
}

func feSqrt(a *big.Int) *big.Int {
	exp := new(big.Int).Add(pEd, big.NewInt(3))
	exp.Quo(exp, big.NewInt(8))
	x := new(big.Int).Exp(a, exp, pEd)
	if feSqr(x).Cmp(a) != 0 {
		iexp := new(big.Int).Sub(pEd, big.NewInt(1))
		iexp.Quo(iexp, big.NewInt(4))
		ii := new(big.Int).Exp(big.NewInt(2), iexp, pEd)
		x = feMul(x, ii)
	}
	return x
}

func recoverX(y *big.Int, sign int) *big.Int {
	yy := feSqr(y)
	num := feSub(yy, big.NewInt(1))
	den := feAdd(feMul(dEd, yy), big.NewInt(1))
	x2 := feMul(num, feInv(den))
	x := feSqrt(x2)
	want := sign == 1
	if feOdd(x) != want {
		x = feSub(pEd, x)
	}
	return x
}

func ident() *point {
	return &point{X: big.NewInt(0), Y: big.NewInt(1)}
}

func basePoint() *point {
	return &point{X: new(big.Int).Set(bxEd), Y: new(big.Int).Set(byEd)}
}

func ptAdd(p *point, q *point) *point {
	if p.Z {
		return &point{X: new(big.Int).Set(q.X), Y: new(big.Int).Set(q.Y), Z: q.Z}
	}
	if q.Z {
		return &point{X: new(big.Int).Set(p.X), Y: new(big.Int).Set(p.Y), Z: p.Z}
	}
	x1y2 := feMul(p.X, q.Y)
	y1x2 := feMul(p.Y, q.X)
	y1y2 := feMul(p.Y, q.Y)
	x1x2 := feMul(p.X, q.X)
	dxxyy := feMul(dEd, feMul(x1x2, y1y2))
	x3 := feMul(feAdd(x1y2, y1x2), feInv(feAdd(big.NewInt(1), dxxyy)))
	y3 := feMul(feAdd(y1y2, x1x2), feInv(feSub(big.NewInt(1), dxxyy)))
	return &point{X: x3, Y: y3}
}

func ptDouble(p *point) *point { return ptAdd(p, p) }

func scalarMult(p *point, k *big.Int) *point {
	result := ident()
	addend := &point{X: new(big.Int).Set(p.X), Y: new(big.Int).Set(p.Y)}
	kk := new(big.Int).Set(k)
	for kk.Sign() > 0 {
		if feOdd(kk) {
			result = ptAdd(result, addend)
		}
		addend = ptDouble(addend)
		kk.Quo(kk, big.NewInt(2))
	}
	return result
}

func fromLE(b []byte) *big.Int {
	r := make([]byte, len(b))
	i := 0
	for i < len(b) {
		r[len(b)-1-i] = b[i]
		i = i + 1
	}
	return new(big.Int).SetBytes(r)
}

func toLE32(z *big.Int) []byte {
	be := z.Bytes()
	out := make([]byte, 32)
	i := 0
	for i < len(be) && i < 32 {
		out[i] = be[len(be)-1-i]
		i = i + 1
	}
	return out
}

func encodePoint(p *point) []byte {
	out := toLE32(p.Y)
	if feOdd(p.X) {
		out[31] = out[31] | 128
	}
	return out
}

func decodePoint(b []byte) *point {
	if len(b) != 32 {
		return nil
	}
	cp := append([]byte{}, b...)
	sign := 0
	if cp[31]&128 != 0 {
		sign = 1
	}
	cp[31] = cp[31] & 127
	y := fromLE(cp)
	if y.Cmp(pEd) >= 0 {
		return nil
	}
	x := recoverX(y, sign)
	return &point{X: x, Y: y}
}

func reduceL(h []byte) *big.Int {
	n := fromLE(h)
	n.Mod(n, lEd)
	return n
}

func clampScalar(seed []byte) *big.Int {
	a := append([]byte{}, seed...)
	a[0] = a[0] & 248
	a[31] = a[31] & 127
	a[31] = a[31] | 64
	return fromLE(a)
}

func PublicKey(priv []byte) []byte {
	h := sha512.Sum(priv)
	a := clampScalar(h[0:32])
	return encodePoint(scalarMult(basePoint(), a))
}

func Sign(priv []byte, message []byte) []byte {
	h := sha512.Sum(priv)
	a := clampScalar(h[0:32])
	prefix := h[32:64]
	A := encodePoint(scalarMult(basePoint(), a))
	r := reduceL(sha512.Sum(append(append([]byte{}, prefix...), message...)))
	Rpt := scalarMult(basePoint(), r)
	R := encodePoint(Rpt)
	k := reduceL(sha512.Sum(append(append(append([]byte{}, R...), A...), message...)))
	s := new(big.Int).Mul(k, a)
	s.Add(s, r)
	s.Mod(s, lEd)
	return append(R, toLE32(s)...)
}

func Verify(public []byte, message []byte, sig []byte) bool {
	if len(public) != 32 || len(sig) != 64 {
		return false
	}
	A := decodePoint(public)
	R := decodePoint(sig[0:32])
	if A == nil || R == nil {
		return false
	}
	S := fromLE(sig[32:64])
	if S.Cmp(lEd) >= 0 {
		return false
	}
	k := reduceL(sha512.Sum(append(append(append([]byte{}, sig[0:32]...), public...), message...)))
	sB := scalarMult(basePoint(), S)
	kA := scalarMult(A, k)
	rhs := ptAdd(R, kA)
	return sB.X.Cmp(rhs.X) == 0 && sB.Y.Cmp(rhs.Y) == 0
}
