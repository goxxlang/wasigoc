package main

import (
	"fmt"
	"go/doc/comment"
)

func main() {
	text := "Foo does the thing.\nIt returns a value.\n\n# Details\n\nMore text here\nspanning two lines.\n\n    codeLine1()\n    codeLine2()\n"

	doc := comment.Parse(text)
	fmt.Println(len(doc.Content))

	fmt.Println(doc.Content[0].Kind == comment.ParagraphBlock)
	fmt.Println(len(doc.Content[0].Lines))
	fmt.Println(doc.Content[0].Lines[0])
	fmt.Println(doc.Content[0].Lines[1])

	fmt.Println(doc.Content[1].Kind == comment.HeadingBlock)
	fmt.Println(doc.Content[1].Text)

	fmt.Println(doc.Content[2].Kind == comment.ParagraphBlock)
	fmt.Println(len(doc.Content[2].Lines))

	fmt.Println(doc.Content[3].Kind == comment.CodeBlock)
	fmt.Println(len(doc.Content[3].Lines))
	fmt.Println(doc.Content[3].Lines[0])
}
