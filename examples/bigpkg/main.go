package main

import (
	"fmt"
	"math/big"
)

func main() {
	a, okA := new(big.Int).SetString("123456789012345678901234567890", 10)
	b, okB := new(big.Int).SetString("987654321098765432109876543210", 10)
	fmt.Println(okA)
	fmt.Println(okB)

	sum := new(big.Int).Add(a, b)
	fmt.Println(sum.String() == "1111111110111111111011111111100")

	diff := new(big.Int).Sub(a, b)
	fmt.Println(diff.String() == "-864197532086419753208641975320")
	fmt.Println(diff.Sign() == -1)

	prod := new(big.Int).Mul(a, b)
	fmt.Println(prod.String() == "121932631137021795226185032733622923332237463801111263526900")

	// 30! via repeated small-int multiplication.
	fact := big.NewInt(1)
	for i := int64(1); i <= 30; i++ {
		fact = new(big.Int).Mul(fact, big.NewInt(i))
	}
	fmt.Println(fact.String() == "265252859812191058636308480000000")

	// Cmp ordering.
	x := big.NewInt(-5)
	y := big.NewInt(3)
	fmt.Println(x.Cmp(y) < 0)
	fmt.Println(y.Cmp(x) > 0)

	p := big.NewInt(100)
	q := big.NewInt(100)
	fmt.Println(p.Cmp(q) == 0)

	r := big.NewInt(-100)
	s := big.NewInt(-50)
	fmt.Println(r.Cmp(s) < 0)

	// Zero handling: sign, subtracting to zero, negation of zero.
	zero := new(big.Int).Sub(a, a)
	fmt.Println(zero.String() == "0")
	fmt.Println(zero.Sign() == 0)

	negZero := new(big.Int).Neg(zero)
	fmt.Println(negZero.String() == "0")

	// A malformed string is a real error, not silently accepted.
	_, ok := new(big.Int).SetString("not-a-number", 10)
	fmt.Println(ok == false)

	// Negative SetInt64 round trip.
	neg := big.NewInt(-42)
	fmt.Println(neg.String() == "-42")

	// Euclidean DivMod: 0 <= m < |y| always, regardless of either sign.
	div1 := new(big.Int)
	mod1 := new(big.Int)
	div1.DivMod(big.NewInt(17), big.NewInt(5), mod1)
	fmt.Println(div1.String() == "3")
	fmt.Println(mod1.String() == "2")

	div2 := new(big.Int)
	mod2 := new(big.Int)
	div2.DivMod(big.NewInt(-7), big.NewInt(2), mod2)
	fmt.Println(div2.String() == "-4")
	fmt.Println(mod2.String() == "1")

	div3 := new(big.Int)
	mod3 := new(big.Int)
	div3.DivMod(big.NewInt(-7), big.NewInt(-2), mod3)
	fmt.Println(div3.String() == "4")
	fmt.Println(mod3.String() == "1")

	// Truncated QuoRem: remainder takes the dividend's sign instead.
	quo := new(big.Int)
	rem := new(big.Int)
	quo.QuoRem(big.NewInt(-7), big.NewInt(2), rem)
	fmt.Println(quo.String() == "-3")
	fmt.Println(rem.String() == "-1")

	exp1 := new(big.Int).Exp(big.NewInt(2), big.NewInt(10), nil)
	fmt.Println(exp1.String() == "1024")

	exp2 := new(big.Int).Exp(big.NewInt(7), big.NewInt(128), big.NewInt(13))
	fmt.Println(exp2.String() == "3")

	fact20, _ := new(big.Int).SetString("2432902008176640000", 10)
	q6 := new(big.Int).Div(fact20, big.NewInt(5040))
	fmt.Println(q6.String() == "482718652416000")

	// GCD + Bezout coefficients (a*x + b*y == gcd), checked against real
	// Go's own math/big.Int.GCD as the oracle, including its sign
	// conventions for negative a/b and its a==0/b==0 special cases.
	checkGCD := func(aStr string, bStr string, wantG string, wantX string, wantY string) {
		a, _ := new(big.Int).SetString(aStr, 10)
		b, _ := new(big.Int).SetString(bStr, 10)
		var g big.Int
		var x big.Int
		var y big.Int
		g.GCD(&x, &y, a, b)
		fmt.Println(g.String() == wantG && x.String() == wantX && y.String() == wantY)
	}
	checkGCD("48", "18", "6", "-1", "3")
	checkGCD("17", "5", "1", "-2", "7")
	checkGCD("0", "5", "5", "0", "1")
	checkGCD("5", "0", "5", "1", "0")
	checkGCD("0", "0", "0", "0", "0")
	checkGCD("-48", "18", "6", "1", "3")
	checkGCD("48", "-18", "6", "-1", "-3")
	checkGCD("-48", "-18", "6", "1", "-3")
	checkGCD("270", "192", "6", "5", "-7")
	checkGCD("1234567891011", "987654321", "3", "20738908", "-25923634785")

	// ModInverse, checked against real Go's own math/big.Int.ModInverse,
	// including the "not invertible" nil case (gcd(4,8) == 4, not 1).
	checkInv := func(gStr string, nStr string, want string) {
		g, _ := new(big.Int).SetString(gStr, 10)
		n, _ := new(big.Int).SetString(nStr, 10)
		var z big.Int
		res := z.ModInverse(g, n)
		if res == nil {
			fmt.Println(want == "nil")
			return
		}
		fmt.Println(res.String() == want)
	}
	checkInv("3", "11", "4")
	checkInv("17", "3120", "2753")
	checkInv("4", "8", "nil")
	checkInv("-3", "11", "7")
	checkInv("7", "587257", "83894")
}
