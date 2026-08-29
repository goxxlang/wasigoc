// Tiny subset of unicode/utf8. Range-over-string already decodes UTF-8
// the way Go does (invalid sequences are U+FFFD and consume one byte).
package utf8

const (
	RuneError = 65533 // U+FFFD
	RuneSelf  = 128
)

func RuneCountInString(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func RuneLen(r rune) int {
	if r < 0 {
		return -1
	}
	if r <= 127 {
		return 1
	}
	if r <= 2047 {
		return 2
	}
	if r <= 65535 {
		return 3
	}
	if r <= 1114111 {
		return 4
	}
	return -1
}

func EncodeRune(p []byte, r rune) int {
	n := string(r)
	for i := 0; i < len(n); i++ {
		p[i] = n[i]
	}
	return len(n)
}

func Valid(p []byte) bool {
	return ValidString(string(p))
}

func ValidString(s string) bool {
	for _, r := range s {
		if r == RuneError {
			return false
		}
	}
	return true
}

func ValidRune(r rune) bool {
	if r < 0 || r > 1114111 {
		return false
	}
	if r >= 55296 && r <= 57343 {
		return false
	}
	return true
}

func DecodeRuneInString(s string) (r rune, size int) {
	if len(s) == 0 {
		return 65533, 0
	}
	n := 0
	size = len(s)
	for i, rr := range s {
		if n == 0 {
			r = rr
			n = 1
		} else {
			size = i
			break
		}
	}
	return
}
