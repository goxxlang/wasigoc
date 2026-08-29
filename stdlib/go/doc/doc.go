// Bounded go/doc: package documentation extraction for exactly the
// top-level declarations this project's own go/parser produces --
// `func` declarations and single-spec `var`/`const` (no type
// declarations at all, since go/parser doesn't parse those; see its own
// tracker line -- so unlike real Go's go/doc, this package has no
// `Type` documentation). `New` takes one already-parsed file (this
// project's go/parser has no multi-file package concept, unlike real
// go/doc.New's `*ast.Package`), plus that file's `[]scanner.Comment`
// from `parser.ParseFileWithComments` -- comment association needs the
// raw comment text/positions go/parser doesn't otherwise expose.
//
// Association algorithm: for a declaration at position P, walk
// `Comments` backward from the nearest one before P, including it (and
// continuing further back) only while the raw source between the
// comment's end and the current anchor is pure whitespace containing at
// most one newline -- i.e. "immediately precedes, no blank line
// in-between, nothing else in between either" -- the same shape real
// Go's own `ast.CommentGroup`-to-declaration association uses, just
// computed directly from source text instead of a pre-built line/File
// position table. Package-level Doc uses the same walk anchored at the
// `package` keyword's own position.
//
// Verified against real Go itself (go1.26.4, installed locally): the
// exact same doc-comment-bearing source (a package comment, a
// doc-commented func, a doc-commented var, and a declaration
// deliberately separated from its preceding comment by a blank line)
// was fed to real Go's own `go/doc.New`, and this package's Doc-field
// results matched for every case, including the blank-line case
// correctly producing an EMPTY Doc (not swallowing an unrelated
// preceding comment).
package doc

import (
	"go/ast"
	"go/scanner"
	"strings"
)

type Value struct {
	Doc  string
	Name string
}

type Func struct {
	Doc  string
	Name string
}

type Package struct {
	Name   string
	Doc    string
	Funcs  []*Func
	Consts []*Value
	Vars   []*Value
}

func stripCommentMarkers(text string) string {
	if strings.HasPrefix(text, "//") {
		s := strings.TrimPrefix(text, "//")
		return strings.TrimPrefix(s, " ")
	}
	if strings.HasPrefix(text, "/*") {
		s := strings.TrimPrefix(text, "/*")
		s = strings.TrimSuffix(s, "*/")
		return strings.TrimPrefix(s, " ")
	}
	return text
}

// commentsBefore returns, in source order, the maximal run of comments
// immediately preceding anchor -- see the package comment's "Association
// algorithm" note.
func commentsBefore(comments []scanner.Comment, src string, anchor int) []scanner.Comment {
	var run []scanner.Comment
	i := len(comments) - 1
	for i >= 0 {
		c := comments[i]
		cEnd := int(c.Pos) + len(c.Text)
		if cEnd > anchor {
			i = i - 1
			continue
		}
		gap := src[cEnd:anchor]
		if strings.TrimSpace(gap) != "" {
			break
		}
		if strings.Count(gap, "\n") >= 2 {
			break
		}
		run = append([]scanner.Comment{c}, run...)
		anchor = int(c.Pos)
		i = i - 1
	}
	return run
}

func docText(comments []scanner.Comment, src string, anchor int) string {
	run := commentsBefore(comments, src, anchor)
	if len(run) == 0 {
		return ""
	}
	out := ""
	i := 0
	for i < len(run) {
		line := stripCommentMarkers(run[i].Text)
		out = out + line + "\n"
		i = i + 1
	}
	return out
}

// New builds a Package from one already-parsed file (an ast.File node
// from go/parser) plus its comments (from
// go/parser.ParseFileWithComments) and the original source text (needed
// to inspect the whitespace between a comment and the declaration it
// may document).
func isExported(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// New builds a Package from one already-parsed file (an ast.File node
// from go/parser) plus its comments (from
// go/parser.ParseFileWithComments) and the original source text (needed
// to inspect the whitespace between a comment and the declaration it
// may document). Same default-mode filtering as real Go's own
// `go/doc.New` (mode 0, no `AllDecls`): only exported names are
// included -- there's no `Mode`/`AllDecls` override here.
func New(file *ast.Node, comments []scanner.Comment, src string) *Package {
	pkg := &Package{Name: file.Name}
	pkg.Doc = docText(comments, src, int(file.Pos))

	i := 0
	for i < len(file.List) {
		d := file.List[i]
		if !isExported(d.Name) {
			i = i + 1
			continue
		}
		if d.Kind == ast.FuncDecl {
			pkg.Funcs = append(pkg.Funcs, &Func{
				Doc:  docText(comments, src, int(d.Pos)),
				Name: d.Name,
			})
		} else if d.Kind == ast.VarSpec {
			pkg.Vars = append(pkg.Vars, &Value{
				Doc:  docText(comments, src, int(d.Pos)),
				Name: d.Name,
			})
		} else if d.Kind == ast.ConstSpec {
			pkg.Consts = append(pkg.Consts, &Value{
				Doc:  docText(comments, src, int(d.Pos)),
				Name: d.Name,
			})
		}
		i = i + 1
	}
	return pkg
}
