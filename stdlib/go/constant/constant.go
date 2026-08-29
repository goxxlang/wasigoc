// Bounded subset of go/constant: represents an untyped Go constant as one
// concrete `Value` struct (Kind + whichever field applies), not real Go's
// arbitrary-precision internal representation (big.Int/big.Rat/big.Float)
// behind a sealed interface -- `int64`/`float64` here, same bounded-numeric-
// precision precedent as this project's own `go/types`. No `Complex` kind
// (no complex number type in this compiler at all -- see the language-gap
// audit) and no `MakeFromLiteral`/arbitrary-precision `ToInt`/`ToFloat`
// conversions. `Kind.String()` doesn't exist (methods need a struct
// receiver here) -- use the free function `KindString` instead, same
// shape as `go/token`'s `TokenString`.
package constant

import "go/token"

type Kind int

const (
	Unknown Kind = iota
	Bool
	String
	Int
	Float
)

func KindString(k Kind) string {
	if k == Bool {
		return "Bool"
	}
	if k == String {
		return "String"
	}
	if k == Int {
		return "Int"
	}
	if k == Float {
		return "Float"
	}
	return "Unknown"
}

type Value struct {
	kind     Kind
	boolVal  bool
	strVal   string
	intVal   int64
	floatVal float64
}

func MakeUnknown() Value          { return Value{kind: Unknown} }
func MakeBool(b bool) Value       { return Value{kind: Bool, boolVal: b} }
func MakeString(s string) Value   { return Value{kind: String, strVal: s} }
func MakeInt64(x int64) Value     { return Value{kind: Int, intVal: x} }
func MakeFloat64(x float64) Value { return Value{kind: Float, floatVal: x} }

func (v Value) Kind() Kind {
	return v.kind
}

func BoolVal(v Value) bool {
	return v.boolVal
}

func StringVal(v Value) string {
	return v.strVal
}

func Int64Val(v Value) (int64, bool) {
	return v.intVal, v.kind == Int
}

func Float64Val(v Value) (float64, bool) {
	if v.kind == Int {
		return float64(v.intVal), true
	}
	return v.floatVal, v.kind == Float
}

func isNumeric(v Value) bool {
	return v.kind == Int || v.kind == Float
}

func asFloat(v Value) float64 {
	if v.kind == Int {
		return float64(v.intVal)
	}
	return v.floatVal
}

func BinaryOp(x Value, op token.Token, y Value) Value {
	if x.kind == Bool && y.kind == Bool {
		if op == token.LAND {
			return MakeBool(x.boolVal && y.boolVal)
		}
		if op == token.LOR {
			return MakeBool(x.boolVal || y.boolVal)
		}
		return MakeUnknown()
	}
	if x.kind == String && y.kind == String {
		if op == token.ADD {
			return MakeString(x.strVal + y.strVal)
		}
		return MakeUnknown()
	}
	if x.kind == Int && y.kind == Int {
		if op == token.ADD {
			return MakeInt64(x.intVal + y.intVal)
		}
		if op == token.SUB {
			return MakeInt64(x.intVal - y.intVal)
		}
		if op == token.MUL {
			return MakeInt64(x.intVal * y.intVal)
		}
		if op == token.QUO {
			if y.intVal == 0 {
				return MakeUnknown()
			}
			return MakeInt64(x.intVal / y.intVal)
		}
		if op == token.REM {
			if y.intVal == 0 {
				return MakeUnknown()
			}
			return MakeInt64(x.intVal % y.intVal)
		}
		if op == token.AND {
			return MakeInt64(x.intVal & y.intVal)
		}
		if op == token.OR {
			return MakeInt64(x.intVal | y.intVal)
		}
		if op == token.XOR {
			return MakeInt64(x.intVal ^ y.intVal)
		}
		return MakeUnknown()
	}
	if isNumeric(x) && isNumeric(y) {
		fx := asFloat(x)
		fy := asFloat(y)
		if op == token.ADD {
			return MakeFloat64(fx + fy)
		}
		if op == token.SUB {
			return MakeFloat64(fx - fy)
		}
		if op == token.MUL {
			return MakeFloat64(fx * fy)
		}
		if op == token.QUO {
			if fy == 0 {
				return MakeUnknown()
			}
			return MakeFloat64(fx / fy)
		}
		return MakeUnknown()
	}
	return MakeUnknown()
}

func UnaryOp(op token.Token, x Value, prec uint) Value {
	if op == token.SUB {
		if x.kind == Int {
			return MakeInt64(-x.intVal)
		}
		if x.kind == Float {
			return MakeFloat64(-x.floatVal)
		}
	}
	if op == token.NOT {
		if x.kind == Bool {
			return MakeBool(!x.boolVal)
		}
	}
	if op == token.XOR {
		if x.kind == Int {
			return MakeInt64(^x.intVal)
		}
	}
	return MakeUnknown()
}

func Compare(x Value, op token.Token, y Value) bool {
	if x.kind == Bool && y.kind == Bool {
		if op == token.EQL {
			return x.boolVal == y.boolVal
		}
		if op == token.NEQ {
			return x.boolVal != y.boolVal
		}
		return false
	}
	if x.kind == String && y.kind == String {
		if op == token.EQL {
			return x.strVal == y.strVal
		}
		if op == token.NEQ {
			return x.strVal != y.strVal
		}
		if op == token.LSS {
			return x.strVal < y.strVal
		}
		if op == token.LEQ {
			return x.strVal <= y.strVal
		}
		if op == token.GTR {
			return x.strVal > y.strVal
		}
		if op == token.GEQ {
			return x.strVal >= y.strVal
		}
		return false
	}
	if isNumeric(x) && isNumeric(y) {
		fx := asFloat(x)
		fy := asFloat(y)
		if op == token.EQL {
			return fx == fy
		}
		if op == token.NEQ {
			return fx != fy
		}
		if op == token.LSS {
			return fx < fy
		}
		if op == token.LEQ {
			return fx <= fy
		}
		if op == token.GTR {
			return fx > fy
		}
		if op == token.GEQ {
			return fx >= fy
		}
	}
	return false
}

func Sign(x Value) int {
	if x.kind == Int {
		if x.intVal > 0 {
			return 1
		}
		if x.intVal < 0 {
			return -1
		}
		return 0
	}
	if x.kind == Float {
		if x.floatVal > 0 {
			return 1
		}
		if x.floatVal < 0 {
			return -1
		}
		return 0
	}
	return 0
}
