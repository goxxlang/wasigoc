// Tiny subset of go/scanner: a real Go tokenizer, including automatic
// semicolon insertion (Go's ASI rule -- a virtual ';' is emitted at a
// newline, or at EOF, following a token that could end a statement).
// Supports identifiers/keywords, decimal/hex/octal/binary integers,
// floats (no exponent-only starting with a digit oddities, no complex
// literals), interpreted "..." and raw `...` strings, 'c' rune literals
// (escape sequences are consumed but not decoded -- the literal text is
// returned as-is, same simplification stdlib/regexp and others use), //
// and /* */ comments (skipped from the token stream, but a block
// comment spanning a newline still triggers ASI the way a real newline
// would -- AND, additively, every comment's raw text+position is
// appended to Scanner.Comments as a side channel, for go/doc's
// comment-association to consume; Scan's own return values/behavior are
// completely unchanged by this, so every existing caller is unaffected).
// No error recovery list -- ErrorCount just counts problems and scanning
// continues past them on a best-effort basis.
package scanner

import "go/token"

// Comment is one // or /* */ comment captured verbatim (delimiters
// included, matching real Go's ast.Comment.Text) as a side effect of
// Scan skipping it -- an additive capability, not a change to Scan's
// own return values or behavior: every existing caller (go/parser and
// everything built on it) sees exactly the same token stream as before.
type Comment struct {
	Pos  token.Pos
	Text string
}

type Scanner struct {
	src        string
	ch         byte
	offset     int
	rdOffset   int
	insertSemi bool
	ErrorCount int
	Comments   []Comment
}

func New(src string) *Scanner {
	s := &Scanner{src: src}
	s.ch = 32
	s.offset = 0
	s.rdOffset = 0
	s.next()
	return s
}

func (s *Scanner) next() {
	if s.rdOffset < len(s.src) {
		s.offset = s.rdOffset
		s.ch = s.src[s.offset]
		s.rdOffset = s.rdOffset + 1
	} else {
		s.offset = len(s.src)
		s.ch = 0
	}
}

func (s *Scanner) peek() byte {
	if s.rdOffset < len(s.src) {
		return s.src[s.rdOffset]
	}
	return 0
}

func isLetter(ch byte) bool {
	return ch == 95 || (ch >= 97 && ch <= 122) || (ch >= 65 && ch <= 90)
}
func isDigit(ch byte) bool {
	return ch >= 48 && ch <= 57
}
func isHexDigit(ch byte) bool {
	return isDigit(ch) || (ch >= 97 && ch <= 102) || (ch >= 65 && ch <= 70)
}

func (s *Scanner) skipWhitespace() {
	for {
		if s.ch == 32 || s.ch == 9 || s.ch == 13 {
			s.next()
			continue
		}
		if s.ch == 10 && !s.insertSemi {
			s.next()
			continue
		}
		break
	}
}

func (s *Scanner) scanIdentifier() string {
	start := s.offset
	for isLetter(s.ch) || isDigit(s.ch) {
		s.next()
	}
	return s.src[start:s.offset]
}

func (s *Scanner) scanNumber() (token.Token, string) {
	start := s.offset
	tok := token.INT
	if s.ch == 48 && (s.peek() == 120 || s.peek() == 88) {
		s.next()
		s.next()
		for isHexDigit(s.ch) || s.ch == 95 {
			s.next()
		}
		return tok, s.src[start:s.offset]
	}
	if s.ch == 48 && (s.peek() == 111 || s.peek() == 79) {
		s.next()
		s.next()
		for (s.ch >= 48 && s.ch <= 55) || s.ch == 95 {
			s.next()
		}
		return tok, s.src[start:s.offset]
	}
	if s.ch == 48 && (s.peek() == 98 || s.peek() == 66) {
		s.next()
		s.next()
		for s.ch == 48 || s.ch == 49 || s.ch == 95 {
			s.next()
		}
		return tok, s.src[start:s.offset]
	}
	for isDigit(s.ch) || s.ch == 95 {
		s.next()
	}
	if s.ch == 46 {
		tok = token.FLOAT
		s.next()
		for isDigit(s.ch) || s.ch == 95 {
			s.next()
		}
	}
	if s.ch == 101 || s.ch == 69 {
		tok = token.FLOAT
		s.next()
		if s.ch == 43 || s.ch == 45 {
			s.next()
		}
		for isDigit(s.ch) {
			s.next()
		}
	}
	return tok, s.src[start:s.offset]
}

func (s *Scanner) scanString() string {
	start := s.offset
	s.next()
	for s.ch != 34 && s.ch != 0 {
		if s.ch == 92 {
			s.next()
			s.next()
			continue
		}
		s.next()
	}
	s.next()
	return s.src[start:s.offset]
}

func (s *Scanner) scanRawString() string {
	start := s.offset
	s.next()
	for s.ch != 96 && s.ch != 0 {
		s.next()
	}
	s.next()
	return s.src[start:s.offset]
}

func (s *Scanner) scanChar() string {
	start := s.offset
	s.next()
	for s.ch != 39 && s.ch != 0 {
		if s.ch == 92 {
			s.next()
			s.next()
			continue
		}
		s.next()
	}
	s.next()
	return s.src[start:s.offset]
}

func (s *Scanner) skipLineComment() {
	for s.ch != 10 && s.ch != 0 {
		s.next()
	}
}

// skipBlockComment consumes a /* ... */ comment (the caller has already
// checked s.ch=='/' and peek()=='*') and reports whether it contained a
// newline.
func (s *Scanner) skipBlockComment() bool {
	hasNewline := false
	s.next()
	s.next()
	for {
		if s.ch == 0 {
			break
		}
		if s.ch == 10 {
			hasNewline = true
		}
		if s.ch == 42 && s.peek() == 47 {
			s.next()
			s.next()
			break
		}
		s.next()
	}
	return hasNewline
}

// Scan returns the next token's position, kind, and literal text (empty
// for tokens with no meaningful literal, e.g. punctuation).
func (s *Scanner) Scan() (token.Pos, token.Token, string) {
	for {
		s.skipWhitespace()
		pos := token.Pos(s.offset)

		if isLetter(s.ch) {
			lit := s.scanIdentifier()
			tok := token.Lookup(lit)
			insertSemi := tok == token.IDENT || tok == token.BREAK || tok == token.CONTINUE ||
				tok == token.FALLTHROUGH || tok == token.RETURN
			s.insertSemi = insertSemi
			return pos, tok, lit
		}

		if isDigit(s.ch) {
			tok, lit := s.scanNumber()
			s.insertSemi = true
			return pos, tok, lit
		}

		ch := s.ch

		if ch == 0 {
			if s.insertSemi {
				s.insertSemi = false
				return pos, token.SEMICOLON, "\n"
			}
			return pos, token.EOF, ""
		}

		// skipWhitespace deliberately leaves a pending newline unconsumed
		// when insertSemi is true (that's the whole signal) -- Scan
		// turns it into a virtual semicolon here instead of skipping it.
		if ch == 10 {
			s.next()
			s.insertSemi = false
			return pos, token.SEMICOLON, "\n"
		}

		if ch == 34 {
			lit := s.scanString()
			s.insertSemi = true
			return pos, token.STRING, lit
		}
		if ch == 96 {
			lit := s.scanRawString()
			s.insertSemi = true
			return pos, token.STRING, lit
		}
		if ch == 39 {
			lit := s.scanChar()
			s.insertSemi = true
			return pos, token.CHAR, lit
		}

		if ch == 47 && s.peek() == 47 {
			start := s.offset
			s.skipLineComment()
			s.Comments = append(s.Comments, Comment{Pos: pos, Text: s.src[start:s.offset]})
			continue
		}
		if ch == 47 && s.peek() == 42 {
			start := s.offset
			precedingSemi := s.insertSemi
			hasNL := s.skipBlockComment()
			s.Comments = append(s.Comments, Comment{Pos: pos, Text: s.src[start:s.offset]})
			if hasNL && precedingSemi {
				s.insertSemi = false
				return pos, token.SEMICOLON, "\n"
			}
			continue
		}

		s.next()
		insertSemi := false
		tok := token.ILLEGAL
		if ch == 43 {
			if s.ch == 43 {
				s.next()
				tok = token.INC
				insertSemi = true
			} else if s.ch == 61 {
				s.next()
				tok = token.ADD_ASSIGN
			} else {
				tok = token.ADD
			}
		} else if ch == 45 {
			if s.ch == 45 {
				s.next()
				tok = token.DEC
				insertSemi = true
			} else if s.ch == 61 {
				s.next()
				tok = token.SUB_ASSIGN
			} else {
				tok = token.SUB
			}
		} else if ch == 42 {
			if s.ch == 61 {
				s.next()
				tok = token.MUL_ASSIGN
			} else {
				tok = token.MUL
			}
		} else if ch == 47 {
			if s.ch == 61 {
				s.next()
				tok = token.QUO_ASSIGN
			} else {
				tok = token.QUO
			}
		} else if ch == 37 {
			if s.ch == 61 {
				s.next()
				tok = token.REM_ASSIGN
			} else {
				tok = token.REM
			}
		} else if ch == 38 {
			if s.ch == 94 {
				s.next()
				if s.ch == 61 {
					s.next()
					tok = token.AND_NOT_ASSIGN
				} else {
					tok = token.AND_NOT
				}
			} else if s.ch == 38 {
				s.next()
				tok = token.LAND
			} else if s.ch == 61 {
				s.next()
				tok = token.AND_ASSIGN
			} else {
				tok = token.AND
			}
		} else if ch == 124 {
			if s.ch == 124 {
				s.next()
				tok = token.LOR
			} else if s.ch == 61 {
				s.next()
				tok = token.OR_ASSIGN
			} else {
				tok = token.OR
			}
		} else if ch == 94 {
			if s.ch == 61 {
				s.next()
				tok = token.XOR_ASSIGN
			} else {
				tok = token.XOR
			}
		} else if ch == 60 {
			if s.ch == 60 {
				s.next()
				if s.ch == 61 {
					s.next()
					tok = token.SHL_ASSIGN
				} else {
					tok = token.SHL
				}
			} else if s.ch == 45 {
				s.next()
				tok = token.ARROW
			} else if s.ch == 61 {
				s.next()
				tok = token.LEQ
			} else {
				tok = token.LSS
			}
		} else if ch == 62 {
			if s.ch == 62 {
				s.next()
				if s.ch == 61 {
					s.next()
					tok = token.SHR_ASSIGN
				} else {
					tok = token.SHR
				}
			} else if s.ch == 61 {
				s.next()
				tok = token.GEQ
			} else {
				tok = token.GTR
			}
		} else if ch == 61 {
			if s.ch == 61 {
				s.next()
				tok = token.EQL
			} else {
				tok = token.ASSIGN
			}
		} else if ch == 33 {
			if s.ch == 61 {
				s.next()
				tok = token.NEQ
			} else {
				tok = token.NOT
			}
		} else if ch == 58 {
			if s.ch == 61 {
				s.next()
				tok = token.DEFINE
			} else {
				tok = token.COLON
			}
		} else if ch == 46 {
			if s.ch == 46 && s.peek() == 46 {
				s.next()
				s.next()
				tok = token.ELLIPSIS
			} else {
				tok = token.PERIOD
			}
		} else if ch == 40 {
			tok = token.LPAREN
		} else if ch == 91 {
			tok = token.LBRACK
		} else if ch == 123 {
			tok = token.LBRACE
		} else if ch == 44 {
			tok = token.COMMA
		} else if ch == 41 {
			tok = token.RPAREN
			insertSemi = true
		} else if ch == 93 {
			tok = token.RBRACK
			insertSemi = true
		} else if ch == 125 {
			tok = token.RBRACE
			insertSemi = true
		} else if ch == 59 {
			tok = token.SEMICOLON
		} else {
			s.ErrorCount = s.ErrorCount + 1
			tok = token.ILLEGAL
		}
		s.insertSemi = insertSemi
		return pos, tok, ""
	}
}
