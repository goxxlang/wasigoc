// Bounded arbitrary-precision Int: sign + magnitude, magnitude stored as
// a little-endian []uint32 of base-1,000,000 "limbs" (not Go's own
// base-2^32 internal representation -- decimal limbs make String/
// SetString trivial, at the cost of slightly denser packing; this
// project prefers a plain []T slice over any fixed-size array type
// throughout, same reasoning as hash/crc32's own header comment).
// Practical size bound, worth knowing rather than hitting by surprise:
// Mul's inner accumulation is exact in uint64 up to roughly 10,000
// limbs per operand (10,000 * 999999^2 is comfortably under 2^64) --
// far more than any real use of this package here needs, but not
// unlimited the way real math/big is.
//
// Implemented: NewInt, Set, SetInt64, SetString/String (base 10 only),
// Add, Sub, Mul, Neg, Cmp, Sign, Quo/Rem (truncated division), DivMod/
// Div/Mod (Euclidean -- 0 <= m < |y|, matching real Go's own documented
// "unlike Go" convention for these four specifically), Exp (square-and-
// multiply, with an optional modulus, built on Quo/Rem's own long division
// rather than a separate binary-exponentiation-on-bits path -- there is
// no cheap way to pull individual bits out of this package's DECIMAL limb
// representation, so exponent bits come from repeatedly dividing the
// exponent by two instead, reusing already-verified code rather than
// adding a second, parallel bit-extraction implementation). Division is
// schoolbook long division one base-1,000,000 limb at a time, each limb's
// quotient digit found by binary search over [0, 999999] (cmpMag/
// mulMagSmall per probe) rather than trial-and-error digit estimation --
// correct and simple, at the cost of ~20 extra multiply/compare passes
// per output limb, an easy trade for numbers this small. Also
// implemented: GCD (textbook extended Euclid, not real Go's Lehmer's-
// algorithm fast path -- see GCD's own doc comment) and ModInverse, built
// on it. NOT implemented: Exp with a negative exponent (would need
// ModInverse wired through it, not attempted), bases other than 10,
// Text/Append variants.
package big

import "strconv"

const limbBase = 1000000
const limbDigits = 6

type Int struct {
	neg   bool
	limbs []uint32
}

func normalize(limbs []uint32) []uint32 {
	n := len(limbs)
	for n > 0 && limbs[n-1] == 0 {
		n = n - 1
	}
	return limbs[0:n]
}

func cmpMag(a []uint32, b []uint32) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func addMag(a []uint32, b []uint32) []uint32 {
	if len(a) < len(b) {
		a, b = b, a
	}
	out := make([]uint32, len(a)+1)
	var carry uint32 = 0
	for i := 0; i < len(a); i++ {
		var bv uint32 = 0
		if i < len(b) {
			bv = b[i]
		}
		sum := a[i] + bv + carry
		if sum >= limbBase {
			sum = sum - limbBase
			carry = 1
		} else {
			carry = 0
		}
		out[i] = sum
	}
	out[len(a)] = carry
	return normalize(out)
}

// subMag requires len(a magnitude) >= len(b magnitude) (cmpMag(a,b) >= 0).
func subMag(a []uint32, b []uint32) []uint32 {
	out := make([]uint32, len(a))
	var borrow int32 = 0
	for i := 0; i < len(a); i++ {
		var bv int32 = 0
		if i < len(b) {
			bv = int32(b[i])
		}
		d := int32(a[i]) - bv - borrow
		if d < 0 {
			d = d + limbBase
			borrow = 1
		} else {
			borrow = 0
		}
		out[i] = uint32(d)
	}
	return normalize(out)
}

func mulMag(a []uint32, b []uint32) []uint32 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	acc := make([]uint64, len(a)+len(b))
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			acc[i+j] = acc[i+j] + uint64(a[i])*uint64(b[j])
		}
	}
	var carry uint64 = 0
	out := make([]uint32, len(acc))
	for k := 0; k < len(acc); k++ {
		v := acc[k] + carry
		out[k] = uint32(v % limbBase)
		carry = v / limbBase
	}
	return normalize(out)
}

func mulMagSmall(a []uint32, m uint32) []uint32 {
	if m == 0 || len(a) == 0 {
		return nil
	}
	out := make([]uint32, len(a)+1)
	var carry uint64 = 0
	for i := 0; i < len(a); i++ {
		v := uint64(a[i])*uint64(m) + carry
		out[i] = uint32(v % limbBase)
		carry = v / limbBase
	}
	out[len(a)] = uint32(carry)
	return normalize(out)
}

// divModMag is schoolbook long division: bring down one limb of a at a
// time into a running remainder, then binary-search the quotient digit
// for that position in [0, limbBase-1]. Caller guarantees b is non-zero.
func divModMag(a []uint32, b []uint32) ([]uint32, []uint32) {
	if cmpMag(a, b) < 0 {
		return nil, append([]uint32{}, a...)
	}
	quotient := make([]uint32, len(a))
	var rem []uint32
	for i := len(a) - 1; i >= 0; i-- {
		shifted := make([]uint32, len(rem)+1)
		shifted[0] = a[i]
		copy(shifted[1:], rem)
		rem = normalize(shifted)

		lo := 0
		hi := limbBase - 1
		q := 0
		for lo <= hi {
			mid := (lo + hi) / 2
			prod := mulMagSmall(b, uint32(mid))
			if cmpMag(prod, rem) <= 0 {
				q = mid
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		quotient[i] = uint32(q)
		rem = subMag(rem, mulMagSmall(b, uint32(q)))
	}
	return normalize(quotient), rem
}

func NewInt(x int64) *Int {
	z := &Int{}
	z.SetInt64(x)
	return z
}

// SetInt64 does not handle math.MinInt64 specially (its magnitude
// doesn't fit in a plain negation) -- a documented, narrow gap, not a
// silent wrong answer for any other value.
func (z *Int) SetInt64(x int64) *Int {
	neg := x < 0
	var ux uint64
	if neg {
		ux = uint64(-x)
	} else {
		ux = uint64(x)
	}
	var limbs []uint32
	for ux > 0 {
		limbs = append(limbs, uint32(ux%limbBase))
		ux = ux / limbBase
	}
	z.limbs = limbs
	z.neg = neg && len(limbs) > 0
	return z
}

func (z *Int) Set(x *Int) *Int {
	z.neg = x.neg
	z.limbs = append([]uint32{}, x.limbs...)
	return z
}

// SetBytes interprets buf as a big-endian unsigned integer and sets
// z to that value (sign always positive). Empty buf is zero.
func (z *Int) SetBytes(buf []byte) *Int {
	z.neg = false
	z.limbs = nil
	i := 0
	for i < len(buf) {
		z.Mul(z, NewInt(256))
		z.Add(z, NewInt(int64(buf[i])))
		i = i + 1
	}
	return z
}

// Bytes returns the absolute value of z as a big-endian byte slice,
// matching real Go's own Int.Bytes (no sign bit, no leading zeros;
// zero is a zero-length slice).
func (z *Int) Bytes() []byte {
	if len(z.limbs) == 0 {
		return nil
	}
	var out []byte
	x := &Int{}
	x.Set(z)
	x.neg = false
	two56 := NewInt(256)
	for x.Sign() > 0 {
		var r Int
		x.QuoRem(x, two56, &r)
		b := byte(0)
		if len(r.limbs) > 0 {
			b = byte(r.limbs[0])
		}
		out = append([]byte{b}, out...)
	}
	if len(out) == 0 {
		return []byte{0}
	}
	return out
}

func (z *Int) Add(x *Int, y *Int) *Int {
	if x.neg == y.neg {
		z.limbs = addMag(x.limbs, y.limbs)
		z.neg = x.neg && len(z.limbs) > 0
		return z
	}
	c := cmpMag(x.limbs, y.limbs)
	if c == 0 {
		z.limbs = nil
		z.neg = false
	} else if c > 0 {
		z.limbs = subMag(x.limbs, y.limbs)
		z.neg = x.neg
	} else {
		z.limbs = subMag(y.limbs, x.limbs)
		z.neg = y.neg
	}
	return z
}

func negOf(y *Int) *Int {
	if len(y.limbs) == 0 {
		return &Int{}
	}
	return &Int{neg: !y.neg, limbs: y.limbs}
}

func (z *Int) Sub(x *Int, y *Int) *Int {
	return z.Add(x, negOf(y))
}

func (z *Int) Mul(x *Int, y *Int) *Int {
	z.limbs = mulMag(x.limbs, y.limbs)
	z.neg = (x.neg != y.neg) && len(z.limbs) > 0
	return z
}

func (z *Int) Neg(x *Int) *Int {
	z.limbs = append([]uint32{}, x.limbs...)
	z.neg = !x.neg && len(z.limbs) > 0
	return z
}

// QuoRem sets z to the truncated quotient x/y and r to x - y*z (r has the
// same sign as x, or is zero), matching real Go's Quo/Rem pair exactly.
func (z *Int) QuoRem(x *Int, y *Int, r *Int) (*Int, *Int) {
	if len(y.limbs) == 0 {
		panic("math/big: division by zero")
	}
	qMag, rMag := divModMag(x.limbs, y.limbs)
	r.limbs = rMag
	r.neg = x.neg && len(r.limbs) > 0
	z.limbs = qMag
	z.neg = (x.neg != y.neg) && len(z.limbs) > 0
	return z, r
}

func (z *Int) Quo(x *Int, y *Int) *Int {
	var r Int
	z.QuoRem(x, y, &r)
	return z
}

func (z *Int) Rem(x *Int, y *Int) *Int {
	var q Int
	q.QuoRem(x, y, z)
	return z
}

// DivMod implements Euclidean division (unlike Go's truncated Quo/Rem,
// same "unlike Go" naming real math/big itself uses): m always satisfies
// 0 <= m < |y|, adjusting the truncated quotient/remainder from QuoRem by
// one step when the truncated remainder came out negative.
func (z *Int) DivMod(x *Int, y *Int, m *Int) (*Int, *Int) {
	var q Int
	q.QuoRem(x, y, m)
	if len(m.limbs) > 0 && x.neg {
		yAbs := &Int{limbs: y.limbs}
		m.Add(m, yAbs)
		one := NewInt(1)
		if y.neg {
			q.Add(&q, one)
		} else {
			q.Sub(&q, one)
		}
	}
	z.Set(&q)
	return z, m
}

func (z *Int) Div(x *Int, y *Int) *Int {
	var m Int
	z.DivMod(x, y, &m)
	return z
}

func (z *Int) Mod(x *Int, y *Int) *Int {
	var q Int
	q.DivMod(x, y, z)
	return z
}

// Exp computes x**y, reduced mod m if m is non-nil and positive (plain
// x**y otherwise). Square-and-multiply, pulling exponent bits out via
// repeated division by two (see the package comment for why -- this
// package's decimal limbs have no cheap bitwise view). Negative y is NOT
// supported (would need a modular inverse); the loop below simply never
// runs, so z is left as 1 -- a documented gap, not a silent wrong answer
// for a case any caller here is expected to hit.
func (z *Int) Exp(x *Int, y *Int, m *Int) *Int {
	useMod := m != nil && m.Sign() > 0
	result := NewInt(1)
	base := &Int{}
	base.Set(x)
	if useMod {
		base.Mod(base, m)
	}
	exp := &Int{}
	exp.Set(y)
	two := NewInt(2)
	for exp.Sign() > 0 {
		var rem Int
		var q Int
		q.QuoRem(exp, two, &rem)
		if rem.Sign() != 0 {
			result.Mul(result, base)
			if useMod {
				result.Mod(result, m)
			}
		}
		base.Mul(base, base)
		if useMod {
			base.Mod(base, m)
		}
		exp.Set(&q)
	}
	z.Set(result)
	return z
}

func (x *Int) Cmp(y *Int) int {
	if x.neg != y.neg {
		if len(x.limbs) == 0 && len(y.limbs) == 0 {
			return 0
		}
		if x.neg {
			return -1
		}
		return 1
	}
	c := cmpMag(x.limbs, y.limbs)
	if x.neg {
		return -c
	}
	return c
}

func (x *Int) Sign() int {
	if len(x.limbs) == 0 {
		return 0
	}
	if x.neg {
		return -1
	}
	return 1
}

func (x *Int) String() string {
	if len(x.limbs) == 0 {
		return "0"
	}
	s := ""
	if x.neg {
		s = "-"
	}
	s = s + strconv.Itoa(int(x.limbs[len(x.limbs)-1]))
	for i := len(x.limbs) - 2; i >= 0; i-- {
		d := strconv.Itoa(int(x.limbs[i]))
		for len(d) < limbDigits {
			d = "0" + d
		}
		s = s + d
	}
	return s
}

// SetString parses a base-10 (only -- other bases return ok=false)
// signed decimal string.
func (z *Int) SetString(s string, base int) (*Int, bool) {
	if base != 10 && base != 0 {
		return nil, false
	}
	if len(s) == 0 {
		return nil, false
	}
	neg := false
	i := 0
	if s[0] == '-' {
		neg = true
		i = 1
	} else if s[0] == '+' {
		i = 1
	}
	digits := s[i:]
	if len(digits) == 0 {
		return nil, false
	}
	for k := 0; k < len(digits); k++ {
		if digits[k] < '0' || digits[k] > '9' {
			return nil, false
		}
	}
	start := 0
	for start < len(digits)-1 && digits[start] == '0' {
		start = start + 1
	}
	digits = digits[start:]
	if digits == "0" {
		z.limbs = nil
		z.neg = false
		return z, true
	}
	var limbs []uint32
	end := len(digits)
	for end > 0 {
		chunkStart := end - limbDigits
		if chunkStart < 0 {
			chunkStart = 0
		}
		chunk := digits[chunkStart:end]
		v, _ := strconv.Atoi(chunk)
		limbs = append(limbs, uint32(v))
		end = chunkStart
	}
	z.limbs = normalize(limbs)
	z.neg = neg && len(z.limbs) > 0
	return z, true
}

// GCD sets z to the greatest common divisor of |a| and |b| and returns z.
// If x or y are non-nil, they are set to Bezout coefficients satisfying
// a*x + b*y = z (real Go's own documented behavior, including the sign
// convention and the a==0/b==0 special cases below). a and b may be
// positive, negative, or zero; z is always >= 0.
//
// Textbook extended Euclid built on this package's own already-verified
// Quo/Sub/Mul (the same "reuse already-correct long division rather than
// a parallel bit-level implementation" choice Exp's own header comment
// explains for this decimal-limb representation) -- not real Go's own
// Lehmer's-algorithm fast path, which operates on machine words this
// package's base-1,000,000 limbs don't have a cheap word-level view into.
// Slower than real Go's for very large inputs, not a concern at the sizes
// this project's callers (crypto/rsa's modular inverse needs, elliptic
// curve point arithmetic) actually use.
func (z *Int) GCD(x *Int, y *Int, a *Int, b *Int) *Int {
	if len(a.limbs) == 0 || len(b.limbs) == 0 {
		if len(a.limbs) == 0 {
			z.Set(b)
		} else {
			z.Set(a)
		}
		z.neg = false
		if x != nil {
			if len(a.limbs) == 0 {
				x.SetInt64(0)
			} else {
				x.SetInt64(1)
				x.neg = a.neg
			}
		}
		if y != nil {
			if len(b.limbs) == 0 {
				y.SetInt64(0)
			} else {
				y.SetInt64(1)
				y.neg = b.neg
			}
		}
		return z
	}

	oldR := &Int{}
	oldR.Set(a)
	oldR.neg = false
	r := &Int{}
	r.Set(b)
	r.neg = false
	oldS := NewInt(1)
	s := NewInt(0)
	oldT := NewInt(0)
	t := NewInt(1)

	for r.Sign() != 0 {
		q := &Int{}
		q.Quo(oldR, r)

		qr := &Int{}
		qr.Mul(q, r)
		newR := &Int{}
		newR.Sub(oldR, qr)
		oldR.Set(r)
		r.Set(newR)

		qs := &Int{}
		qs.Mul(q, s)
		newS := &Int{}
		newS.Sub(oldS, qs)
		oldS.Set(s)
		s.Set(newS)

		qt := &Int{}
		qt.Mul(q, t)
		newT := &Int{}
		newT.Sub(oldT, qt)
		oldT.Set(t)
		t.Set(newT)
	}

	z.Set(oldR)
	if x != nil {
		x.Set(oldS)
		if a.neg {
			x.Neg(x)
		}
	}
	if y != nil {
		y.Set(oldT)
		if b.neg {
			y.Neg(y)
		}
	}
	return z
}

// ModInverse sets z to the multiplicative inverse of g in the ring
// ZZ/nZZ and returns z, or returns nil if g and n are not relatively
// prime (no inverse exists). n must not be zero.
func (z *Int) ModInverse(g *Int, n *Int) *Int {
	nn := n
	if n.neg {
		nn = &Int{}
		nn.Neg(n)
	}
	gg := g
	if g.neg {
		gg = &Int{}
		gg.Mod(g, nn)
	}
	var d Int
	var x Int
	d.GCD(&x, nil, gg, nn)

	one := NewInt(1)
	if d.Cmp(one) != 0 {
		return nil
	}

	if x.neg {
		z.Add(&x, nn)
	} else {
		z.Set(&x)
	}
	return z
}
