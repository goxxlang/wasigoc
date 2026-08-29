// Tiny subset of net/mail: parsing a single "Name <addr@host>" or bare
// "addr@host" address, and a comma-separated address list. No full RFC
// 5322 header/date parsing (no ParseDate, no ReadMessage) -- just the
// address-header shape most callers actually need.
package mail

import "errors"

type Address struct {
	Name    string
	Address string
}

func isSpaceByte(c byte) bool {
	return c == 32 || c == 9
}

func trimSpace(s string) string {
	i := 0
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	j := len(s)
	for j > i && isSpaceByte(s[j-1]) {
		j--
	}
	return s[i:j]
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == 34 && s[len(s)-1] == 34 {
		return s[1 : len(s)-1]
	}
	return s
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func validAddrSpec(s string) bool {
	at := indexByte(s, 64)
	if at <= 0 || at == len(s)-1 {
		return false
	}
	if indexByte(s[at+1:], 64) >= 0 {
		return false
	}
	return true
}

// ParseAddress parses a single address, either "addr@host" or
// "Display Name <addr@host>".
func ParseAddress(s string) (*Address, error) {
	s = trimSpace(s)
	lt := indexByte(s, 60)
	if lt >= 0 {
		gt := indexByte(s, 62)
		if gt < lt {
			return nil, errors.New("mail: missing '>' in address")
		}
		name := trimQuotes(trimSpace(s[0:lt]))
		spec := trimSpace(s[lt+1 : gt])
		if !validAddrSpec(spec) {
			return nil, errors.New("mail: invalid address: " + spec)
		}
		return &Address{Name: name, Address: spec}, nil
	}
	if !validAddrSpec(s) {
		return nil, errors.New("mail: invalid address: " + s)
	}
	return &Address{Name: "", Address: s}, nil
}

func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 60 {
			depth++
		} else if c == 62 {
			depth--
		} else if c == 44 && depth == 0 {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ParseAddressList parses a comma-separated list of addresses.
func ParseAddressList(s string) ([]*Address, error) {
	parts := splitTopLevelComma(s)
	var out []*Address
	for i := 0; i < len(parts); i++ {
		p := trimSpace(parts[i])
		if p == "" {
			continue
		}
		a, err := ParseAddress(p)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (a *Address) String() string {
	if a.Name == "" {
		return a.Address
	}
	return a.Name + " <" + a.Address + ">"
}
