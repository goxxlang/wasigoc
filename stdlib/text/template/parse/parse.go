// Bounded text/template/parse: a public parse tree for exactly the
// template subset this project's own `text/template` supports -- plain
// text, `{{.Field}}`/`{{.Field.Nested}}` actions, and nestable
// `{{if .Field}}...{{else}}...{{end}}` -- NOT real Go's actual wide
// Node interface hierarchy (ListNode/TextNode/PipeNode/ActionNode/
// IfNode/RangeNode/..., each its own concrete type embedding NodeType).
// One tagged `Node` struct (a `Type` enum field) instead, the same
// simplification precedent this project's `go/ast` and `text/template`'s
// own internal `node` already established -- deliberately, not an
// oversight: a Go interface hierarchy this wide would need one `adapt<T>`
// vtable per concrete node type for a feature this project's `template`
// package doesn't exercise anyway (no pipelines, range, with, variables,
// functions, or `{{define}}`/`{{template}}` -- so no need for real Go's
// `map[string]*Tree` multi-template return; `Parse` returns one `*Tree`).
// `Type`/`Text`/`Nodes`/`Pipe`/`List`/`ElseList` cover exactly the shapes
// `text/template`'s own tokenizer already recognizes; this package is an
// independent second implementation of that same tokenizer (not a
// wrapper -- there's no way to share an unexported package's internals
// here), rebuilt to hand back a public tree instead of executing inline.
// Verified two ways: (1) against real Go itself (go1.26.4, installed
// locally) -- every template string this package's own golden test
// parses was also confirmed to parse without error under real Go's own
// `text/template/parse.Parse`, so none of the test inputs are
// accidentally invalid Go template syntax; (2) cross-checked against
// this project's OWN `text/template` package: walking this package's
// returned Tree by hand for a `{{if .Ok}}yes{{else}}no{{end}}`-shaped
// template and computing the same substitution independently produced
// output identical to actually running that template through
// `text/template.Template.Execute`.
package parse

import (
	"errors"
	"strings"
)

type NodeType int

const NodeText NodeType = 0
const NodeAction NodeType = 1
const NodeIf NodeType = 2
const NodeList NodeType = 3
const NodeField NodeType = 4

// Node is one parse-tree node. Which fields are meaningful depends on
// Type:
//   - NodeText: Text holds the literal text.
//   - NodeField: Text holds the dotted field path, leading "." stripped
//     (matching real Go's own FieldNode.Ident, joined back with "."
//     instead of split into a []string -- this project's bounded
//     `template` package already works on the joined form).
//   - NodeAction: Pipe holds the field-path Node the action prints.
//   - NodeIf: Pipe holds the condition's field-path Node, List holds the
//     if-body (a NodeList), ElseList holds the else-body (a NodeList,
//     nil if the {{if}} had no {{else}}).
//   - NodeList: Nodes holds the child nodes in order.
type Node struct {
	Type     NodeType
	Text     string
	Nodes    []*Node
	Pipe     *Node
	List     *Node
	ElseList *Node
}

// Tree is the parse tree for a single template.
type Tree struct {
	Name string
	Root *Node
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

func fieldNode(path string) *Node {
	return &Node{Type: NodeField, Text: strings.TrimPrefix(path, ".")}
}

func buildNodes(toks []token, pos int) ([]*Node, int, string, error) {
	var nodes []*Node
	for pos < len(toks) {
		t := toks[pos]
		if !t.isAction {
			nodes = append(nodes, &Node{Type: NodeText, Text: t.text})
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
			ifNodes, newPos, stop, err := buildNodes(toks, pos+1)
			if err != nil {
				return nil, 0, "", err
			}
			var elseNodes []*Node
			if stop == "else" {
				elseNodes, newPos, stop, err = buildNodes(toks, newPos)
				if err != nil {
					return nil, 0, "", err
				}
			}
			if stop != "end" {
				return nil, 0, "", errors.New("template: missing {{end}} for {{if}}")
			}
			ifNode := &Node{Type: NodeIf, Pipe: fieldNode(condPath), List: &Node{Type: NodeList, Nodes: ifNodes}}
			if elseNodes != nil {
				ifNode.ElseList = &Node{Type: NodeList, Nodes: elseNodes}
			}
			nodes = append(nodes, ifNode)
			pos = newPos
			continue
		}
		nodes = append(nodes, &Node{Type: NodeAction, Pipe: fieldNode(content)})
		pos = pos + 1
	}
	return nodes, pos, "", nil
}

// Parse parses text into a Tree named name. Bounded to the same subset
// text/template supports -- no delimiters/funcMap parameters, since
// neither is configurable here.
func Parse(name string, text string) (*Tree, error) {
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
	return &Tree{Name: name, Root: &Node{Type: NodeList, Nodes: nodes}}, nil
}
