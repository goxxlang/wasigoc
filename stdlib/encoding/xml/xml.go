// Bounded encoding/xml: Marshal only, via reflect -- exported struct
// fields only, in declaration order, no `xml:"..."` struct tags (same gap
// as encoding/json's Marshal: no real reflect.StructTag to parse a tag
// string from). No Unmarshal at all (needs settable/addressable reflect
// Values, the same bigger feature json's own Unmarshal-into-struct gap is
// blocked on). The root element's tag is the struct's own Go type name
// (`reflect.Value.Type().Name()`), matching real Go's own default when no
// `XMLName` field or tag overrides it. A nested struct field becomes a
// nested element (recursively); a scalar field becomes a leaf element
// named after the field, its text content escaped for the five XML
// special characters. No attributes (`xml:"...,attr"`), no `chardata`,
// no slice-of-struct repetition (a struct field of slice type isn't
// handled -- documented gap, same shape as encoding/json's own "no
// slice-of-struct or map-of-struct field support yet").
package xml

import (
	"errors"
	"reflect"
	"strconv"
)

func Marshal(v any) ([]byte, error) {
	rv := reflect.ValueOf(v)
	name := rv.Type().Name()
	if name == "" {
		name = "value"
	}
	return marshalElement(nil, name, rv)
}

func marshalElement(b []byte, name string, rv reflect.Value) ([]byte, error) {
	k := rv.Kind()
	if k == reflect.Struct {
		b = append(b, '<')
		b = append(b, name...)
		b = append(b, '>')
		n := rv.NumField()
		i := 0
		for i < n {
			fname := rv.FieldName(i)
			fv := rv.Field(i)
			nb, err := marshalElement(b, fname, fv)
			if err != nil {
				return b, err
			}
			b = nb
			i = i + 1
		}
		b = append(b, "</"...)
		b = append(b, name...)
		b = append(b, '>')
		return b, nil
	}
	text, err := marshalScalar(rv)
	if err != nil {
		return b, err
	}
	b = append(b, '<')
	b = append(b, name...)
	b = append(b, '>')
	b = append(b, escapeText(text)...)
	b = append(b, "</"...)
	b = append(b, name...)
	b = append(b, '>')
	return b, nil
}

func marshalScalar(rv reflect.Value) (string, error) {
	k := rv.Kind()
	if k == reflect.String {
		return rv.String(), nil
	}
	if k == reflect.Bool {
		if rv.Bool() {
			return "true", nil
		}
		return "false", nil
	}
	if k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64 {
		return strconv.FormatInt(rv.Int(), 10), nil
	}
	if k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 {
		return strconv.FormatInt(rv.Int(), 10), nil
	}
	if k == reflect.Float32 || k == reflect.Float64 {
		return formatFloat(rv.Float()), nil
	}
	return "", errors.New("xml: unsupported type for Marshal")
}

// formatFloat is the same bounded fixed-point-ish formatting encoding/json's
// own formatNumber uses (this project's strconv has no FormatFloat) -- good
// enough for the plain decimal values XML text content actually needs, not
// a general shortest-round-trip float formatter.
func formatFloat(f float64) string {
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

func escapeText(s string) string {
	var b []byte
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '&' {
			b = append(b, "&amp;"...)
		} else if c == '<' {
			b = append(b, "&lt;"...)
		} else if c == '>' {
			b = append(b, "&gt;"...)
		} else if c == '\'' {
			b = append(b, "&apos;"...)
		} else if c == '"' {
			b = append(b, "&quot;"...)
		} else {
			b = append(b, c)
		}
		i = i + 1
	}
	return string(b)
}
