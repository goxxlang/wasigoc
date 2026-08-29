// Bounded subset of text/scanner: a generic (not Go-specific -- that's
// go/scanner) tokenizer for identifiers/ints/floats/quoted strings/raw
// strings/char literals/single-rune punctuation, with // and /* */
// comments always skipped. No `Mode` bitmask (comments are unconditionally
// skipped, never tokenized), no `Whitespace` bitmask, no `Error` hook, no
// `IsIdentRune` override. `Init` takes an `io.Reader` like real Go but
// bounded like encoding/csv's Reader -- slurps the whole input up front
// rather than truly streaming.
package scanner

import (
	"io"
	"unicode"
	"unicode/utf8"
)

const (
	EOF       = -1
	Ident     = -2
	Int       = -3
	Float     = -4
	Char      = -5
	String    = -6
	RawString = -7
)

type Position struct {
	Filename string
	Offset   int
	Line     int
	Column   int
}

type Scanner struct {
	Filename string

	src string
	pos int
	line int
	col  int

	tokStart int
	tokEnd   int
	tokLine  int
	tokCol   int
}

func (s *Scanner) Init(src io.Reader) *Scanner {
	data, _ := io.ReadAll(src)
	s.src = string(data)
	s.pos = 0
	s.line = 1
	s.col = 0
	return s
}

func isDigitAscii(r rune) bool {
	return r >= '0' && r <= '9'
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (s *Scanner) Peek() rune {
	if s.pos >= len(s.src) {
		return EOF
	}
	r, _ := utf8.DecodeRuneInString(s.src[s.pos:])
	return r
}

func (s *Scanner) Next() rune {
	if s.pos >= len(s.src) {
		return EOF
	}
	r, size := utf8.DecodeRuneInString(s.src[s.pos:])
	s.pos += size
	if r == '\n' {
		s.line++
		s.col = 0
	} else {
		s.col++
	}
	return r
}

func (s *Scanner) skipWhitespaceAndComments() {
	for {
		ch := s.Peek()
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			s.Next()
			continue
		}
		if ch == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '/' {
			s.Next()
			s.Next()
			for s.Peek() != '\n' && s.Peek() != EOF {
				s.Next()
			}
			continue
		}
		if ch == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*' {
			s.Next()
			s.Next()
			for {
				if s.Peek() == EOF {
					break
				}
				if s.Peek() == '*' {
					s.Next()
					if s.Peek() == '/' {
						s.Next()
						break
					}
					continue
				}
				s.Next()
			}
			continue
		}
		break
	}
}

func (s *Scanner) scanQuoted(quote rune) {
	s.Next()
	for {
		c := s.Peek()
		if c == EOF || c == quote {
			break
		}
		if c == '\\' {
			s.Next()
			if s.Peek() == EOF {
				break
			}
		}
		s.Next()
	}
	if s.Peek() == quote {
		s.Next()
	}
}

func (s *Scanner) Scan() rune {
	s.skipWhitespaceAndComments()
	ch := s.Peek()
	s.tokStart = s.pos
	s.tokLine = s.line
	s.tokCol = s.col + 1
	if ch == EOF {
		s.tokEnd = s.pos
		return EOF
	}
	if isIdentStart(ch) {
		s.Next()
		for isIdentPart(s.Peek()) {
			s.Next()
		}
		s.tokEnd = s.pos
		return Ident
	}
	if isDigitAscii(ch) {
		isFloat := false
		s.Next()
		for isDigitAscii(s.Peek()) {
			s.Next()
		}
		if s.Peek() == '.' {
			isFloat = true
			s.Next()
			for isDigitAscii(s.Peek()) {
				s.Next()
			}
		}
		if s.Peek() == 'e' || s.Peek() == 'E' {
			isFloat = true
			s.Next()
			if s.Peek() == '+' || s.Peek() == '-' {
				s.Next()
			}
			for isDigitAscii(s.Peek()) {
				s.Next()
			}
		}
		s.tokEnd = s.pos
		if isFloat {
			return Float
		}
		return Int
	}
	if ch == '"' {
		s.scanQuoted('"')
		s.tokEnd = s.pos
		return String
	}
	if ch == '\'' {
		s.scanQuoted('\'')
		s.tokEnd = s.pos
		return Char
	}
	if ch == '`' {
		s.Next()
		for s.Peek() != '`' && s.Peek() != EOF {
			s.Next()
		}
		if s.Peek() == '`' {
			s.Next()
		}
		s.tokEnd = s.pos
		return RawString
	}
	s.Next()
	s.tokEnd = s.pos
	return ch
}

func (s *Scanner) TokenText() string {
	return s.src[s.tokStart:s.tokEnd]
}

func (s *Scanner) Pos() Position {
	return Position{Filename: s.Filename, Offset: s.tokStart, Line: s.tokLine, Column: s.tokCol}
}
