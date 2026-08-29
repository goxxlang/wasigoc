// Tiny subset of encoding/json: encodes/decodes the *generic* decoded-JSON
// value shape (nil, bool, float64, int, string, []any, map[string]any) --
// parsing arbitrary JSON into `any`/`map[string]any`/`[]any` and
// serializing that same shape back out covers a real and common use case
// (config files, API responses of unknown/dynamic shape). Map keys are
// sorted when marshaling (real Go's json.Marshal does this too).
//
// Marshal ALSO supports arbitrary structs now, via reflect (exported
// fields only, in declaration order -- no `json:"..."` struct tags, no
// omitempty, since there's nowhere to parse a tag string from without a
// real reflect.StructTag). Unmarshal does NOT support decoding into an
// arbitrary struct pointer -- that needs settable/addressable reflect
// Values (pointer-based field mutation), a bigger feature than the
// read-only reflect this compiler has; only *any/*map[string]any/*[]any/
// *string/*float64/*bool targets work.
package json

import (
	"errors"
	"reflect"
	"sort"
	"strconv"
)

func Marshal(v any) ([]byte, error) {
	return marshalValue(nil, v)
}

func marshalValue(b []byte, v any) ([]byte, error) {
	if v == nil {
		return append(b, "null"...), nil
	}
	if bv, ok := v.(bool); ok {
		if bv {
			return append(b, "true"...), nil
		}
		return append(b, "false"...), nil
	}
	if sv, ok := v.(string); ok {
		return marshalString(b, sv), nil
	}
	if fv, ok := v.(float64); ok {
		return append(b, formatNumber(fv)...), nil
	}
	if iv, ok := v.(int); ok {
		return append(b, strconv.Itoa(iv)...), nil
	}
	if av, ok := v.([]any); ok {
		return marshalArray(b, av)
	}
	if mv, ok := v.(map[string]any); ok {
		return marshalObject(b, mv)
	}
	return marshalReflect(b, v)
}

// marshalReflect handles everything marshalValue's direct type
// assertions don't: a struct (recursively, field by field), and any
// numeric-family type other than plain "int"/"float64" (int8/16/32/64,
// uint/8/16/32/64, float32) -- those all lower to a *different* C++ type
// than "int"/"float64" here, so `v.(int)` above never matches them even
// though they're still integers/floats from Go's point of view.
func marshalReflect(b []byte, v any) ([]byte, error) {
	rv := reflect.ValueOf(v)
	k := rv.Kind()
	if k == reflect.Struct {
		return marshalStruct(b, rv)
	}
	if k == reflect.Bool {
		if rv.Bool() {
			return append(b, "true"...), nil
		}
		return append(b, "false"...), nil
	}
	if k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64 {
		return append(b, strconv.FormatInt(rv.Int(), 10)...), nil
	}
	if k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 {
		return append(b, strconv.FormatInt(rv.Int(), 10)...), nil
	}
	if k == reflect.Float32 || k == reflect.Float64 {
		return append(b, formatNumber(rv.Float())...), nil
	}
	if k == reflect.String {
		return marshalString(b, rv.String()), nil
	}
	return b, errors.New("json: unsupported type for Marshal (nil/bool/numbers/string/[]any/" +
		"map[string]any/struct -- no slice-of-struct or map-of-struct field support yet)")
}

func marshalStruct(b []byte, rv reflect.Value) ([]byte, error) {
	b = append(b, byte(123))
	n := rv.NumField()
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, byte(44))
		}
		b = marshalString(b, rv.FieldName(i))
		b = append(b, byte(58))
		nb, err := marshalValue(b, rv.Field(i).Interface())
		if err != nil {
			return b, err
		}
		b = nb
	}
	b = append(b, byte(125))
	return b, nil
}

func marshalString(b []byte, s string) []byte {
	b = append(b, byte(34))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 34 {
			b = append(b, byte(92), byte(34))
		} else if c == 92 {
			b = append(b, byte(92), byte(92))
		} else if c == 10 {
			b = append(b, byte(92), byte(110))
		} else if c == 13 {
			b = append(b, byte(92), byte(114))
		} else if c == 9 {
			b = append(b, byte(92), byte(116))
		} else if c < 32 {
			b = append(b, byte(92), byte(117), byte(48), byte(48))
			b = append(b, hexDigits[c>>4], hexDigits[c&15])
		} else {
			b = append(b, c)
		}
	}
	b = append(b, byte(34))
	return b
}

const hexDigits = "0123456789abcdef"

func formatNumber(f float64) string {
	if f != f {
		return "null"
	}
	neg := f < 0
	if neg {
		f = -f
	}
	whole := int64(f)
	frac := f - float64(whole)
	out := strconv.FormatInt(whole, 10)
	if frac > 0.0000001 {
		scaled := int64(frac*1000000.0 + 0.5)
		fs := strconv.FormatInt(scaled, 10)
		for len(fs) < 6 {
			fs = "0" + fs
		}
		end := len(fs)
		for end > 0 && fs[end-1:end] == "0" {
			end--
		}
		if end > 0 {
			out = out + "." + fs[0:end]
		}
	}
	if neg {
		out = "-" + out
	}
	return out
}

func marshalArray(b []byte, arr []any) ([]byte, error) {
	b = append(b, byte(91))
	for i := 0; i < len(arr); i++ {
		if i > 0 {
			b = append(b, byte(44))
		}
		nb, err := marshalValue(b, arr[i])
		if err != nil {
			return b, err
		}
		b = nb
	}
	b = append(b, byte(93))
	return b, nil
}

func marshalObject(b []byte, m map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b = append(b, byte(123))
	for i := 0; i < len(keys); i++ {
		if i > 0 {
			b = append(b, byte(44))
		}
		b = marshalString(b, keys[i])
		b = append(b, byte(58))
		nb, err := marshalValue(b, m[keys[i]])
		if err != nil {
			return b, err
		}
		b = nb
	}
	b = append(b, byte(125))
	return b, nil
}

// Unmarshal supports these targets (a pointer to each): any, map[string]any,
// []any, string, float64, bool -- not an arbitrary struct pointer (needs
// reflection to know its fields; see the package comment).
func Unmarshal(data []byte, v any) error {
	p := &parser{s: string(data)}
	val, err := p.parseValue()
	if err != nil {
		return err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return errors.New("json: unexpected trailing data after the JSON value")
	}
	switch ptr := v.(type) {
	case *any:
		*ptr = val
		return nil
	case *map[string]any:
		m, ok := val.(map[string]any)
		if !ok {
			return errors.New("json: cannot unmarshal into map[string]any")
		}
		*ptr = m
		return nil
	case *[]any:
		a, ok := val.([]any)
		if !ok {
			return errors.New("json: cannot unmarshal into []any")
		}
		*ptr = a
		return nil
	case *string:
		s, ok := val.(string)
		if !ok {
			return errors.New("json: cannot unmarshal into string")
		}
		*ptr = s
		return nil
	case *float64:
		f, ok := val.(float64)
		if !ok {
			return errors.New("json: cannot unmarshal into float64")
		}
		*ptr = f
		return nil
	case *bool:
		b, ok := val.(bool)
		if !ok {
			return errors.New("json: cannot unmarshal into bool")
		}
		*ptr = b
		return nil
	}
	return errors.New("json: unsupported Unmarshal target (no reflection -- only *any, " +
		"*map[string]any, *[]any, *string, *float64, *bool)")
}

type parser struct {
	s   string
	pos int
}

func (p *parser) skipSpace() {
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == 32 || c == 9 || c == 10 || c == 13 {
			p.pos++
			continue
		}
		break
	}
}

func (p *parser) parseValue() (any, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return nil, errors.New("json: unexpected end of input")
	}
	c := p.s[p.pos]
	if c == 123 {
		return p.parseObject()
	}
	if c == 91 {
		return p.parseArray()
	}
	if c == 34 {
		return p.parseString()
	}
	if c == 116 {
		return p.parseLiteral("true", true)
	}
	if c == 102 {
		return p.parseLiteral("false", false)
	}
	if c == 110 {
		return p.parseLiteral("null", nil)
	}
	if c == 45 || (c >= 48 && c <= 57) {
		return p.parseNumber()
	}
	return nil, errors.New("json: unexpected character in input")
}

func (p *parser) parseLiteral(lit string, val any) (any, error) {
	if p.pos+len(lit) > len(p.s) || p.s[p.pos:p.pos+len(lit)] != lit {
		return nil, errors.New("json: invalid literal")
	}
	p.pos = p.pos + len(lit)
	return val, nil
}

func (p *parser) parseObject() (any, error) {
	p.pos++
	m := make(map[string]any)
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == 125 {
		p.pos++
		return m, nil
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != 34 {
			return nil, errors.New("json: expected a string key")
		}
		key, err := p.parseString()
		if err != nil {
			return nil, err
		}
		ks, _ := key.(string)
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != 58 {
			return nil, errors.New("json: expected ':' after object key")
		}
		p.pos++
		val, err2 := p.parseValue()
		if err2 != nil {
			return nil, err2
		}
		m[ks] = val
		p.skipSpace()
		if p.pos >= len(p.s) {
			return nil, errors.New("json: unexpected end of object")
		}
		if p.s[p.pos] == 44 {
			p.pos++
			continue
		}
		if p.s[p.pos] == 125 {
			p.pos++
			break
		}
		return nil, errors.New("json: expected ',' or '}' in object")
	}
	return m, nil
}

func (p *parser) parseArray() (any, error) {
	p.pos++
	var arr []any
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == 93 {
		p.pos++
		return arr, nil
	}
	for {
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		arr = append(arr, val)
		p.skipSpace()
		if p.pos >= len(p.s) {
			return nil, errors.New("json: unexpected end of array")
		}
		if p.s[p.pos] == 44 {
			p.pos++
			continue
		}
		if p.s[p.pos] == 93 {
			p.pos++
			break
		}
		return nil, errors.New("json: expected ',' or ']' in array")
	}
	return arr, nil
}

func hexVal(c byte) int {
	if c >= 48 && c <= 57 {
		return int(c - 48)
	}
	if c >= 97 && c <= 102 {
		return int(c-97) + 10
	}
	if c >= 65 && c <= 70 {
		return int(c-65) + 10
	}
	return -1
}

func parseHex4(s string) (int, error) {
	n := 0
	for i := 0; i < 4; i++ {
		d := hexVal(s[i])
		if d < 0 {
			return 0, errors.New("json: invalid \\u escape")
		}
		n = n*16 + d
	}
	return n, nil
}

func (p *parser) parseString() (any, error) {
	p.pos++
	var out []byte
	for {
		if p.pos >= len(p.s) {
			return nil, errors.New("json: unterminated string")
		}
		c := p.s[p.pos]
		if c == 34 {
			p.pos++
			break
		}
		if c == 92 {
			p.pos++
			if p.pos >= len(p.s) {
				return nil, errors.New("json: unterminated escape")
			}
			e := p.s[p.pos]
			if e == 34 {
				out = append(out, byte(34))
			} else if e == 92 {
				out = append(out, byte(92))
			} else if e == 47 {
				out = append(out, byte(47))
			} else if e == 110 {
				out = append(out, byte(10))
			} else if e == 116 {
				out = append(out, byte(9))
			} else if e == 114 {
				out = append(out, byte(13))
			} else if e == 98 {
				out = append(out, byte(8))
			} else if e == 102 {
				out = append(out, byte(12))
			} else if e == 117 {
				if p.pos+4 >= len(p.s) {
					return nil, errors.New("json: invalid \\u escape")
				}
				r, err := parseHex4(p.s[p.pos+1 : p.pos+5])
				if err != nil {
					return nil, err
				}
				out = append(out, string(rune(r))...)
				p.pos = p.pos + 4
			} else {
				return nil, errors.New("json: invalid escape sequence")
			}
			p.pos++
			continue
		}
		out = append(out, c)
		p.pos++
	}
	return string(out), nil
}

func (p *parser) parseNumber() (any, error) {
	start := p.pos
	if p.pos < len(p.s) && p.s[p.pos] == 45 {
		p.pos++
	}
	for p.pos < len(p.s) && p.s[p.pos] >= 48 && p.s[p.pos] <= 57 {
		p.pos++
	}
	if p.pos < len(p.s) && p.s[p.pos] == 46 {
		p.pos++
		for p.pos < len(p.s) && p.s[p.pos] >= 48 && p.s[p.pos] <= 57 {
			p.pos++
		}
	}
	// Scientific notation is consumed (to keep the parser in sync with the
	// rest of a larger document) but strconv.ParseFloat here doesn't
	// support exponents (see README's stdlib tracker), so a number written
	// that way will surface as an error rather than a silently wrong value.
	if p.pos < len(p.s) && (p.s[p.pos] == 101 || p.s[p.pos] == 69) {
		p.pos++
		if p.pos < len(p.s) && (p.s[p.pos] == 43 || p.s[p.pos] == 45) {
			p.pos++
		}
		for p.pos < len(p.s) && p.s[p.pos] >= 48 && p.s[p.pos] <= 57 {
			p.pos++
		}
	}
	numStr := p.s[start:p.pos]
	f, err := strconv.ParseFloat(numStr)
	if err != nil {
		return nil, errors.New("json: invalid number " + numStr)
	}
	return f, nil
}
