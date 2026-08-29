// Tiny subset of net/url: QueryEscape/QueryUnescape/PathEscape and a
// simple URL struct covering the common "scheme://host/path?query#frag"
// shape. No userinfo, no percent-decoding host/port split, no query
// multi-value Values type (just a flat map[string]string via ParseQuery).
package url

import "errors"

func isUnreserved(c byte) bool {
	if c >= 65 && c <= 90 {
		return true
	}
	if c >= 97 && c <= 122 {
		return true
	}
	if c >= 48 && c <= 57 {
		return true
	}
	return c == 45 || c == 95 || c == 46 || c == 126
}

const hexDigits = "0123456789ABCDEF"

func QueryEscape(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			out = append(out, c)
			continue
		}
		if c == 32 {
			out = append(out, byte(43))
			continue
		}
		out = append(out, byte(37))
		out = append(out, hexDigits[c>>4])
		out = append(out, hexDigits[c&15])
	}
	return string(out)
}

func PathEscape(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) || c == 47 {
			out = append(out, c)
			continue
		}
		out = append(out, byte(37))
		out = append(out, hexDigits[c>>4])
		out = append(out, hexDigits[c&15])
	}
	return string(out)
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

func QueryUnescape(s string) (string, error) {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 43 {
			out = append(out, byte(32))
			continue
		}
		if c == 37 {
			if i+2 >= len(s) {
				return "", errInvalidEscape
			}
			hi := hexVal(s[i+1])
			lo := hexVal(s[i+2])
			if hi < 0 || lo < 0 {
				return "", errInvalidEscape
			}
			out = append(out, byte(hi<<4|lo))
			i = i + 2
			continue
		}
		out = append(out, c)
	}
	return string(out), nil
}

func PathUnescape(s string) (string, error) {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 37 {
			if i+2 >= len(s) {
				return "", errInvalidEscape
			}
			hi := hexVal(s[i+1])
			lo := hexVal(s[i+2])
			if hi < 0 || lo < 0 {
				return "", errInvalidEscape
			}
			out = append(out, byte(hi<<4|lo))
			i = i + 2
			continue
		}
		out = append(out, c)
	}
	return string(out), nil
}

type URL struct {
	Scheme   string
	Host     string
	Path     string
	RawQuery string
	Fragment string
}

func splitOnce(s string, sep string) (string, string, bool) {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return s[0:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

func Parse(rawURL string) (*URL, error) {
	u := &URL{}
	rest := rawURL
	if scheme, after, ok := splitOnce(rest, "://"); ok {
		u.Scheme = scheme
		rest = after
	}
	if before, after, ok := splitOnce(rest, "#"); ok {
		rest = before
		u.Fragment = after
	}
	if before, after, ok := splitOnce(rest, "?"); ok {
		rest = before
		u.RawQuery = after
	}
	if u.Scheme != "" {
		if slash, after, ok := splitOnce(rest, "/"); ok {
			u.Host = slash
			u.Path = "/" + after
		} else {
			u.Host = rest
		}
	} else {
		u.Path = rest
	}
	return u, nil
}

func (u *URL) String() string {
	out := ""
	if u.Scheme != "" {
		out = out + u.Scheme + "://" + u.Host
	}
	out = out + u.Path
	if u.RawQuery != "" {
		out = out + "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out = out + "#" + u.Fragment
	}
	return out
}

// ParseQuery: a flat "last value wins" map, not real Go's multi-value
// url.Values.
func ParseQuery(rawQuery string) (map[string]string, error) {
	out := make(map[string]string)
	if rawQuery == "" {
		return out, nil
	}
	start := 0
	for start <= len(rawQuery) {
		end := start
		for end < len(rawQuery) && rawQuery[end] != 38 {
			end++
		}
		pair := rawQuery[start:end]
		if pair != "" {
			key := pair
			val := ""
			if k, v, ok := splitOnce(pair, "="); ok {
				key = k
				val = v
			}
			dk, err := QueryUnescape(key)
			if err != nil {
				return out, err
			}
			dv, err2 := QueryUnescape(val)
			if err2 != nil {
				return out, err2
			}
			out[dk] = dv
		}
		start = end + 1
	}
	return out, nil
}

var errInvalidEscape = errors.New("invalid URL escape")
