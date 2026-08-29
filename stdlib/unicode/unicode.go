// Tiny subset of unicode: ASCII + Latin-1 classification (no full Unicode
// category tables -- ordinary ASCII text and Latin-1 accented letters only).
package unicode

const (
	MaxRune         = 1114111
	ReplacementChar = 65533
	MaxASCII        = 127
	MaxLatin1       = 255
)

func IsDigit(r rune) bool {
	return r >= 48 && r <= 57
}

func IsUpper(r rune) bool {
	if r >= 65 && r <= 90 {
		return true
	}
	if r >= 192 && r <= 222 && r != 215 {
		return true
	}
	return false
}

func IsLower(r rune) bool {
	if r >= 97 && r <= 122 {
		return true
	}
	if r >= 223 && r <= 255 && r != 247 {
		return true
	}
	return false
}

func IsLetter(r rune) bool {
	if r >= 65 && r <= 90 {
		return true
	}
	if r >= 97 && r <= 122 {
		return true
	}
	if r >= 192 && r <= 255 && r != 215 && r != 247 {
		return true
	}
	return false
}

func IsSpace(r rune) bool {
	if r == 32 || r == 9 || r == 10 || r == 11 || r == 12 || r == 13 {
		return true
	}
	if r == 133 || r == 160 {
		return true
	}
	return false
}

func IsPunct(r rune) bool {
	if r >= 33 && r <= 47 {
		return true
	}
	if r >= 58 && r <= 64 {
		return true
	}
	if r >= 91 && r <= 96 {
		return true
	}
	if r >= 123 && r <= 126 {
		return true
	}
	return false
}

func IsControl(r rune) bool {
	if r < 32 {
		return true
	}
	return r == 127
}

func IsPrint(r rune) bool {
	return !IsControl(r)
}

func IsNumber(r rune) bool {
	return IsDigit(r)
}

func ToUpper(r rune) rune {
	if r >= 97 && r <= 122 {
		return r - 32
	}
	if r >= 224 && r <= 254 && r != 247 {
		return r - 32
	}
	return r
}

func ToLower(r rune) rune {
	if r >= 65 && r <= 90 {
		return r + 32
	}
	if r >= 192 && r <= 222 && r != 215 {
		return r + 32
	}
	return r
}

func ToTitle(r rune) rune {
	return ToUpper(r)
}
