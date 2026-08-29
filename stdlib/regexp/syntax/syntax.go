// Bounded subset of regexp/syntax: parses a pattern into a real `Regexp`
// AST (`Op`-tagged, `Sub`/`Rune` fields -- same shape real Go's own
// package uses), bounded to exactly the pattern subset this project's own
// `regexp` package supports (see its header comment): literals, `.`, `*`
// `+` `?` (greedy only), character classes `[...]`/`[^...]` with ranges
// and `\d \D \w \W \s \S` shorthand, `^` `$` anchors, `|` alternation,
// `(...)` capturing groups. No `{m,n}` counted repetition, no non-greedy,
// no lookaround, no `Flags` bitmask (`Parse`'s second argument is accepted
// for signature compatibility but ignored). This is a SEPARATE, independent
// parser from `regexp`'s own internal one -- not wired in to back it, since
// `regexp`'s `node` type is unexported and this package's public `Regexp`
// shape is deliberately different (real Go's own `regexp` package does
// share `regexp/syntax` internally; this project's bounded `regexp`
// predates this package and keeps its own simpler parser rather than being
// retrofitted).
//
// Char classes are byte-range based (like this project's own `regexp`, not
// full Unicode rune classes) -- `Rune` holds paired [lo, hi] byte-range
// values (0-255) even though its declared element type is `rune`, matching
// real regexp/syntax's own "Rune holds paired lo/hi values for CharClass"
// convention. A negated class (`[^...]`/`\D`/`\W`/`\S`) is expanded into
// its literal complement ranges over 0-255 at parse time, exactly like
// real Go -- there is no separate "negate" flag on `Regexp`.
package syntax

import "errors"

type Op int

const (
	OpLiteral Op = iota
	OpCharClass
	OpAnyChar
	OpBeginLine
	OpEndLine
	OpStar
	OpPlus
	OpQuest
	OpConcat
	OpAlternate
	OpCapture
)

type Regexp struct {
	Op   Op
	Sub  []*Regexp
	Rune []rune
	Cap  int
}

type Flags int

const Perl Flags = 0

type parser struct {
	s      string
	pos    int
	numCap int
}

func (p *parser) peek() byte {
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

func Parse(pattern string, flags Flags) (*Regexp, error) {
	p := &parser{s: pattern}
	re, err := p.parseAlt()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.s) {
		return nil, newSyntaxError("unexpected ')'")
	}
	return re, nil
}

func newSyntaxError(msg string) error {
	return errors.New("regexp/syntax: " + msg)
}

func (p *parser) parseAlt() (*Regexp, error) {
	first, err := p.parseConcat()
	if err != nil {
		return nil, err
	}
	alts := []*Regexp{first}
	for p.peek() == 124 {
		p.pos = p.pos + 1
		n, err2 := p.parseConcat()
		if err2 != nil {
			return nil, err2
		}
		alts = append(alts, n)
	}
	if len(alts) == 1 {
		return first, nil
	}
	return &Regexp{Op: OpAlternate, Sub: alts}, nil
}

func (p *parser) parseConcat() (*Regexp, error) {
	var parts []*Regexp
	for p.pos < len(p.s) && p.peek() != 124 && p.peek() != 41 {
		n, err := p.parseRepeat()
		if err != nil {
			return nil, err
		}
		parts = append(parts, n)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return &Regexp{Op: OpConcat, Sub: parts}, nil
}

func (p *parser) parseRepeat() (*Regexp, error) {
	a, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for {
		c := p.peek()
		if c == 42 {
			p.pos = p.pos + 1
			a = &Regexp{Op: OpStar, Sub: []*Regexp{a}}
			continue
		}
		if c == 43 {
			p.pos = p.pos + 1
			a = &Regexp{Op: OpPlus, Sub: []*Regexp{a}}
			continue
		}
		if c == 63 {
			p.pos = p.pos + 1
			a = &Regexp{Op: OpQuest, Sub: []*Regexp{a}}
			continue
		}
		break
	}
	return a, nil
}

func (p *parser) parseAtom() (*Regexp, error) {
	c := p.peek()
	if c == 40 {
		p.pos = p.pos + 1
		p.numCap = p.numCap + 1
		capIdx := p.numCap
		n, err := p.parseAlt()
		if err != nil {
			return nil, err
		}
		if p.peek() != 41 {
			return nil, newSyntaxError("missing closing )")
		}
		p.pos = p.pos + 1
		return &Regexp{Op: OpCapture, Sub: []*Regexp{n}, Cap: capIdx}, nil
	}
	if c == 46 {
		p.pos = p.pos + 1
		return &Regexp{Op: OpAnyChar}, nil
	}
	if c == 94 {
		p.pos = p.pos + 1
		return &Regexp{Op: OpBeginLine}, nil
	}
	if c == 36 {
		p.pos = p.pos + 1
		return &Regexp{Op: OpEndLine}, nil
	}
	if c == 91 {
		return p.parseClass()
	}
	if c == 92 {
		p.pos = p.pos + 1
		e := p.peek()
		p.pos = p.pos + 1
		return p.escapeAtom(e)
	}
	if c == 0 {
		return nil, newSyntaxError("unexpected end of pattern")
	}
	p.pos = p.pos + 1
	return &Regexp{Op: OpLiteral, Rune: []rune{rune(c)}}, nil
}

var digitRanges = []rune{48, 57}
var wordRanges = []rune{48, 57, 65, 90, 97, 122, 95, 95}
var spaceRanges = []rune{32, 32, 9, 9, 10, 10, 13, 13, 12, 12, 11, 11}

func negateByteRanges(ranges []rune) []rune {
	var out []rune
	cur := rune(0)
	for i := 0; i+1 < len(ranges); i = i + 2 {
		lo := ranges[i]
		hi := ranges[i+1]
		if cur < lo {
			out = append(out, cur, lo-1)
		}
		if hi+1 > cur {
			cur = hi + 1
		}
	}
	if cur <= 255 {
		out = append(out, cur, 255)
	}
	return out
}

func (p *parser) escapeAtom(e byte) (*Regexp, error) {
	if e == 100 {
		return &Regexp{Op: OpCharClass, Rune: digitRanges}, nil
	}
	if e == 68 {
		return &Regexp{Op: OpCharClass, Rune: negateByteRanges(digitRanges)}, nil
	}
	if e == 119 {
		return &Regexp{Op: OpCharClass, Rune: wordRanges}, nil
	}
	if e == 87 {
		return &Regexp{Op: OpCharClass, Rune: negateByteRanges(wordRanges)}, nil
	}
	if e == 115 {
		return &Regexp{Op: OpCharClass, Rune: spaceRanges}, nil
	}
	if e == 83 {
		return &Regexp{Op: OpCharClass, Rune: negateByteRanges(spaceRanges)}, nil
	}
	if e == 110 {
		return &Regexp{Op: OpLiteral, Rune: []rune{10}}, nil
	}
	if e == 116 {
		return &Regexp{Op: OpLiteral, Rune: []rune{9}}, nil
	}
	if e == 114 {
		return &Regexp{Op: OpLiteral, Rune: []rune{13}}, nil
	}
	return &Regexp{Op: OpLiteral, Rune: []rune{rune(e)}}, nil
}

func classEscapeByte(e byte) byte {
	if e == 110 {
		return 10
	}
	if e == 116 {
		return 9
	}
	if e == 114 {
		return 13
	}
	return e
}

func (p *parser) parseClass() (*Regexp, error) {
	p.pos = p.pos + 1
	neg := false
	if p.peek() == 94 {
		neg = true
		p.pos = p.pos + 1
	}
	var ranges []rune
	first := true
	for p.pos < len(p.s) && (p.peek() != 93 || first) {
		first = false
		lo := p.peek()
		if lo == 92 {
			p.pos = p.pos + 1
			lo = classEscapeByte(p.peek())
		}
		p.pos = p.pos + 1
		hi := lo
		if p.peek() == 45 && p.pos+1 < len(p.s) && p.s[p.pos+1] != 93 {
			p.pos = p.pos + 1
			hiB := p.peek()
			if hiB == 92 {
				p.pos = p.pos + 1
				hiB = classEscapeByte(p.peek())
			}
			p.pos = p.pos + 1
			hi = hiB
		}
		ranges = append(ranges, rune(lo), rune(hi))
	}
	if p.peek() != 93 {
		return nil, newSyntaxError("missing closing ]")
	}
	p.pos = p.pos + 1
	if neg {
		ranges = negateByteRanges(ranges)
	}
	return &Regexp{Op: OpCharClass, Rune: ranges}, nil
}
