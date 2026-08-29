// Bounded subset of go/doc/comment: parses ALREADY-EXTRACTED doc-comment
// TEXT (a plain string) into a block structure -- unlike `go/doc` itself,
// this doesn't need comment-to-declaration association (which `go/scanner`
// currently discards entirely, the reason `go/doc` stays todo), since the
// caller supplies the raw comment text directly.
//
// One tagged `Block` struct with a `Kind` enum, not real Go's
// Block-interface hierarchy (Paragraph/Heading/Code/List each a distinct
// concrete type) -- same simplification precedent as this project's own
// `go/ast` Node. Recognizes blank-line-separated paragraphs, `# Heading`
// lines (the Go 1.19+ doc-comment heading convention), and indented lines
// as a Code block. No inline spans (bold/italic/links within a paragraph
// -- real Go's `Text` is `[]Text` with `Plain`/`Italic`/`Link` variants,
// here a paragraph's lines are plain strings), no `List` blocks, no
// `Printer` (Text/Markdown/HTML/Comment output formats).
package comment

import "strings"

type BlockKind int

const (
	ParagraphBlock BlockKind = iota
	HeadingBlock
	CodeBlock
)

type Block struct {
	Kind  BlockKind
	Text  string
	Lines []string
}

type Doc struct {
	Content []Block
}

func hasIndent(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == 9)
}

func Parse(text string) *Doc {
	lines := strings.Split(text, "\n")
	var blocks []Block
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i = i + 1
			continue
		}
		if strings.HasPrefix(line, "# ") {
			blocks = append(blocks, Block{Kind: HeadingBlock, Text: strings.TrimSpace(line[2:])})
			i = i + 1
			continue
		}
		if hasIndent(line) {
			var codeLines []string
			for i < len(lines) && hasIndent(lines[i]) {
				codeLines = append(codeLines, lines[i])
				i = i + 1
			}
			blocks = append(blocks, Block{Kind: CodeBlock, Lines: codeLines})
			continue
		}
		var paraLines []string
		for i < len(lines) {
			l := lines[i]
			if strings.TrimSpace(l) == "" {
				break
			}
			if strings.HasPrefix(l, "# ") {
				break
			}
			if hasIndent(l) {
				break
			}
			paraLines = append(paraLines, l)
			i = i + 1
		}
		blocks = append(blocks, Block{Kind: ParagraphBlock, Lines: paraLines})
	}
	return &Doc{Content: blocks}
}
