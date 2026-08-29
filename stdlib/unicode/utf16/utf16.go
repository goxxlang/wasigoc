// Tiny subset of unicode/utf16.
package utf16

const (
	replacementChar = 65533
	maxRune         = 1114111
	surr1           = 55296
	surr2           = 56320
	surr3           = 57344
	surrSelf        = 65536
)

func IsSurrogate(r rune) bool {
	return surr1 <= r && r < surr3
}

func RuneLen(r rune) int {
	if r < 0 || (surr1 <= r && r < surr3) || r > maxRune {
		return -1
	}
	if r >= surrSelf {
		return 2
	}
	return 1
}

func EncodeRune(r rune) (rune, rune) {
	if r < surrSelf || r > maxRune {
		return replacementChar, replacementChar
	}
	r = r - surrSelf
	return rune(surr1 + (r>>10)&1023), rune(surr2 + r&1023)
}

func DecodeRune(r1 rune, r2 rune) rune {
	if surr1 <= r1 && r1 < surr2 && surr2 <= r2 && r2 < surr3 {
		return (r1-surr1)<<10 | (r2 - surr2) + surrSelf
	}
	return replacementChar
}

func Encode(s []rune) []uint16 {
	var out []uint16
	for i := 0; i < len(s); i++ {
		r := s[i]
		if 0 <= r && r < surr1 || surr3 <= r && r < surrSelf {
			out = append(out, uint16(r))
		} else if surrSelf <= r && r <= maxRune {
			r1, r2 := EncodeRune(r)
			out = append(out, uint16(r1))
			out = append(out, uint16(r2))
		} else {
			out = append(out, uint16(replacementChar))
		}
	}
	return out
}

func Decode(s []uint16) []rune {
	var out []rune
	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		if surr1 <= r && r < surr2 && i+1 < len(s) {
			r2 := rune(s[i+1])
			if surr2 <= r2 && r2 < surr3 {
				out = append(out, DecodeRune(r, r2))
				i++
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
