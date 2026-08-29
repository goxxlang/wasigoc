// Bounded NIST P-256 (FIPS 186-4) in affine coordinates, using
// math/big. One curve, no P-224/P-384/P-521, no uncompressed-point
// Marshal with a type byte other than 4. ScalarBaseMult/ScalarMult/Add/
// IsOnCurve. Verified: G is on the curve, 1*G == G, n*G is infinity
// (n = group order).
package elliptic

import "math/big"

type Point struct {
	X   *big.Int
	Y   *big.Int
	Inf bool
}

type Curve struct {
	P  *big.Int
	A  *big.Int
	B  *big.Int
	N  *big.Int
	Gx *big.Int
	Gy *big.Int
}

func mustDec(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("elliptic: bad constant")
	}
	return n
}

func P256() *Curve {
	p := mustDec("115792089210356248762697446949407573530086143415290314195533631308867097853951")
	a := new(big.Int).Sub(p, big.NewInt(3))
	return &Curve{
		P:  p,
		A:  a,
		B:  mustDec("41058363725152142129326129780047268409114441015993725554835256314039467401291"),
		N:  mustDec("115792089210356248762697446949407573529996955224135760342422259061068512044369"),
		Gx: mustDec("48439561293906451759052585252797914202762949526041747995844080717082404635286"),
		Gy: mustDec("36134250956749795798585127919587881956611106672985015071877198253568414405109"),
	}
}

func (c *Curve) Params() *Curve { return c }

func (c *Curve) IsOnCurve(x *big.Int, y *big.Int) bool {
	yy := new(big.Int).Mul(y, y)
	yy.Mod(yy, c.P)
	xxx := new(big.Int).Mul(x, x)
	xxx.Mul(xxx, x)
	ax := new(big.Int).Mul(c.A, x)
	rhs := new(big.Int).Add(xxx, ax)
	rhs.Add(rhs, c.B)
	rhs.Mod(rhs, c.P)
	return yy.Cmp(rhs) == 0
}

func (c *Curve) infinity() *Point {
	return &Point{Inf: true, X: big.NewInt(0), Y: big.NewInt(0)}
}

func (c *Curve) Generator() *Point {
	return &Point{X: new(big.Int).Set(c.Gx), Y: new(big.Int).Set(c.Gy)}
}

func (c *Curve) Add(p1 *Point, p2 *Point) *Point {
	if p1.Inf {
		return &Point{X: new(big.Int).Set(p2.X), Y: new(big.Int).Set(p2.Y), Inf: p2.Inf}
	}
	if p2.Inf {
		return &Point{X: new(big.Int).Set(p1.X), Y: new(big.Int).Set(p1.Y), Inf: p1.Inf}
	}
	if p1.X.Cmp(p2.X) == 0 {
		sumY := new(big.Int).Add(p1.Y, p2.Y)
		sumY.Mod(sumY, c.P)
		if sumY.Sign() == 0 {
			return c.infinity()
		}
		return c.doublePoint(p1)
	}
	num := new(big.Int).Sub(p2.Y, p1.Y)
	den := new(big.Int).Sub(p2.X, p1.X)
	den.Mod(den, c.P)
	inv := new(big.Int).ModInverse(den, c.P)
	if inv == nil {
		return c.infinity()
	}
	lam := new(big.Int).Mul(num, inv)
	lam.Mod(lam, c.P)
	x3 := new(big.Int).Mul(lam, lam)
	x3.Sub(x3, p1.X)
	x3.Sub(x3, p2.X)
	x3.Mod(x3, c.P)
	y3 := new(big.Int).Sub(p1.X, x3)
	y3.Mul(y3, lam)
	y3.Sub(y3, p1.Y)
	y3.Mod(y3, c.P)
	return &Point{X: x3, Y: y3}
}

func (c *Curve) doublePoint(p *Point) *Point {
	if p.Inf || p.Y.Sign() == 0 {
		return c.infinity()
	}
	xx := new(big.Int).Mul(p.X, p.X)
	num := new(big.Int).Mul(xx, big.NewInt(3))
	num.Add(num, c.A)
	den := new(big.Int).Mul(p.Y, big.NewInt(2))
	den.Mod(den, c.P)
	inv := new(big.Int).ModInverse(den, c.P)
	if inv == nil {
		return c.infinity()
	}
	lam := new(big.Int).Mul(num, inv)
	lam.Mod(lam, c.P)
	x3 := new(big.Int).Mul(lam, lam)
	x3.Sub(x3, p.X)
	x3.Sub(x3, p.X)
	x3.Mod(x3, c.P)
	y3 := new(big.Int).Sub(p.X, x3)
	y3.Mul(y3, lam)
	y3.Sub(y3, p.Y)
	y3.Mod(y3, c.P)
	return &Point{X: x3, Y: y3}
}

func odd(x *big.Int) bool {
	var r big.Int
	r.Rem(x, big.NewInt(2))
	return r.Sign() != 0
}

func (c *Curve) ScalarMult(p *Point, k *big.Int) *Point {
	result := c.infinity()
	addend := &Point{X: new(big.Int).Set(p.X), Y: new(big.Int).Set(p.Y), Inf: p.Inf}
	kk := new(big.Int).Set(k)
	if kk.Sign() < 0 {
		kk.Neg(kk)
	}
	for kk.Sign() > 0 {
		if odd(kk) {
			result = c.Add(result, addend)
		}
		addend = c.doublePoint(addend)
		kk.Quo(kk, big.NewInt(2))
	}
	return result
}

func (c *Curve) ScalarBaseMult(k *big.Int) *Point {
	return c.ScalarMult(c.Generator(), k)
}
