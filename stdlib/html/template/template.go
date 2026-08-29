// Bounded html/template: same `{{.Field}}`/`{{if}}` engine as this
// project's own `text/template` (a separate, independent implementation,
// NOT a wrapper reusing text/template's unexported internals -- same
// "keep parsers independent" precedent as regexp/syntax's own package
// comment, since there is no way to share unexported package internals
// here anyway). The one real difference: every `{{.Field}}` substitution
// is passed through `html.EscapeString` before being written out.
//
// This is NOT real Go's html/template: real Go does CONTEXTUAL escaping
// (it parses enough HTML/JS/CSS/URL structure to know a value landing
// inside a `<script>` block needs JS-string escaping, one inside an
// `href` needs URL escaping, and only plain HTML text needs entity
// escaping) -- a genuinely large, structure-aware feature. This package
// always applies the SAME `html.EscapeString` regardless of where the
// value lands. That is safe-by-default for the common case (interpolating
// into ordinary HTML text) and never introduces script injection through
// entity-escaped `<`/`>`/`&`/`"`/`'`, but it is NOT a substitute for real
// contextual escaping inside `<script>`/`<style>`/a URL attribute, where
// HTML entity escaping is the wrong escaping mechanism entirely -- a
// stated, real gap, not glossed over.
package template

import (
	"errors"
	"html"
	"io"
	"reflect"
	"strconv"
	"strings"
)

type nodeKind int

const nodeText nodeKind = 0
const nodeAction nodeKind = 1
const nodeIf nodeKind = 2

type node struct {
	kind     nodeKind
	text     string
	path     string
	ifBody   []node
	elseBody []node
}

type token struct {
	isAction bool
	text     string
}

func tokenize(s string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], "{{")
		if j < 0 {
			toks = append(toks, token{isAction: false, text: s[i:]})
			break
		}
		if j > 0 {
			toks = append(toks, token{isAction: false, text: s[i : i+j]})
		}
		start := i + j + 2
		k := strings.Index(s[start:], "}}")
		if k < 0 {
			return nil, errors.New("template: unterminated action")
		}
		content := strings.TrimSpace(s[start : start+k])
		toks = append(toks, token{isAction: true, text: content})
		i = start + k + 2
	}
	return toks, nil
}

func buildNodes(toks []token, pos int) ([]node, int, string, error) {
	var nodes []node
	for pos < len(toks) {
		t := toks[pos]
		if !t.isAction {
			nodes = append(nodes, node{kind: nodeText, text: t.text})
			pos = pos + 1
			continue
		}
		content := t.text
		if content == "end" {
			return nodes, pos + 1, "end", nil
		}
		if content == "else" {
			return nodes, pos + 1, "else", nil
		}
		if strings.HasPrefix(content, "if ") {
			condPath := strings.TrimSpace(content[3:])
			condPath = strings.TrimPrefix(condPath, ".")
			ifBody, newPos, stop, err := buildNodes(toks, pos+1)
			if err != nil {
				return nil, 0, "", err
			}
			var elseBody []node
			if stop == "else" {
				elseBody, newPos, stop, err = buildNodes(toks, newPos)
				if err != nil {
					return nil, 0, "", err
				}
			}
			if stop != "end" {
				return nil, 0, "", errors.New("template: missing {{end}} for {{if}}")
			}
			nodes = append(nodes, node{kind: nodeIf, path: condPath, ifBody: ifBody, elseBody: elseBody})
			pos = newPos
			continue
		}
		path := strings.TrimPrefix(content, ".")
		nodes = append(nodes, node{kind: nodeAction, text: path})
		pos = pos + 1
	}
	return nodes, pos, "", nil
}

func lookupPath(rv reflect.Value, path string) (reflect.Value, error) {
	if path == "" {
		return rv, nil
	}
	parts := strings.Split(path, ".")
	cur := rv
	i := 0
	for i < len(parts) {
		if cur.Kind() != reflect.Struct {
			return cur, errors.New("template: cannot access field \"" + parts[i] + "\" on a non-struct value")
		}
		found := false
		n := cur.NumField()
		j := 0
		for j < n {
			if cur.FieldName(j) == parts[i] {
				cur = cur.Field(j)
				found = true
				break
			}
			j = j + 1
		}
		if !found {
			return cur, errors.New("template: no such field: " + parts[i])
		}
		i = i + 1
	}
	return cur, nil
}

func truthy(rv reflect.Value) bool {
	k := rv.Kind()
	if k == reflect.Bool {
		return rv.Bool()
	}
	if k == reflect.String {
		return rv.String() != ""
	}
	if k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64 {
		return rv.Int() != 0
	}
	if k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 {
		return rv.Int() != 0
	}
	if k == reflect.Float32 || k == reflect.Float64 {
		return rv.Float() != 0
	}
	return true
}

func formatFloat(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	whole := int64(f)
	frac := f - float64(whole)
	out := strconv.FormatInt(whole, 10)
	if frac > 0.0000001 {
		scaled := int64(frac*1000000.0 + 0.5)
		fs := strconv.FormatInt(scaled, 10)
		for len(fs) < 6 {
			fs = "0" + fs
		}
		end := len(fs)
		for end > 0 && fs[end-1:end] == "0" {
			end--
		}
		if end > 0 {
			out = out + "." + fs[0:end]
		}
	}
	if neg {
		out = "-" + out
	}
	return out
}

func stringify(rv reflect.Value) string {
	k := rv.Kind()
	if k == reflect.String {
		return rv.String()
	}
	if k == reflect.Bool {
		if rv.Bool() {
			return "true"
		}
		return "false"
	}
	if k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64 {
		return strconv.FormatInt(rv.Int(), 10)
	}
	if k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 {
		return strconv.FormatInt(rv.Int(), 10)
	}
	if k == reflect.Float32 || k == reflect.Float64 {
		return formatFloat(rv.Float())
	}
	return ""
}

func execNodes(nodes []node, data reflect.Value, buf []byte) ([]byte, error) {
	i := 0
	for i < len(nodes) {
		nd := nodes[i]
		if nd.kind == nodeText {
			buf = append(buf, nd.text...)
		} else if nd.kind == nodeAction {
			v, err := lookupPath(data, nd.text)
			if err != nil {
				return buf, err
			}
			buf = append(buf, html.EscapeString(stringify(v))...)
		} else if nd.kind == nodeIf {
			v, err := lookupPath(data, nd.path)
			if err != nil {
				return buf, err
			}
			var nb []byte
			var err2 error
			if truthy(v) {
				nb, err2 = execNodes(nd.ifBody, data, buf)
			} else {
				nb, err2 = execNodes(nd.elseBody, data, buf)
			}
			if err2 != nil {
				return buf, err2
			}
			buf = nb
		}
		i = i + 1
	}
	return buf, nil
}

type Template struct {
	name  string
	nodes []node
}

func New(name string) *Template {
	return &Template{name: name}
}

func (t *Template) Parse(text string) (*Template, error) {
	toks, err := tokenize(text)
	if err != nil {
		return nil, err
	}
	nodes, _, stop, err2 := buildNodes(toks, 0)
	if err2 != nil {
		return nil, err2
	}
	if stop != "" {
		return nil, errors.New("template: unexpected {{" + stop + "}}")
	}
	t.nodes = nodes
	return t, nil
}

func Must(t *Template, err error) *Template {
	if err != nil {
		panic(err)
	}
	return t
}

func (t *Template) Execute(w io.Writer, data any) error {
	rv := reflect.ValueOf(data)
	out, err := execNodes(t.nodes, rv, nil)
	if err != nil {
		return err
	}
	_, werr := w.Write(out)
	return werr
}
