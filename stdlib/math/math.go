// Tiny subset of math, implemented in Go (Sqrt is Newton).
package math

func Abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func Max(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func Min(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func Sqrt(x float64) float64 {
	if x == 0 {
		return 0
	}
	if x < 0 {
		return NaN()
	}
	z := x
	for i := 0; i < 20; i++ {
		z = z - (z*z-x)/(z+z)
	}
	return z
}

const MaxFloat64 = 1.7976931348623157e+308

// Inf/NaN are computed at runtime (not folded like a real Go constant
// division-by-zero would be) so IEEE754 float division actually produces
// +/-Inf and 0/0 actually produces NaN, instead of wasigoc rejecting a
// literal x/0.0 the way const evaluation would.
func Inf(sign int) float64 {
	z := 0.0
	if sign >= 0 {
		return 1.0 / z
	}
	return -1.0 / z
}

func NaN() float64 {
	z := 0.0
	return z / z
}

func IsNaN(f float64) bool {
	return f != f
}

func IsInf(f float64, sign int) bool {
	if sign > 0 {
		return f > MaxFloat64
	}
	if sign < 0 {
		return f < -MaxFloat64
	}
	return f > MaxFloat64 || f < -MaxFloat64
}

func Floor(x float64) float64 {
	i := int64(x)
	f := float64(i)
	if f > x {
		f = f - 1
	}
	return f
}

func Ceil(x float64) float64 {
	i := int64(x)
	f := float64(i)
	if f < x {
		f = f + 1
	}
	return f
}

func Trunc(x float64) float64 {
	i := int64(x)
	return float64(i)
}

func Mod(x float64, y float64) float64 {
	if y == 0 {
		return NaN()
	}
	q := Trunc(x / y)
	return x - q*y
}

func Copysign(x float64, y float64) float64 {
	ax := Abs(x)
	if y < 0 {
		return -ax
	}
	return ax
}

func Signbit(x float64) bool {
	return x < 0
}

// Exp: range-reduce x into [0, 1) by repeated halving, Taylor series there
// (fast convergence), then repeated squaring back -- plain Taylor around 0
// converges far too slowly (or not at all, in float64) for |x| much above 1.
func Exp(x float64) float64 {
	if x == 0 {
		return 1
	}
	neg := x < 0
	ax := x
	if neg {
		ax = -ax
	}
	k := 0
	for ax > 1 {
		ax = ax / 2
		k++
	}
	term := 1.0
	sum := 1.0
	for i := 1; i <= 30; i++ {
		term = term * ax / float64(i)
		sum = sum + term
	}
	for i := 0; i < k; i++ {
		sum = sum * sum
	}
	if neg {
		return 1 / sum
	}
	return sum
}

const ln2 = 0.6931471805599453

// Log (natural log): range-reduce x into [1, 2) by repeated doubling/
// halving, then the atanh-style series ln(x) = 2*atanh((x-1)/(x+1)) --
// converges quickly since (x-1)/(x+1) is small once x is near 1.
func Log(x float64) float64 {
	if x < 0 {
		return NaN()
	}
	if x == 0 {
		return Inf(-1)
	}
	k := 0
	for x >= 2 {
		x = x / 2
		k++
	}
	for x < 1 {
		x = x * 2
		k--
	}
	t := (x - 1) / (x + 1)
	t2 := t * t
	term := t
	sum := t
	for i := 1; i <= 25; i++ {
		term = term * t2
		n := float64(2*i + 1)
		sum = sum + term/n
	}
	return 2*sum + float64(k)*ln2
}

func Log2(x float64) float64 {
	return Log(x) / ln2
}

func Log10(x float64) float64 {
	return Log(x) / 2.302585092994046
}

func Pow(x float64, y float64) float64 {
	if y == 0 {
		return 1
	}
	if x == 0 {
		if y > 0 {
			return 0
		}
		return Inf(1)
	}
	if x < 0 {
		yi := int64(y)
		if float64(yi) == y {
			r := Exp(float64(yi) * Log(-x))
			if yi%2 != 0 {
				return -r
			}
			return r
		}
		return NaN()
	}
	return Exp(y * Log(x))
}

func Hypot(p float64, q float64) float64 {
	return Sqrt(p*p + q*q)
}
