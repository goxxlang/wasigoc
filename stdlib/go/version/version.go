// Tiny subset of go/version: parses/compares "goN.N[.N]"-shaped version
// strings (IsValid/Lang/Compare) -- real go/version also understands
// "devel" builds and pre-release suffixes; not handled here.
package version

import "strconv"

func parseParts(x string) ([]int, bool) {
	if len(x) < 3 || x[0] != 103 || x[1] != 111 { // "go"
		return nil, false
	}
	rest := x[2:]
	if rest == "" {
		return nil, false
	}
	var parts []int
	start := 0
	for i := 0; i <= len(rest); i++ {
		if i == len(rest) || rest[i] == 46 {
			seg := rest[start:i]
			if seg == "" {
				return nil, false
			}
			n, err := strconv.Atoi(seg)
			if err != nil {
				return nil, false
			}
			parts = append(parts, n)
			start = i + 1
		}
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}

// IsValid reports whether x looks like "goN[.N[.N]]".
func IsValid(x string) bool {
	_, ok := parseParts(x)
	return ok
}

// Lang returns the Go language version (major.minor only) of x, e.g.
// "go1.21" from "go1.21.3" -- x itself if it doesn't parse.
func Lang(x string) string {
	parts, ok := parseParts(x)
	if !ok || len(parts) < 2 {
		return x
	}
	return "go" + strconv.Itoa(parts[0]) + "." + strconv.Itoa(parts[1])
}

// Compare returns -1, 0, or +1 as x is less than, equal to, or greater
// than y, comparing numerically part by part (so "go1.9" < "go1.10").
// Returns 0 if either string doesn't parse.
func Compare(x string, y string) int {
	px, okx := parseParts(x)
	py, oky := parseParts(y)
	if !okx || !oky {
		return 0
	}
	n := len(px)
	if len(py) > n {
		n = len(py)
	}
	for i := 0; i < n; i++ {
		a := 0
		if i < len(px) {
			a = px[i]
		}
		b := 0
		if i < len(py) {
			b = py[i]
		}
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
	}
	return 0
}
