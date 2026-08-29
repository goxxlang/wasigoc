// Published, named, in-memory variables (originally an HTTP-served debug
// endpoint in real Go -- no net/http here, so this is just the variable-
// registry half: Publish/Get/Do, plus the four concrete Var kinds). `Var`
// is a real single-method interface (`String() string`); `Int`/`Float`/
// `String`/`Map` all implement it. Registration order is preserved (a
// parallel `[]string` alongside each `map[string]Var]`, since this
// project's `wasigo::Map` iteration order is unspecified, same as real
// Go's own maps -- see hash/crc32's header comment on the same point).
package expvar

import (
	"fmt"
	"strconv"
)

type Var interface {
	String() string
}

type Int struct {
	i int64
}

func (v *Int) Value() int64    { return v.i }
func (v *Int) String() string  { return strconv.FormatInt(v.i, 10) }
func (v *Int) Add(delta int64) { v.i = v.i + delta }
func (v *Int) Set(value int64) { v.i = value }

type Float struct {
	f float64
}

func (v *Float) Value() float64      { return v.f }
func (v *Float) String() string      { return fmt.Sprintf("%f", v.f) }
func (v *Float) Add(delta float64)   { v.f = v.f + delta }
func (v *Float) Set(value float64)   { v.f = value }

// Named StringVar, not String -- a struct named identically to its own
// String() method (needed to satisfy Var) becomes, post-codegen, a
// same-named C++ member function parsed as a constructor ("return type
// specification for constructor invalid"). Same naming trap as
// hash/fnv's Digest32/Digest64 -- rename the struct, not the method.
type StringVar struct {
	s string
}

func (v *StringVar) Value() string    { return v.s }
func (v *StringVar) String() string   { return strconv.Quote(v.s) }
func (v *StringVar) Set(value string) { v.s = value }

type KeyValue struct {
	Key   string
	Value Var
}

type Map struct {
	m    map[string]Var
	keys []string
}

func (v *Map) Init() *Map {
	v.m = make(map[string]Var)
	return v
}

func (v *Map) Get(key string) Var {
	val, ok := v.m[key]
	if !ok {
		return nil
	}
	return val
}

func (v *Map) Set(key string, av Var) {
	_, exists := v.m[key]
	if !exists {
		v.keys = append(v.keys, key)
	}
	v.m[key] = av
}

func (v *Map) Do(f func(KeyValue)) {
	for i := 0; i < len(v.keys); i++ {
		k := v.keys[i]
		f(KeyValue{Key: k, Value: v.m[k]})
	}
}

func (v *Map) String() string {
	s := "{"
	for i := 0; i < len(v.keys); i++ {
		if i > 0 {
			s = s + ", "
		}
		k := v.keys[i]
		s = s + strconv.Quote(k) + ": " + v.m[k].String()
	}
	s = s + "}"
	return s
}

var vars = make(map[string]Var)
var varKeys []string

func Publish(name string, v Var) {
	_, exists := vars[name]
	if !exists {
		varKeys = append(varKeys, name)
	}
	vars[name] = v
}

func Get(name string) Var {
	v, ok := vars[name]
	if !ok {
		return nil
	}
	return v
}

func NewInt(name string) *Int {
	v := &Int{}
	Publish(name, v)
	return v
}

func NewFloat(name string) *Float {
	v := &Float{}
	Publish(name, v)
	return v
}

func NewString(name string) *StringVar {
	v := &StringVar{}
	Publish(name, v)
	return v
}

func NewMap(name string) *Map {
	v := (&Map{}).Init()
	Publish(name, v)
	return v
}

func Do(f func(KeyValue)) {
	for i := 0; i < len(varKeys); i++ {
		k := varKeys[i]
		f(KeyValue{Key: k, Value: vars[k]})
	}
}
