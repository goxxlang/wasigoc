// Tiny subset of regexp: a plain recursive-descent parser + backtracking
// matcher (continuation-passing, the classic simple-regex-engine shape --
// not RE2-style guaranteed-linear-time, but correct for everyday patterns
// and needs no extra runtime support). Supports literals, `.`, `*` `+` `?`
// (greedy only), character classes `[...]`/`[^...]` with ranges,
// `\d \D \w \W \s \S` shorthand classes, `^` `$` anchors (start/end of the
// whole string, no multiline mode), `|` alternation, and `(...)` grouping
// for precedence -- but grouping does NOT capture: there is no
// FindStringSubmatch here, only whole-match Find*/Match*. No `{m,n}`
// counted repetition, no non-greedy `*?`, no lookaround.
package regexp

import "errors"

const (
	nChar = iota
	nAny
	nClass
	nStar
	nPlus
	nQuest
	nConcat
	nAlt
	nGroup
	nStart
	nEnd
)

type classRange struct {
	lo byte
	hi byte
}

type node struct {
	kind   int
	ch     byte
	ranges []classRange
	negate bool
	sub    []*node
}

type reParser struct {
	s   string
	pos int
}

func (p *reParser) peek() byte {
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

func (p *reParser) parseAlt() (*node, error) {
	first, err := p.parseConcat()
	if err != nil {
		return nil, err
	}
	alts := []*node{first}
	for p.peek() == 124 {
		p.pos++
		n, err2 := p.parseConcat()
		if err2 != nil {
			return nil, err2
		}
		alts = append(alts, n)
	}
	if len(alts) == 1 {
		return first, nil
	}
	return &node{kind: nAlt, sub: alts}, nil
}

func (p *reParser) parseConcat() (*node, error) {
	var parts []*node
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
	return &node{kind: nConcat, sub: parts}, nil
}

func (p *reParser) parseRepeat() (*node, error) {
	a, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for {
		c := p.peek()
		if c == 42 {
			p.pos++
			a = &node{kind: nStar, sub: []*node{a}}
			continue
		}
		if c == 43 {
			p.pos++
			a = &node{kind: nPlus, sub: []*node{a}}
			continue
		}
		if c == 63 {
			p.pos++
			a = &node{kind: nQuest, sub: []*node{a}}
			continue
		}
		break
	}
	return a, nil
}

func (p *reParser) parseAtom() (*node, error) {
	c := p.peek()
	if c == 40 {
		p.pos++
		if p.pos+1 < len(p.s) && p.s[p.pos] == 63 && p.s[p.pos+1] == 58 {
			p.pos = p.pos + 2
		}
		n, err := p.parseAlt()
		if err != nil {
			return nil, err
		}
		if p.peek() != 41 {
			return nil, errors.New("regexp: missing closing )")
		}
		p.pos++
		return &node{kind: nGroup, sub: []*node{n}}, nil
	}
	if c == 46 {
		p.pos++
		return &node{kind: nAny}, nil
	}
	if c == 94 {
		p.pos++
		return &node{kind: nStart}, nil
	}
	if c == 36 {
		p.pos++
		return &node{kind: nEnd}, nil
	}
	if c == 91 {
		return p.parseClass()
	}
	if c == 92 {
		p.pos++
		e := p.peek()
		p.pos++
		return p.escapeNode(e)
	}
	if c == 0 {
		return nil, errors.New("regexp: unexpected end of pattern")
	}
	p.pos++
	return &node{kind: nChar, ch: c}, nil
}

var digitClass = []classRange{classRange{48, 57}}
var wordClass = []classRange{classRange{48, 57}, classRange{65, 90}, classRange{97, 122}, classRange{95, 95}}
var spaceClass = []classRange{classRange{32, 32}, classRange{9, 9}, classRange{10, 10}, classRange{13, 13}, classRange{12, 12}, classRange{11, 11}}

func (p *reParser) escapeNode(e byte) (*node, error) {
	if e == 100 {
		return &node{kind: nClass, ranges: digitClass}, nil
	}
	if e == 68 {
		return &node{kind: nClass, ranges: digitClass, negate: true}, nil
	}
	if e == 119 {
		return &node{kind: nClass, ranges: wordClass}, nil
	}
	if e == 87 {
		return &node{kind: nClass, ranges: wordClass, negate: true}, nil
	}
	if e == 115 {
		return &node{kind: nClass, ranges: spaceClass}, nil
	}
	if e == 83 {
		return &node{kind: nClass, ranges: spaceClass, negate: true}, nil
	}
	if e == 110 {
		return &node{kind: nChar, ch: 10}, nil
	}
	if e == 116 {
		return &node{kind: nChar, ch: 9}, nil
	}
	if e == 114 {
		return &node{kind: nChar, ch: 13}, nil
	}
	return &node{kind: nChar, ch: e}, nil
}

func (p *reParser) classEscapeByte(e byte) byte {
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

func (p *reParser) parseClass() (*node, error) {
	p.pos++
	neg := false
	if p.peek() == 94 {
		neg = true
		p.pos++
	}
	var ranges []classRange
	first := true
	for p.pos < len(p.s) && (p.peek() != 93 || first) {
		first = false
		lo := p.peek()
		if lo == 92 {
			p.pos++
			lo = p.classEscapeByte(p.peek())
		}
		p.pos++
		if p.peek() == 45 && p.pos+1 < len(p.s) && p.s[p.pos+1] != 93 {
			p.pos++
			hi := p.peek()
			if hi == 92 {
				p.pos++
				hi = p.classEscapeByte(p.peek())
			}
			p.pos++
			ranges = append(ranges, classRange{lo, hi})
		} else {
			ranges = append(ranges, classRange{lo, lo})
		}
	}
	if p.peek() != 93 {
		return nil, errors.New("regexp: missing closing ]")
	}
	p.pos++
	return &node{kind: nClass, ranges: ranges, negate: neg}, nil
}

type Regexp struct {
	root *node
}

func Compile(pattern string) (*Regexp, error) {
	p := &reParser{s: pattern}
	n, err := p.parseAlt()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.s) {
		return nil, errors.New("regexp: unexpected character in pattern")
	}
	return &Regexp{root: n}, nil
}

func MustCompile(pattern string) *Regexp {
	re, err := Compile(pattern)
	if err != nil {
		panic(err.Error())
	}
	return re
}

func classMatches(n *node, c byte) bool {
	found := false
	for i := 0; i < len(n.ranges); i++ {
		if c >= n.ranges[i].lo && c <= n.ranges[i].hi {
			found = true
			break
		}
	}
	if n.negate {
		return !found
	}
	return found
}

func matchStar(sub *node, s string, pos int, cont func(int) bool) bool {
	if matchNode(sub, s, pos, func(p2 int) bool {
		if p2 == pos {
			return false
		}
		return matchStar(sub, s, p2, cont)
	}) {
		return true
	}
	return cont(pos)
}

func matchConcat(parts []*node, i int, s string, pos int, cont func(int) bool) bool {
	if i >= len(parts) {
		return cont(pos)
	}
	return matchNode(parts[i], s, pos, func(p2 int) bool {
		return matchConcat(parts, i+1, s, p2, cont)
	})
}

// matchNode tries to match n at s[pos:], calling cont(newPos) for each way
// it could match, and succeeding as soon as some continuation does --
// classic backtracking via continuation passing (each quantifier/
// alternative just tries the next possibility if cont ultimately fails).
func matchNode(n *node, s string, pos int, cont func(int) bool) bool {
	if n == nil {
		return cont(pos)
	}
	if n.kind == nChar {
		if pos < len(s) && s[pos] == n.ch {
			return cont(pos + 1)
		}
		return false
	}
	if n.kind == nAny {
		if pos < len(s) && s[pos] != 10 {
			return cont(pos + 1)
		}
		return false
	}
	if n.kind == nClass {
		if pos < len(s) && classMatches(n, s[pos]) {
			return cont(pos + 1)
		}
		return false
	}
	if n.kind == nStart {
		if pos == 0 {
			return cont(pos)
		}
		return false
	}
	if n.kind == nEnd {
		if pos == len(s) {
			return cont(pos)
		}
		return false
	}
	if n.kind == nGroup {
		return matchNode(n.sub[0], s, pos, cont)
	}
	if n.kind == nConcat {
		return matchConcat(n.sub, 0, s, pos, cont)
	}
	if n.kind == nAlt {
		for i := 0; i < len(n.sub); i++ {
			if matchNode(n.sub[i], s, pos, cont) {
				return true
			}
		}
		return false
	}
	if n.kind == nQuest {
		if matchNode(n.sub[0], s, pos, cont) {
			return true
		}
		return cont(pos)
	}
	if n.kind == nStar {
		return matchStar(n.sub[0], s, pos, cont)
	}
	if n.kind == nPlus {
		return matchNode(n.sub[0], s, pos, func(p2 int) bool {
			return matchStar(n.sub[0], s, p2, cont)
		})
	}
	return false
}

func (re *Regexp) findAt(s string, start int) (int, int, bool) {
	for i := start; i <= len(s); i++ {
		end := -1
		ok := matchNode(re.root, s, i, func(p2 int) bool {
			end = p2
			return true
		})
		if ok {
			return i, end, true
		}
	}
	return 0, 0, false
}

func (re *Regexp) MatchString(s string) bool {
	_, _, ok := re.findAt(s, 0)
	return ok
}

func (re *Regexp) String() string {
	return ""
}

func Match(pattern string, s string) (bool, error) {
	re, err := Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

func MatchString(pattern string, s string) (bool, error) {
	return Match(pattern, s)
}

func (re *Regexp) FindString(s string) string {
	start, end, ok := re.findAt(s, 0)
	if !ok {
		return ""
	}
	return s[start:end]
}

func (re *Regexp) FindStringIndex(s string) []int {
	start, end, ok := re.findAt(s, 0)
	if !ok {
		return nil
	}
	return []int{start, end}
}

func (re *Regexp) FindAllString(s string, n int) []string {
	var out []string
	pos := 0
	for pos <= len(s) {
		if n >= 0 && len(out) >= n {
			break
		}
		start, end, ok := re.findAt(s, pos)
		if !ok {
			break
		}
		out = append(out, s[start:end])
		if end == start {
			pos = end + 1
		} else {
			pos = end
		}
	}
	return out
}

func (re *Regexp) ReplaceAllString(s string, repl string) string {
	var out []byte
	pos := 0
	for pos <= len(s) {
		start, end, ok := re.findAt(s, pos)
		if !ok {
			out = append(out, s[pos:]...)
			break
		}
		out = append(out, s[pos:start]...)
		out = append(out, repl...)
		if end == start {
			if start < len(s) {
				out = append(out, s[start])
			}
			pos = end + 1
		} else {
			pos = end
		}
	}
	return string(out)
}

func (re *Regexp) Split(s string, n int) []string {
	var out []string
	pos := 0
	for pos <= len(s) {
		if n >= 0 && len(out) >= n-1 {
			break
		}
		start, end, ok := re.findAt(s, pos)
		if !ok || start >= len(s) || end == start {
			break
		}
		out = append(out, s[pos:start])
		pos = end
	}
	out = append(out, s[pos:])
	return out
}
