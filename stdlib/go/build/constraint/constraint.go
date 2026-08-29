// Build tag constraint expressions ("//go:build linux && amd64", the
// older "// +build linux amd64" line syntax). One tagged Expr struct, not
// real Go's Expr interface (TagExpr/NotExpr/AndExpr/OrExpr each their own
// concrete type) -- same simplification go/ast already made here, for the
// identical reason (this compiler's own AST is a tagged struct too, and
// a small closed set of four kinds doesn't need an interface hierarchy).
//
// Parse takes the comment TEXT itself (the caller already extracted it),
// so unlike go/build or go/doc this package needs no scanner/comment
// association work -- fully self-contained.
//
// Not implemented: PlusBuildLines (converting an arbitrary Expr back into
// old-style "+build" lines -- only possible for a subset of expressions,
// real Go returns an error for the rest; not needed yet, would be a real,
// bounded addition later, not a stub, if a need comes up).
package constraint

import "errors"

type ExprKind int

const (
	TagExpr ExprKind = iota
	NotExpr
	AndExpr
	OrExpr
)

type Expr struct {
	Kind ExprKind
	Tag  string
	X    *Expr
	Y    *Expr
}

// Eval reports whether the expression is satisfied, calling ok(tag) to
// decide whether a given build tag is set.
func (e *Expr) Eval(ok func(tag string) bool) bool {
	if e.Kind == TagExpr {
		return ok(e.Tag)
	}
	if e.Kind == NotExpr {
		return !e.X.Eval(ok)
	}
	if e.Kind == AndExpr {
		return e.X.Eval(ok) && e.Y.Eval(ok)
	}
	if e.Kind == OrExpr {
		return e.X.Eval(ok) || e.Y.Eval(ok)
	}
	return false
}

func (e *Expr) String() string {
	if e.Kind == TagExpr {
		return e.Tag
	}
	if e.Kind == NotExpr {
		return "!" + parenIfNeeded(e.X)
	}
	if e.Kind == AndExpr {
		return parenIfNeeded(e.X) + " && " + parenIfNeeded(e.Y)
	}
	if e.Kind == OrExpr {
		return parenIfNeeded(e.X) + " || " + parenIfNeeded(e.Y)
	}
	return ""
}

func parenIfNeeded(e *Expr) string {
	if e.Kind == AndExpr || e.Kind == OrExpr {
		return "(" + e.String() + ")"
	}
	return e.String()
}

// IsGoBuild reports whether line is a "//go:build" constraint line.
func IsGoBuild(line string) bool {
	return hasPrefix(line, "//go:build")
}

// IsPlusBuild reports whether line is an old-style "// +build" line.
func IsPlusBuild(line string) bool {
	trimmed := trimTrailingSpace(line)
	return hasPrefix(trimmed, "// +build") || hasPrefix(trimmed, "//+build")
}

func hasPrefix(s string, p string) bool {
	if len(s) < len(p) {
		return false
	}
	return s[0:len(p)] == p
}

func trimTrailingSpace(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == ' ' || s[end-1] == '\t') {
		end = end - 1
	}
	return s[0:end]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func isTagByte(b byte) bool {
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b >= '0' && b <= '9' {
		return true
	}
	return b == '_' || b == '.'
}

type constraintParser struct {
	s   string
	pos int
}

func (p *constraintParser) skipSpace() {
	for p.pos < len(p.s) && isSpace(p.s[p.pos]) {
		p.pos = p.pos + 1
	}
}

func (p *constraintParser) peek() byte {
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

func (p *constraintParser) parseTag() (*Expr, error) {
	start := p.pos
	for p.pos < len(p.s) && isTagByte(p.s[p.pos]) {
		p.pos = p.pos + 1
	}
	if p.pos == start {
		return nil, errors.New("constraint: missing build tag")
	}
	return &Expr{Kind: TagExpr, Tag: p.s[start:p.pos]}, nil
}

func (p *constraintParser) parsePrimary() (*Expr, error) {
	p.skipSpace()
	if p.peek() == '!' {
		p.pos = p.pos + 1
		p.skipSpace()
		x, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &Expr{Kind: NotExpr, X: x}, nil
	}
	if p.peek() == '(' {
		p.pos = p.pos + 1
		x, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return nil, errors.New("constraint: missing close paren")
		}
		p.pos = p.pos + 1
		return x, nil
	}
	return p.parseTag()
}

func (p *constraintParser) parseAnd() (*Expr, error) {
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.pos+1 < len(p.s) && p.s[p.pos] == '&' && p.s[p.pos+1] == '&' {
			p.pos = p.pos + 2
			p.skipSpace()
			y, err2 := p.parsePrimary()
			if err2 != nil {
				return nil, err2
			}
			x = &Expr{Kind: AndExpr, X: x, Y: y}
			continue
		}
		break
	}
	return x, nil
}

func (p *constraintParser) parseOr() (*Expr, error) {
	x, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.pos+1 < len(p.s) && p.s[p.pos] == '|' && p.s[p.pos+1] == '|' {
			p.pos = p.pos + 2
			p.skipSpace()
			y, err2 := p.parseAnd()
			if err2 != nil {
				return nil, err2
			}
			x = &Expr{Kind: OrExpr, X: x, Y: y}
			continue
		}
		break
	}
	return x, nil
}

func parseGoBuildExpr(s string) (*Expr, error) {
	p := &constraintParser{s: s}
	x, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return nil, errors.New("constraint: unexpected token")
	}
	return x, nil
}

// parsePlusBuildLine parses the old "// +build a b c" style body (already
// stripped of the leading "// +build "): space-separated terms are OR'd;
// within one term, comma-separated names are AND'd; a leading '!' on a
// name negates it.
func parsePlusBuildLine(body string) (*Expr, error) {
	var result *Expr
	fields := splitFields(body)
	if len(fields) == 0 {
		return nil, errors.New("constraint: empty +build line")
	}
	for _, field := range fields {
		var term *Expr
		names := splitComma(field)
		for _, name := range names {
			var t *Expr
			if len(name) > 0 && name[0] == '!' {
				t = &Expr{Kind: NotExpr, X: &Expr{Kind: TagExpr, Tag: name[1:]}}
			} else {
				t = &Expr{Kind: TagExpr, Tag: name}
			}
			if term == nil {
				term = t
			} else {
				term = &Expr{Kind: AndExpr, X: term, Y: t}
			}
		}
		if result == nil {
			result = term
		} else {
			result = &Expr{Kind: OrExpr, X: result, Y: term}
		}
	}
	return result, nil
}

func splitFields(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i = i + 1
		}
		start := i
		for i < len(s) && !isSpace(s[i]) {
			i = i + 1
		}
		if i > start {
			out = append(out, s[start:i])
		}
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// Parse parses a single build-constraint line, either "//go:build ..." or
// the older "// +build ...".
func Parse(line string) (*Expr, error) {
	trimmed := trimTrailingSpace(line)
	if IsGoBuild(trimmed) {
		return parseGoBuildExpr(trimmed[len("//go:build"):])
	}
	if hasPrefix(trimmed, "// +build") {
		return parsePlusBuildLine(trimmed[len("// +build"):])
	}
	if hasPrefix(trimmed, "//+build") {
		return parsePlusBuildLine(trimmed[len("//+build"):])
	}
	return nil, errors.New("constraint: not a build constraint line")
}
