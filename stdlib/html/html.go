// EscapeString/UnescapeString only -- no HTML/template parsing.
// UnescapeString handles the handful of named entities real Go's own
// EscapeString can ever produce (amp/lt/gt/quot/#39), plus decimal
// (&#NNN;) and hex (&#xHH;) numeric character references for any
// codepoint up to 0x10FFFF -- not the full ~2000-entry named-entity
// table real Go's html package supports (nbsp, copy, times, ...), a
// real, bounded, documented gap.
package html

import "strconv"

func EscapeString(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '&' {
			out = append(out, []byte("&amp;")...)
		} else if c == '\'' {
			out = append(out, []byte("&#39;")...)
		} else if c == '<' {
			out = append(out, []byte("&lt;")...)
		} else if c == '>' {
			out = append(out, []byte("&gt;")...)
		} else if c == '"' {
			out = append(out, []byte("&#34;")...)
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// namedEntity maps the entity name (without & or ;) to its replacement
// text, for the small set real Go's own EscapeString can produce plus
// the other common HTML-source entities.
func namedEntity(name string) (string, bool) {
	if name == "amp" {
		return "&", true
	}
	if name == "lt" {
		return "<", true
	}
	if name == "gt" {
		return ">", true
	}
	if name == "quot" {
		return "\"", true
	}
	if name == "apos" {
		return "'", true
	}
	return "", false
}

func UnescapeString(s string) string {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			out = append(out, s[i])
			i = i + 1
			continue
		}
		semi := -1
		for j := i + 1; j < len(s) && j < i+32; j++ {
			if s[j] == ';' {
				semi = j
				break
			}
		}
		if semi == -1 {
			out = append(out, s[i])
			i = i + 1
			continue
		}
		body := s[i+1 : semi]
		if len(body) > 1 && body[0] == '#' {
			numPart := body[1:]
			var code int64
			var err error
			ok := true
			if len(numPart) > 1 && (numPart[0] == 'x' || numPart[0] == 'X') {
				hexPart := numPart[1:]
				for k := 0; k < len(hexPart); k++ {
					if !isHexDigit(hexPart[k]) {
						ok = false
					}
				}
				if ok && len(hexPart) > 0 {
					code, err = strconv.ParseInt(hexPart, 16, 32)
					ok = err == nil
				} else {
					ok = false
				}
			} else {
				for k := 0; k < len(numPart); k++ {
					if !isDigit(numPart[k]) {
						ok = false
					}
				}
				if ok && len(numPart) > 0 {
					code, err = strconv.ParseInt(numPart, 10, 32)
					ok = err == nil
				} else {
					ok = false
				}
			}
			if ok && code >= 0 && code <= 0x10FFFF {
				out = append(out, []byte(string(rune(code)))...)
				i = semi + 1
				continue
			}
			out = append(out, s[i])
			i = i + 1
			continue
		}
		repl, found := namedEntity(body)
		if found {
			out = append(out, []byte(repl)...)
			i = semi + 1
			continue
		}
		out = append(out, s[i])
		i = i + 1
	}
	return string(out)
}
