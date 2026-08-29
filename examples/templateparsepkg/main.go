package main

import (
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
)

type Data struct {
	Ok bool
}

func renderNode(n *parse.Node, data Data) string {
	if n.Type == parse.NodeList {
		out := ""
		i := 0
		for i < len(n.Nodes) {
			out = out + renderNode(n.Nodes[i], data)
			i = i + 1
		}
		return out
	}
	if n.Type == parse.NodeText {
		return n.Text
	}
	if n.Type == parse.NodeIf {
		if data.Ok {
			return renderNode(n.List, data)
		}
		if n.ElseList != nil {
			return renderNode(n.ElseList, data)
		}
		return ""
	}
	return ""
}

func main() {
	t1, err := parse.Parse("plain", "hello")
	fmt.Println(err == nil)
	fmt.Println(t1.Name == "plain")
	fmt.Println(t1.Root.Type == parse.NodeList)
	fmt.Println(len(t1.Root.Nodes) == 1)
	fmt.Println(t1.Root.Nodes[0].Type == parse.NodeText)
	fmt.Println(t1.Root.Nodes[0].Text == "hello")

	t2, _ := parse.Parse("field", "{{.Name}}")
	fmt.Println(t2.Root.Nodes[0].Type == parse.NodeAction)
	fmt.Println(t2.Root.Nodes[0].Pipe.Type == parse.NodeField)
	fmt.Println(t2.Root.Nodes[0].Pipe.Text == "Name")

	tmplText := "{{if .Ok}}yes{{else}}no{{end}}"
	t3, _ := parse.Parse("ifelse", tmplText)
	fmt.Println(t3.Root.Nodes[0].Type == parse.NodeIf)
	fmt.Println(t3.Root.Nodes[0].Pipe.Text == "Ok")
	fmt.Println(t3.Root.Nodes[0].List.Nodes[0].Text == "yes")
	fmt.Println(t3.Root.Nodes[0].ElseList.Nodes[0].Text == "no")

	tmpl, tmplErr := template.New("x").Parse(tmplText)
	tt := template.Must(tmpl, tmplErr)
	var buf1 strings.Builder
	tt.Execute(&buf1, Data{Ok: true})
	fmt.Println(buf1.String() == renderNode(t3.Root, Data{Ok: true}))

	var buf2 strings.Builder
	tt.Execute(&buf2, Data{Ok: false})
	fmt.Println(buf2.String() == renderNode(t3.Root, Data{Ok: false}))

	_, badErr := parse.Parse("bad", "{{if .X}}oops")
	fmt.Println(badErr != nil)
}
